package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 全表扫描时每批读取的账号数量，控制单次驻留内存的实体数量。
const accountScanBatchSize = 100

// POST /admin/accounts/import
// Body: JSON 数组，支持直接数组或 accounts 数组。
// 流程：按 refreshToken 去重，健康检查通过后写入 active 账号。
func ImportAccounts(c *gin.Context) {
	var accounts []map[string]interface{}
	if err := c.ShouldBindJSON(&accounts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "JSON 格式错误: " + err.Error()})
		return
	}

	total := len(accounts)
	taskID := strconv.FormatInt(time.Now().UnixNano(), 36)

	// 初始化任务状态。
	importTasksMu.Lock()
	importTasks[taskID] = &ImportTask{
		ID:         taskID,
		Total:      total,
		Processed:  0,
		Imported:   0,
		SkippedDup: 0,
		SkippedBad: 0,
		Status:     "processing",
		StartTime:  time.Now(),
	}
	importTasksMu.Unlock()

	// 异步处理导入。
	go processImport(taskID, accounts)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "导入任务已启动",
		"data": gin.H{
			"taskId": taskID,
			"total":  total,
		},
	})
}

// GET /admin/accounts/import/status/:taskId
// 查询导入任务状态。
func ImportStatus(c *gin.Context) {
	taskID := c.Param("taskId")

	importTasksMu.RLock()
	task, exists := importTasks[taskID]
	importTasksMu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "任务不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"taskId":        task.ID,
			"status":        task.Status,
			"total":         task.Total,
			"processed":     task.Processed,
			"imported":      task.Imported,
			"skippedDup":    task.SkippedDup,
			"skippedBad":    task.SkippedBad,
			"badDetails":    task.BadDetails,
			"badDetailMore": task.BadDetailMore,
			"startTime":     task.StartTime,
			"endTime":       task.EndTime,
		},
	})
}

// 导入任务结构。
type ImportBadDetail struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

type ImportTask struct {
	ID            string
	Total         int
	Processed     int
	Imported      int
	SkippedDup    int
	SkippedBad    int
	BadDetails    []ImportBadDetail
	BadDetailMore int
	Status        string // processing, completed, failed
	StartTime     time.Time
	EndTime       *time.Time
}

var (
	importTasks   = make(map[string]*ImportTask)
	importTasksMu sync.RWMutex
)

const (
	importInsertBatchSize  = 25
	importBadDetailMaxSize = 100
)

type importCheckResult struct {
	account model.Account
	row     int
	reason  string
	bad     bool
}

type importCandidate struct {
	row     int
	account model.Account
}

func addImportProgress(task *ImportTask, processed, imported, skippedDup, skippedBad int) {
	importTasksMu.Lock()
	task.Processed += processed
	task.Imported += imported
	task.SkippedDup += skippedDup
	task.SkippedBad += skippedBad
	importTasksMu.Unlock()
}

func addImportBadDetail(task *ImportTask, row int, reason string) {
	importTasksMu.Lock()
	if len(task.BadDetails) < importBadDetailMaxSize {
		task.BadDetails = append(task.BadDetails, ImportBadDetail{
			Row:    row,
			Reason: reason,
		})
	} else {
		task.BadDetailMore++
	}
	importTasksMu.Unlock()
}

// 分批扫描已有 refreshToken，避免账号量大时一次性把全表实体读进内存。
func loadExistingRefreshTokens() (map[string]struct{}, error) {
	tokens := make(map[string]struct{})
	var batch []model.Account
	err := database.DB.Model(&model.Account{}).Select("id", "refresh_token").
		FindInBatches(&batch, accountScanBatchSize, func(tx *gorm.DB, _ int) error {
			for i := range batch {
				if batch[i].RefreshToken != "" {
					tokens[batch[i].RefreshToken] = struct{}{}
				}
			}
			return nil
		}).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

func applyImportHealthResult(acc *model.Account, result healthResult) bool {
	if result.status != model.AccountStatusActive || result.errMsg != "" {
		return false
	}

	now := time.Now()
	acc.Status = result.status
	acc.LastCheckedAt = &now
	if result.newToken != "" {
		acc.AccessToken = result.newToken
	}
	if result.newRefresh != "" {
		acc.RefreshToken = result.newRefresh
	}
	if result.provider != "" {
		acc.Provider = result.provider
	}
	if result.email != "" {
		acc.Email = result.email
	}
	if result.subscription != "" {
		acc.Subscription = result.subscription
	}
	if result.creditLimit > 0 {
		acc.CreditUsed = result.creditUsed
		acc.CreditLimit = result.creditLimit
	}
	return true
}

func flushImportBatch(task *ImportTask, batch []model.Account) {
	if len(batch) == 0 {
		return
	}

	if err := database.DB.CreateInBatches(&batch, importInsertBatchSize).Error; err == nil {
		addImportProgress(task, 0, len(batch), 0, 0)
		return
	}

	// 批量写入失败时降级为单条写入，尽量保住其它有效账号。
	for _, acc := range batch {
		if err := database.DB.Create(&acc).Error; err != nil {
			addImportProgress(task, 0, 0, 0, 1)
			continue
		}
		addImportProgress(task, 0, 1, 0, 0)
	}
}

// 异步处理导入：查重前置、健康检查并发、数据库批量写入。
func processImport(taskID string, accounts []map[string]interface{}) {
	importTasksMu.RLock()
	task := importTasks[taskID]
	importTasksMu.RUnlock()

	// 导入只建立流水线 worker，真正的上游请求并发由全局检查池统一控制。
	//
	// worker 数要高于限流值：worker 在解析结果、等数据库写入时并不占用上游槽位，
	// 如果两者相等，每有一个 worker 在做本地工作就有一个上游槽位空转。
	// 超发几倍让槽位始终有人排队，真实上游并发仍由限流器封顶。
	limit := currentUpstreamCheckConcurrency()
	if limit <= 0 {
		limit = 6
	}
	concurrency := limit * 3
	if concurrency > 64 {
		concurrency = 64
	}

	existingTokens, err := loadExistingRefreshTokens()
	if err != nil {
		importTasksMu.Lock()
		now := time.Now()
		task.Status = "failed"
		task.EndTime = &now
		importTasksMu.Unlock()
		return
	}

	// 批内去重和库内去重前置，避免每个账号都做一次 COUNT 查询。
	seenTokens := make(map[string]struct{})
	candidates := make([]importCandidate, 0, len(accounts))
	for i, a := range accounts {
		row := i + 1
		refreshToken := strVal(a, "refreshToken")
		if refreshToken == "" {
			addImportProgress(task, 1, 0, 0, 1)
			addImportBadDetail(task, row, "缺少 refreshToken")
			continue
		}
		if _, dup := seenTokens[refreshToken]; dup {
			addImportProgress(task, 1, 0, 1, 0)
			continue
		}
		if _, dup := existingTokens[refreshToken]; dup {
			addImportProgress(task, 1, 0, 1, 0)
			continue
		}
		seenTokens[refreshToken] = struct{}{}

		candidates = append(candidates, importCandidate{
			row: row,
			account: model.Account{
				AccessToken:  strVal(a, "accessToken"),
				RefreshToken: refreshToken,
				ClientId:     strVal(a, "clientId"),
				ClientSecret: strVal(a, "clientSecret"),
				Provider:     strVal(a, "provider"),
				Region:       strVal(a, "region"),
			},
		})
	}

	jobs := make(chan importCandidate)
	results := make(chan importCheckResult, len(candidates))
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				acc := job.account
				result := checkAccountHealth(acc)
				if !applyImportHealthResult(&acc, result) {
					reason := result.errMsg
					if reason == "" {
						reason = "账号健康检查未通过"
					}
					results <- importCheckResult{row: job.row, reason: reason, bad: true}
					continue
				}
				results <- importCheckResult{account: acc, row: job.row}
			}
		}()
	}

	go func() {
		for _, candidate := range candidates {
			jobs <- candidate
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	insertBatch := make([]model.Account, 0, importInsertBatchSize)
	for result := range results {
		if result.bad {
			addImportProgress(task, 1, 0, 0, 1)
			addImportBadDetail(task, result.row, result.reason)
			continue
		}
		addImportProgress(task, 1, 0, 0, 0)
		insertBatch = append(insertBatch, result.account)
		if len(insertBatch) >= importInsertBatchSize {
			flushImportBatch(task, insertBatch)
			insertBatch = make([]model.Account, 0, importInsertBatchSize)
		}
	}
	flushImportBatch(task, insertBatch)

	// 标记任务完成。
	importTasksMu.Lock()
	now := time.Now()
	task.Status = "completed"
	task.EndTime = &now
	importTasksMu.Unlock()

	// 记录操作日志。
	AddOpLog("import", "导入账号：总计 "+strconv.Itoa(task.Total)+
		"，成功 "+strconv.Itoa(task.Imported)+
		"，重复 "+strconv.Itoa(task.SkippedDup)+
		"，失败 "+strconv.Itoa(task.SkippedBad), "admin")

	// 30 分钟后清理任务记录。
	time.AfterFunc(30*time.Minute, func() {
		importTasksMu.Lock()
		delete(importTasks, taskID)
		importTasksMu.Unlock()
	})
}

// GET /admin/accounts?page=1&size=20&status=active&used=false&keyword=xxx&subscription=KIRO%20FREE
func ListAccounts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	statusFilter := c.Query("status")
	usedFilter := c.Query("used")
	keyword := c.Query("keyword")
	subscriptionFilter := c.Query("subscription")
	createdFrom := c.Query("created_from")
	createdTo := c.Query("created_to")
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 1000 {
		size = 1000
	}
	var total int64
	
	// 列表专用结构，只查询展示需要的列，避免加载 token 等大字段。
	// 字段名与 model.Account 保持一致，序列化后的 JSON key 不变，前端无需调整。
	type AccountListItem struct {
		ID            uint
		CreatedAt     time.Time
		Email         string
		Status        model.AccountStatus
		Subscription  string
		CreditUsed    float64
		CreditLimit   float64
		Used          bool
		UsedAt        *time.Time
		Provider      string
		Region        string
		LastCheckedAt *time.Time
	}
	
	var accounts []AccountListItem
	q := database.DB.Model(&model.Account{}).Select(
		"id", "email", "status", "subscription", "credit_used", "credit_limit",
		"used", "used_at", "provider", "region", "last_checked_at", "created_at",
	)

	// 按健康状态筛选。
	if statusFilter != "" {
		q = q.Where("status = ?", statusFilter)
	}
	// 按使用状态筛选。
	if usedFilter == "true" {
		q = q.Where("used = ?", true)
	} else if usedFilter == "false" {
		q = q.Where("used = ?", false)
	}
	// 按订阅筛选。
	if subscriptionFilter != "" {
		q = q.Where("subscription = ?", subscriptionFilter)
	}
	// 按关键词搜索邮箱。
	if keyword != "" {
		q = q.Where("email LIKE ?", "%"+keyword+"%")
	}
	if createdFrom != "" {
		if t, err := time.Parse("2006-01-02", createdFrom); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if createdTo != "" {
		if t, err := time.Parse("2006-01-02", createdTo); err == nil {
			q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}

	q.Count(&total)
	q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&accounts)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"total": total, "page": page, "size": size, "list": accounts,
	}})
}

// DELETE /admin/accounts/:id
func DeleteAccount(c *gin.Context) {
	id := c.Param("id")
	accountID64, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的 ID"})
		return
	}
	result := deleteAccountsPhysically([]uint{uint(accountID64)})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}
	AddOpLogWithCtx(c, "delete", "删除账号 ID:"+id, "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除"})
}

// POST /admin/accounts/batch-delete
// Body: { "ids": [1, 2, 3] }
func BatchDeleteAccounts(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请选择要删除的账号"})
		return
	}
	result := deleteAccountsPhysically(req.IDs)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}
	AddOpLogWithCtx(c, "delete", "批量删除账号 "+strconv.Itoa(len(req.IDs))+" 个，实际删除 "+strconv.FormatInt(result.RowsAffected, 10)+" 个", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除", "data": gin.H{"deleted": result.RowsAffected}})
}

// POST /admin/accounts/delete-by-status
// Body: { "status": "suspended" }
// 按状态批量删除账号，常用于清理封禁账号。
// 只删除未分配（used = false）的账号，已分配账号不受影响。
// 删除前会先刷新所有未分配账号的健康状态，确保基于最新状态执行删除。
func DeleteAccountsByStatus(c *gin.Context) {
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}
	validStatus := map[string]bool{"suspended": true}
	if !validStatus[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "只允许删除 suspended 状态"})
		return
	}

	// 并发刷新未分配账号状态，已分配账号不处理。
	// 使用分批扫描，避免账号量大时把全表实体一次性读进内存。
	concurrency := currentUpstreamCheckConcurrency()
	if concurrency <= 0 {
		concurrency = 6
	}

	jobs := make(chan model.Account, concurrency*2)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range jobs {
				result := checkAccountHealth(account)
				_ = applyHealthResult(account.ID, result)
			}
		}()
	}

	var batch []model.Account
	scanErr := database.DB.Where("used = ?", false).
		FindInBatches(&batch, accountScanBatchSize, func(tx *gorm.DB, _ int) error {
			for i := range batch {
				jobs <- batch[i]
			}
			return nil
		}).Error
	close(jobs)
	wg.Wait()

	if scanErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "查询账号失败: " + scanErr.Error()})
		return
	}

	// 刷新完成后，查询最新的封禁账号并删除（只删除未分配的）
	result := database.DB.Unscoped().Where("status = ? AND used = ?", req.Status, false).Delete(&model.Account{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}
	AddOpLogWithCtx(c, "delete", "批量删除未分配的 "+req.Status+" 状态账号 "+strconv.FormatInt(result.RowsAffected, 10)+" 个（已刷新状态）", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除", "data": gin.H{"deleted": result.RowsAffected}})
}

// POST /admin/accounts/clear-all
// Body: { "confirm": true }
// 清空号池中所有账号（硬删除），需要 confirm=true。
func ClearAllAccounts(c *gin.Context) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请确认清空操作（confirm: true）"})
		return
	}

	result := database.DB.Unscoped().Where("1 = 1").Delete(&model.Account{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}

	AddOpLogWithCtx(c, "clear", "清空号池，共删除 "+strconv.FormatInt(result.RowsAffected, 10)+" 个账号", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "号池已清空", "data": gin.H{"deleted": result.RowsAffected}})
}

// POST /admin/accounts/clear-assigned
// Body: { "confirm": true }
// 清空所有已分配账号（used=true），需要 confirm=true。
func ClearAssignedAccounts(c *gin.Context) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请确认清空操作（confirm: true）"})
		return
	}

	var ids []uint
	database.DB.Model(&model.Account{}).Where("used = ?", true).Pluck("id", &ids)
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已分配账号已清空", "data": gin.H{"deleted": 0}})
		return
	}
	result := deleteAccountsPhysically(ids)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}

	AddOpLogWithCtx(c, "clear", "清空已分配账号，共删除 "+strconv.FormatInt(result.RowsAffected, 10)+" 个", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已分配账号已清空", "data": gin.H{"deleted": result.RowsAffected}})
}

// GET /admin/pool/stats
func PoolStats(c *gin.Context) {
	// 账号统计：单次 GROUP BY 查询替代多次 COUNT。
	type statusCount struct {
		Status string
		Used   bool
		Count  int64
	}
	var statusCounts []statusCount
	database.DB.Model(&model.Account{}).Select("status, used, count(*) as count").Group("status, used").Find(&statusCounts)

	var total, unused, used, available int64
	var statusActive, statusSuspended, statusUsed int64
	for _, sc := range statusCounts {
		total += sc.Count
		if sc.Used {
			used += sc.Count
		} else {
			unused += sc.Count
			if sc.Status == string(model.AccountStatusActive) {
				available += sc.Count
			}
		}
		// 按状态分类统计
		switch model.AccountStatus(sc.Status) {
		case model.AccountStatusActive:
			statusActive += sc.Count
		case model.AccountStatusSuspended:
			statusSuspended += sc.Count
		case model.AccountStatusUsed:
			statusUsed += sc.Count
		}
	}

	var accountSubscriptions []string
	database.DB.Model(&model.Account{}).Pluck("subscription", &accountSubscriptions)
	subscriptionCounts := make(map[string]int64)
	for _, subscription := range accountSubscriptions {
		subscription = strings.TrimSpace(subscription)
		if subscription == "" {
			continue
		}
		subscriptionCounts[subscription]++
	}
	subscriptionDist := make([]gin.H, 0, len(subscriptionCounts))
	for subscription, count := range subscriptionCounts {
		subscriptionDist = append(subscriptionDist, gin.H{"subscription": subscription, "count": count})
	}
	sort.SliceStable(subscriptionDist, func(i, j int) bool {
		ci, _ := subscriptionDist[i]["count"].(int64)
		cj, _ := subscriptionDist[j]["count"].(int64)
		if ci != cj {
			return ci > cj
		}
		return subscriptionDist[i]["subscription"].(string) < subscriptionDist[j]["subscription"].(string)
	})

	var cardTotal, cardUnused int64
	database.DB.Model(&model.Card{}).Count(&cardTotal)
	database.DB.Model(&model.Card{}).Where("used_at IS NULL").Count(&cardUnused)
	cardActive := cardTotal - cardUnused

	// 多号卡统计。
	var multiCardTotal int64
	database.DB.Model(&model.Card{}).Where("account_count > 1").Count(&multiCardTotal)

	// 今日统计：单次 GROUP BY。
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
	type actionCount struct {
		Action string
		Count  int64
	}
	var todayCounts []actionCount
	database.DB.Model(&model.OpLog{}).Select("action, count(*) as count").
		Where("action = ? AND created_at >= ?", "activate", todayStart).
		Group("action").Find(&todayCounts)
	var todayActivate int64
	for _, tc := range todayCounts {
		if tc.Action == "activate" {
			todayActivate = tc.Count
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"accounts": gin.H{
			"total":     total,
			"unused":    unused,
			"used":      used,
			"available": available,
			"status": gin.H{
				"active":    statusActive,
				"suspended": statusSuspended,
				"used":      statusUsed,
			},
			"subscriptions": subscriptionDist,
		},
		"cards": gin.H{
			"total":  cardTotal,
			"unused": cardUnused,
			"active": cardActive,
			"multi":  multiCardTotal,
		},
		"today": gin.H{
			"activate": todayActivate,
		},
	}})
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GET /admin/accounts/subscription-stats
// 基于账号表中实际存在的 subscription 动态聚合，用于账号列表订阅筛选。
func AccountSubscriptionStats(c *gin.Context) {
	type SubscriptionStat struct {
		Subscription string `json:"subscription"`
		UnusedCount  int64  `json:"unusedCount"`
		TotalCount   int64  `json:"totalCount"`
	}

	// 交由数据库做 GROUP BY 聚合，避免把全表读进内存统计。
	stats := make([]SubscriptionStat, 0, 8)
	if err := database.DB.Model(&model.Account{}).
		Select("subscription, COUNT(*) AS total_count, SUM(CASE WHEN used = ? AND status = ? THEN 1 ELSE 0 END) AS unused_count",
			false, model.AccountStatusActive).
		Where("subscription != ''").
		Group("subscription").
		Scan(&stats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "统计失败: " + err.Error()})
		return
	}

	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].UnusedCount != stats[j].UnusedCount {
			return stats[i].UnusedCount > stats[j].UnusedCount
		}
		if stats[i].TotalCount != stats[j].TotalCount {
			return stats[i].TotalCount > stats[j].TotalCount
		}
		return stats[i].Subscription < stats[j].Subscription
	})

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": stats})
}
