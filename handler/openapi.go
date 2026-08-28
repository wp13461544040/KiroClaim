package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func jsonUnmarshal(data []byte, v interface{}) error { return json.Unmarshal(data, v) }

// 对外机器对接接口（/openapi）。
//
// 面向外部程序，例如批量注册脚本：注册完账号后投递进号池，并按库存决定是否继续注册。
// 用独立的 API Key 鉴权而不是复用管理员 JWT，这样对接方拿不到删除账号、
// 导出明文凭证、修改系统配置的能力。
//
// 有意不提供的能力：查询账号明细、导出凭证、删除或清空号池。

// POST /openapi/v1/accounts
// Body: {"accounts":[{"refreshToken":"...","accessToken":"...","clientId":"...","clientSecret":"..."}]}
// 也接受顶层直接是数组的形式。
//
// 行为与后台导入一致：按 refreshToken 去重、逐个健康检查、通过的写入号池。
// 立即返回 taskId，导入在后台进行，用 GET /openapi/v1/accounts/import/:taskId 查进度。
func OpenAPIImportAccounts(c *gin.Context) {
	var accounts []map[string]interface{}

	// 先按 {"accounts":[...]} 解析，失败再退回顶层数组，兼容两种投递格式
	var wrapped struct {
		Accounts []map[string]interface{} `json:"accounts"`
	}
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "读取请求体失败"})
		return
	}
	if err := jsonUnmarshal(body, &wrapped); err == nil && len(wrapped.Accounts) > 0 {
		accounts = wrapped.Accounts
	} else if err := jsonUnmarshal(body, &accounts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "JSON 格式错误，请提交 {\"accounts\":[...]} 或直接提交数组",
		})
		return
	}

	if len(accounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "accounts 不能为空"})
		return
	}
	if len(accounts) > openAPIImportMaxBatch {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "单次最多提交 " + strconv.Itoa(openAPIImportMaxBatch) + " 个账号，请分批投递",
		})
		return
	}

	total := len(accounts)
	taskID := strconv.FormatInt(time.Now().UnixNano(), 36)

	importTasksMu.Lock()
	importTasks[taskID] = &ImportTask{
		ID:        taskID,
		Total:     total,
		Status:    "processing",
		StartTime: time.Now(),
	}
	importTasksMu.Unlock()

	goSafe("openapi-import:"+taskID, func() { processImport(taskID, accounts) })

	AddOpLogWithCtx(c, "import", "对接接口提交导入 "+strconv.Itoa(total)+" 个账号", "openapi")
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "导入任务已创建",
		"data": gin.H{
			"taskId": taskID,
			"total":  total,
		},
	})
}

// 单次投递上限。导入是串行执行的，批次太大会长时间占用导入队列。
const openAPIImportMaxBatch = 500

// GET /openapi/v1/accounts/import/:taskId
// 查询导入进度。任务记录在完成 30 分钟后清理。
func OpenAPIImportStatus(c *gin.Context) {
	taskID := c.Param("taskId")

	importTasksMu.RLock()
	task, exists := importTasks[taskID]
	importTasksMu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "任务不存在或已过期"})
		return
	}

	importTasksMu.RLock()
	data := gin.H{
		"taskId":     task.ID,
		"status":     task.Status,
		"total":      task.Total,
		"processed":  task.Processed,
		"imported":   task.Imported,
		"skippedDup": task.SkippedDup,
		"skippedBad": task.SkippedBad,
		"badDetails": task.BadDetails,
		"startTime":  task.StartTime,
		"endTime":    task.EndTime,
	}
	importTasksMu.RUnlock()

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}

// GET /openapi/v1/stock
// 库存查询。只返回聚合数量，不含任何账号明细或凭证。
// 注册程序可据此决定是否继续注册。
//
// 可选参数 subscription：只统计指定订阅。
func OpenAPIStock(c *gin.Context) {
	subscription := strings.TrimSpace(c.Query("subscription"))

	type subscriptionStock struct {
		Subscription string `json:"subscription"`
		Available    int64  `json:"available"`
		Total        int64  `json:"total"`
	}

	base := func() *gorm.DB {
		q := database.DB.Model(&model.Account{})
		if subscription != "" {
			q = q.Where("subscription = ?", subscription)
		}
		return q
	}

	// 可用 = 未分配 且 状态正常 且 额度未消耗，与派发时的筛选条件保持一致
	var available, total, suspended, assigned int64
	base().Where("used = ? AND status = ? AND credit_used = ?",
		false, model.AccountStatusActive, 0).Count(&available)
	base().Count(&total)
	base().Where("status = ?", model.AccountStatusSuspended).Count(&suspended)
	base().Where("used = ?", true).Count(&assigned)

	data := gin.H{
		"available": available,
		"total":     total,
		"assigned":  assigned,
		"suspended": suspended,
	}

	// 未指定订阅时按订阅维度拆分，交给数据库聚合
	if subscription == "" {
		byType := make([]subscriptionStock, 0, 8)
		if err := database.DB.Model(&model.Account{}).
			Select("subscription, "+
				"SUM(CASE WHEN used = ? AND status = ? AND credit_used = ? THEN 1 ELSE 0 END) AS available, "+
				"COUNT(*) AS total",
				false, model.AccountStatusActive, 0).
			Where("subscription != ''").
			Group("subscription").
			Scan(&byType).Error; err == nil {
			data["bySubscription"] = byType
		}
	} else {
		data["subscription"] = subscription
	}

	// 巡检信息帮助对接方判断库存数字的新鲜度
	s := GetCurrentSettings()
	scan := gin.H{
		"enabled":         s.HealthScanEnabled,
		"intervalMinutes": s.HealthScanIntervalMinutes,
	}
	healthScan.mu.RLock()
	if !healthScan.lastEnded.IsZero() {
		scan["lastFinishedAt"] = healthScan.lastEnded
	}
	scan["running"] = healthScan.running
	healthScan.mu.RUnlock()
	data["healthScan"] = scan

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}

// POST /admin/settings/openapi-key
// 生成一个新的 API Key 并立即生效。明文只在本次响应里返回一次。
func GenerateOpenAPIKey(c *gin.Context) {
	key, err := randomAPIKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "生成失败: " + err.Error()})
		return
	}

	settingsMu.RLock()
	s := currentSettings
	settingsMu.RUnlock()

	s.OpenAPIKey = key
	if err := persistRuntimeSettings(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "保存失败: " + err.Error()})
		return
	}

	settingsMu.Lock()
	currentSettings = s
	settingsMu.Unlock()
	updateSecuritySettings(s)

	AddOpLogWithCtx(c, "settings", "生成新的对接 API Key", "admin")
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "已生成新 Key，请立即保存，页面刷新后不再显示",
		"data":    gin.H{"apiKey": key},
	})
}

func randomAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "kc_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
