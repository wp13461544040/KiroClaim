package handler

import (
	"fmt"
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
	lastFailed  int
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
		// 启动前排空可能已积压的变更信号。
		// LoadSettingsFromEnv 在启动时就会触发一次配置变更通知，不清掉的话
		// 循环第一次 select 会直接走 wake 分支，跳过下面的启动延迟。
		select {
		case <-healthScanWake:
		default:
		}
		// 巡检循环必须常驻。加一层重启：单轮 panic 被兜底后循环继续，
		// 若连循环本身都挂了则重新拉起，不让服务失去状态维护能力。
		goSafe("health-scan-supervisor", func() {
			for {
				runSafe("health-scan-loop", healthScanLoop)
				log.Println("健康巡检循环异常退出，5 秒后重启")
				time.Sleep(5 * time.Second)
			}
		})
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
			now := time.Now()

			if s.HealthScanEnabled && inQuietHours(s, now) {
				// 静默时段不跑自动巡检，直接睡到时段结束。
				// 只推迟 nextRun，不动 base —— base 表示上次实际巡检的基准时间，
				// 把它挪到未来会让后续按间隔的重算全部偏移。
				resume := nextQuietEnd(s, now)
				log.Printf("健康巡检处于静默时段（北京时间 %02d:00-%02d:00），%s 恢复",
					s.HealthScanQuietStartHour, s.HealthScanQuietEndHour,
					resume.In(beijingZone).Format("01-02 15:04"))
				nextRun = resume
				continue
			}

			if s.HealthScanEnabled {
				// 单轮 panic 不应中断调度循环
				runSafe("health-scan-tick", func() { runHealthScanTick(s) })
			}
			// 以本轮结束时间为基准排下一轮，避免长时间巡检导致轮次堆叠。
			base = time.Now()
			nextRun = base.Add(scanInterval(s))

		case <-healthScanWake:
			// 设置变更只重算下次时间，不额外触发一轮巡检。
			timer.Stop()
			s := GetCurrentSettings()
			now := time.Now()
			if s.HealthScanEnabled && inQuietHours(s, now) {
				// 新配置下当前落在静默时段，睡到时段结束
				nextRun = nextQuietEnd(s, now)
				log.Printf("健康巡检配置已更新：当前处于静默时段，%s 恢复",
					nextRun.In(beijingZone).Format("01-02 15:04"))
				continue
			}
			recalculated := base.Add(scanInterval(s))
			if recalculated.Before(now) {
				// 距上次巡检已超过一个间隔（可能刚从静默时段出来），下个循环立即执行
				recalculated = now
			}
			// 配置变更不应把已排定的时间往后推
			if recalculated.Before(nextRun) {
				nextRun = recalculated
			}
			log.Printf("健康巡检配置已更新：下一轮 %s",
				nextRun.In(beijingZone).Format("01-02 15:04"))
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

// 静默时段固定按北京时间判断，不跟随服务器本地时区。
// 容器部署时系统时区通常是 UTC，用 time.Now().Hour() 会把时段整体算错 8 小时。
var beijingZone = time.FixedZone("CST", 8*60*60)

// inQuietHours 判断当前（北京时间）是否处于静默时段。
// 起止小时相同视为不静默。时段含起始小时、不含结束小时，
// 例如 0 和 8 表示 00:00:00 到 07:59:59 静默，08:00 开始工作。
func inQuietHours(s AppSettings, now time.Time) bool {
	start, end := s.HealthScanQuietStartHour, s.HealthScanQuietEndHour
	if start == end {
		return false
	}
	h := now.In(beijingZone).Hour()
	if start < end {
		return h >= start && h < end
	}
	// 跨午夜，例如 22 到 6
	return h >= start || h < end
}

// nextQuietEnd 返回静默时段结束的时刻（北京时间的整点）。
// 静默期内让调度直接睡到这个时刻，而不是继续按间隔空转，
// 这样时段一结束就能立刻开始巡检。
func nextQuietEnd(s AppSettings, now time.Time) time.Time {
	local := now.In(beijingZone)
	end := time.Date(local.Year(), local.Month(), local.Day(),
		s.HealthScanQuietEndHour, 0, 0, 0, beijingZone)
	if !end.After(local) {
		end = end.AddDate(0, 0, 1)
	}
	return end
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
	checked, flipped, failed, err := scanAccountsOnce(s, epoch)

	healthScan.mu.Lock()
	healthScan.running = false
	healthScan.lastEnded = time.Now()
	healthScan.lastChecked = checked
	healthScan.lastFlipped = flipped
	healthScan.lastFailed = failed
	if err != nil {
		healthScan.lastErr = err.Error()
	}
	started := healthScan.lastStarted
	healthScan.mu.Unlock()

	cost := time.Since(started).Round(time.Second)
	if err != nil {
		log.Printf("健康巡检结束(有错误): 成功 %d，未完成 %d，状态变更 %d，耗时 %s，错误: %v",
			checked, failed, flipped, cost, err)
		return
	}
	log.Printf("健康巡检结束: 成功 %d，未完成 %d，状态变更 %d，耗时 %s",
		checked, failed, flipped, cost)
	if checked > 0 || failed > 0 {
		detail := "定时巡检账号 " + strconv.Itoa(checked) + " 个，状态变更 " + strconv.Itoa(flipped) + " 个"
		if failed > 0 {
			detail += "，未完成 " + strconv.Itoa(failed) + " 个"
		}
		AddOpLog("refresh", detail, "system")
	}
}

// touchLastChecked 只推进 last_checked_at，不改动账号状态。
// 用于检查未能完成的场景：让该账号在排序中让位，避免卡住巡检队首。
func touchLastChecked(accountID uint) {
	database.DB.Model(&model.Account{}).Where("id = ?", accountID).
		Update("last_checked_at", time.Now())
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
func scanAccountsOnce(s AppSettings, epoch uint64) (int, int, int, error) {
	budget := s.HealthScanBatchSize
	if budget <= 0 {
		budget = 1000
	}

	candidates, err := healthScanCandidates(budget)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(candidates) == 0 {
		return 0, 0, 0, nil
	}

	// 巡检是后台任务，并发压到限流值的一半，给前台的提取和导入留出槽位。
	workers := currentUpstreamCheckConcurrency() / 2
	if workers < 1 {
		workers = 1
	}

	var checked, flipped, failed atomic.Int64
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

				account := acc
				// 单个账号异常不影响本轮其余账号
				runSafe("health-scan-account", func() {
					done := false
					defer func() {
						if done {
							return
						}
						// 检查没能走完（写库失败或 panic）时也要推进时间戳。
						// 候选是按 last_checked_at 升序取的，时间戳不动的账号会
						// 永久停在队首，每轮都占一个配额位；这类账号累积到每轮
						// 配额那么多时，排在后面的账号就再也轮不到检查了。
						touchLastChecked(account.ID)
						failed.Add(1)
					}()

					before := account.Status
					result := checkAccountHealth(account)
					if err := applyHealthResult(account.ID, result); err != nil {
						return
					}
					done = true
					checked.Add(1)
					if result.status != before {
						flipped.Add(1)
					}
				})
			}
		}()
	}

	for i := range candidates {
		jobs <- candidates[i]
	}
	close(jobs)
	wg.Wait()

	return int(checked.Load()), int(flipped.Load()), int(failed.Load()), nil
}

// HealthScanStatus 暴露巡检运行状态，供设置页展示。
func HealthScanStatus() map[string]interface{} {
	// 先取需要外部锁或数据库的数据，避免在 healthScan.mu 里嵌套加锁、跑查询
	s := GetCurrentSettings()
	now := time.Now()
	quiet := inQuietHours(s, now)
	pending := pendingHealthScanCount()

	healthScan.mu.RLock()
	running := healthScan.running
	lastChecked := healthScan.lastChecked
	lastFlipped := healthScan.lastFlipped
	lastFailed := healthScan.lastFailed
	lastErr := healthScan.lastErr
	lastStarted := healthScan.lastStarted
	lastEnded := healthScan.lastEnded
	healthScan.mu.RUnlock()

	status := map[string]interface{}{
		"running":      running,
		"lastChecked":  lastChecked,
		"lastFlipped":  lastFlipped,
		"lastFailed":   lastFailed,
		"lastError":    lastErr,
		"pendingTotal": pending,
		"inQuietHours": quiet,
	}
	if s.HealthScanQuietStartHour != s.HealthScanQuietEndHour {
		status["quietWindow"] = fmt.Sprintf("北京时间 %02d:00-%02d:00",
			s.HealthScanQuietStartHour, s.HealthScanQuietEndHour)
	}
	if quiet {
		status["quietResumeAt"] = nextQuietEnd(s, now)
	}
	if !lastStarted.IsZero() {
		status["lastStartedAt"] = lastStarted
	}
	if !lastEnded.IsZero() {
		status["lastEndedAt"] = lastEnded
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

	s := GetCurrentSettings()
	goSafe("health-scan-manual", func() { runHealthScanTick(s) })
	AddOpLogWithCtx(c, "refresh", "手动触发账号健康巡检", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "巡检已启动，可在设置页查看进度"})
}
