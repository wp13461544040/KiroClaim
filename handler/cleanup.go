package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"
)

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
