package handler

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"

	"github.com/gin-gonic/gin"
)

// 定时健康巡检。
//
// 派发路径（popAccount / popMultipleAccounts）默认不再逐个探测上游，
// 账号状态改由这里在后台周期性刷新。两者的关系：
//   - 巡检开着：派发直接查库，毫秒级返回
//   - 巡检关掉：必须打开派发检测，否则账号状态没人维护
//
// 巡检只处理未分配（used = false）的账号：已分配的账号已经交付给用户，
// 再检查它们既不影响派发决策，又会白白占用上游配额。
type healthScanState struct {
	mu          sync.RWMutex
	running     bool
	lastStarted time.Time
	lastEnded   time.Time
	lastChecked int
	lastFlipped int
	lastErr     string
}

var (
	healthScanOnce  sync.Once
	healthScan      healthScanState
	healthScanWake  = make(chan struct{}, 1)
	healthScanEpoch atomic.Uint64
)

// StartHealthScanScheduler 启动巡检调度。多次调用只生效一次。
func StartHealthScanScheduler() {
	healthScanOnce.Do(func() {
		go healthScanLoop()
	})
}

func healthScanLoop() {
	// 启动后先缓一会，避开进程刚起来时的迁移和配置加载。
	const startupDelay = 1 * time.Minute
	base := time.Now()
	nextRun := base.Add(startupDelay)

	for {
		wait := time.Until(nextRun)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)

		select {
		case <-timer.C:
			s := GetCurrentSettings()
			if s.HealthScanEnabled {
				runHealthScanTick(s)
			}
			// 以本轮结束时间为基准排下一轮，避免长时间巡检导致轮次堆叠。
			base = time.Now()
			nextRun = base.Add(scanInterval(s))

		case <-healthScanWake:
			// 设置变更只重算下次时间，不额外触发一轮巡检。
			timer.Stop()
			nextRun = base.Add(scanInterval(GetCurrentSettings()))
			if nextRun.Before(time.Now()) {
				// 新间隔比已等待的时间还短，则下个循环立即执行。
				nextRun = time.Now()
			}
		}
	}
}

func scanInterval(s AppSettings) time.Duration {
	d := time.Duration(s.HealthScanIntervalMinutes) * time.Minute
	if d <= 0 {
		d = 30 * time.Minute
	}
	return d
}

// notifyHealthScanSettingsChanged 让巡检循环立刻采用新的间隔配置。
func notifyHealthScanSettingsChanged() {
	select {
	case healthScanWake <- struct{}{}:
	default:
	}
}

func runHealthScanTick(s AppSettings) {
	healthScan.mu.Lock()
	if healthScan.running {
		healthScan.mu.Unlock()
		log.Println("健康巡检: 上一轮尚未结束，跳过本轮")
		return
	}
	healthScan.running = true
	healthScan.lastStarted = time.Now()
	healthScan.lastErr = ""
	healthScan.mu.Unlock()

	epoch := healthScanEpoch.Add(1)
	checked, flipped, err := scanAccountsOnce(s, epoch)

	healthScan.mu.Lock()
	healthScan.running = false
	healthScan.lastEnded = time.Now()
	healthScan.lastChecked = checked
	healthScan.lastFlipped = flipped
	if err != nil {
		healthScan.lastErr = err.Error()
	}
	healthScan.mu.Unlock()

	cost := time.Since(healthScan.lastStarted).Round(time.Second)
	if err != nil {
		log.Printf("健康巡检结束(有错误): 检查 %d 个，状态变更 %d 个，耗时 %s，错误: %v",
			checked, flipped, cost, err)
		return
	}
	log.Printf("健康巡检结束: 检查 %d 个，状态变更 %d 个，耗时 %s", checked, flipped, cost)
	if checked > 0 {
		AddOpLog("refresh", "定时巡检账号 "+strconv.Itoa(checked)+" 个，状态变更 "+strconv.Itoa(flipped)+" 个", "system")
	}
}

// healthScanCandidates 取本轮要检查的账号。
//
// 只要未分配账号：已分配的号已经交付，刷新它们不影响派发决策。
// last_checked_at 为 NULL 的排最前，其余按时间升序，
// 这样即使每轮配额小于号池规模，也总是在刷新最陈旧的数据。
func healthScanCandidates(budget int) ([]model.Account, error) {
	var candidates []model.Account
	err := database.DB.
		Where("used = ?", false).
		Order("last_checked_at IS NULL DESC, last_checked_at ASC, id ASC").
		Limit(budget).
		Find(&candidates).Error
	return candidates, err
}

// scanAccountsOnce 取一批最久未检查的未分配账号做健康检查。
// 返回实际检查数量和状态发生变化的数量。
func scanAccountsOnce(s AppSettings, epoch uint64) (int, int, error) {
	budget := s.HealthScanBatchSize
	if budget <= 0 {
		budget = 1000
	}

	candidates, err := healthScanCandidates(budget)
	if err != nil {
		return 0, 0, err
	}
	if len(candidates) == 0 {
		return 0, 0, nil
	}

	// 巡检是后台任务，并发压到限流值的一半，给前台的提取和导入留出槽位。
	workers := currentUpstreamCheckConcurrency() / 2
	if workers < 1 {
		workers = 1
	}

	var checked, flipped atomic.Int64
	jobs := make(chan model.Account, workers*2)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for acc := range jobs {
				// 配置被改成关闭时尽早收敛，不把整批跑完。
				if !GetCurrentSettings().HealthScanEnabled {
					return
				}
				// 有新一轮巡检启动说明本轮已被取代，直接退出。
				if healthScanEpoch.Load() != epoch {
					return
				}

				before := acc.Status
				result := checkAccountHealth(acc)
				if err := applyHealthResult(acc.ID, result); err != nil {
					continue
				}
				checked.Add(1)
				if result.status != before {
					flipped.Add(1)
				}
			}
		}()
	}

	for i := range candidates {
		jobs <- candidates[i]
	}
	close(jobs)
	wg.Wait()

	return int(checked.Load()), int(flipped.Load()), nil
}

// HealthScanStatus 暴露巡检运行状态，供设置页展示。
func HealthScanStatus() map[string]interface{} {
	healthScan.mu.RLock()
	defer healthScan.mu.RUnlock()

	status := map[string]interface{}{
		"running":      healthScan.running,
		"lastChecked":  healthScan.lastChecked,
		"lastFlipped":  healthScan.lastFlipped,
		"lastError":    healthScan.lastErr,
		"pendingTotal": pendingHealthScanCount(),
	}
	if !healthScan.lastStarted.IsZero() {
		status["lastStartedAt"] = healthScan.lastStarted
	}
	if !healthScan.lastEnded.IsZero() {
		status["lastEndedAt"] = healthScan.lastEnded
	}
	return status
}

// pendingHealthScanCount 统计还有多少未分配账号在本轮间隔内没被检查过，
// 用于判断当前的每轮数量是否够覆盖整个号池。
func pendingHealthScanCount() int64 {
	if database.DB == nil {
		return 0
	}
	s := GetCurrentSettings()
	interval := time.Duration(s.HealthScanIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	cutoff := time.Now().Add(-interval)

	var n int64
	database.DB.Model(&model.Account{}).
		Where("used = ?", false).
		Where("last_checked_at IS NULL OR last_checked_at < ?", cutoff).
		Count(&n)
	return n
}

// POST /admin/accounts/health-scan
// 手动触发一轮巡检，不等待完成。
func TriggerHealthScan(c *gin.Context) {
	healthScan.mu.RLock()
	running := healthScan.running
	healthScan.mu.RUnlock()
	if running {
		c.JSON(http.StatusConflict, gin.H{"code": 1, "message": "巡检正在进行中，请稍后再试"})
		return
	}

	go runHealthScanTick(GetCurrentSettings())
	AddOpLogWithCtx(c, "refresh", "手动触发账号健康巡检", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "巡检已启动，可在设置页查看进度"})
}
