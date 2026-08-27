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
// 只清理未分配（used = false）的账号，已分配账号不受影响。
// 使用单条批量 UPDATE，避免逐个账号读取再更新。
func CleanupUsedCreditAccountsManual() (int, error) {
	result := database.DB.Model(&model.Account{}).
		Where("used = ? AND credit_used > ? AND status = ?", false, 0, model.AccountStatusActive).
		Updates(map[string]interface{}{
			"status":  model.AccountStatusUsed,
			"used":    true,
			"used_at": time.Now(),
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
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
