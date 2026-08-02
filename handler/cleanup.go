package handler

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"
)

// StartCleanupScheduler 启动定期清理任务
func StartCleanupScheduler() {
	// 启动时立即执行一次
	go cleanupUsedCreditAccounts()

	// 每小时执行一次
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			cleanupUsedCreditAccounts()
		}
	}()

	log.Println("[Cleanup] 定期清理任务已启动，每小时检查一次额度使用情况")
}

// cleanupUsedCreditAccounts 清理额度已被使用的未分配账号
func cleanupUsedCreditAccounts() {
	// 查找：未分配但额度已用的账号
	var accounts []model.Account
	err := database.DB.Where("used = ? AND credit_used > ? AND status = ?",
		false, 0, model.AccountStatusActive).Find(&accounts).Error

	if err != nil {
		log.Printf("[Cleanup] 查询额度已用账号失败: %v", err)
		return
	}

	if len(accounts) == 0 {
		log.Println("[Cleanup] 没有需要清理的账号")
		return
	}

	// 批量更新状态
	now := time.Now()
	cleanedCount := 0

	for _, account := range accounts {
		updates := map[string]interface{}{
			"status":  model.AccountStatusUsed,
			"used":    true,
			"used_at": now,
		}

		result := database.DB.Model(&model.Account{}).
			Where("id = ?", account.ID).
			Updates(updates)

		if result.Error != nil {
			log.Printf("[Cleanup] 更新账号 %d 状态失败: %v", account.ID, result.Error)
			continue
		}

		cleanedCount++
	}

	log.Printf("[Cleanup] 已清理 %d 个额度已用账号（总共发现 %d 个）", cleanedCount, len(accounts))

	// 记录操作日志
	if cleanedCount > 0 {
		AddOpLog("cleanup", "自动清理额度已用账号 "+strconv.Itoa(cleanedCount)+" 个", "system")
	}
}

// CleanupUsedCreditAccountsManual 手动触发清理（用于管理后台）
func CleanupUsedCreditAccountsManual() (int, error) {
	var accounts []model.Account
	err := database.DB.Where("used = ? AND credit_used > ? AND status = ?",
		false, 0, model.AccountStatusActive).Find(&accounts).Error

	if err != nil {
		return 0, err
	}

	if len(accounts) == 0 {
		return 0, nil
	}

	now := time.Now()
	cleanedCount := 0

	for _, account := range accounts {
		updates := map[string]interface{}{
			"status":  model.AccountStatusUsed,
			"used":    true,
			"used_at": now,
		}

		result := database.DB.Model(&model.Account{}).
			Where("id = ?", account.ID).
			Updates(updates)

		if result.Error != nil {
			continue
		}

		cleanedCount++
	}

	return cleanedCount, nil
}

// CleanupUsedCreditAccountsAPI 手动清理的 HTTP 处理函数
func CleanupUsedCreditAccountsAPI(c *gin.Context) {
	count, err := CleanupUsedCreditAccountsManual()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "清理失败: " + err.Error()})
		return
	}

	AddOpLogWithCtx(c, "cleanup", "手动清理额度已用账号 "+strconv.Itoa(count)+" 个", "admin")
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "清理完成",
		"data":    gin.H{"cleaned": count},
	})
}
