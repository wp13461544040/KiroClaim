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
//
// 清理条件（需同时满足）：
//  1. used = false（未分配）
//  2. status = active（状态正常）
//  3. credit_used > 0（已消耗额度）
func CleanupUsedCreditAccountsManual() (int, error) {
	result := database.DB.Model(&model.Account{}).
		Where("used = ? AND status = ?", false, model.AccountStatusActive).
		Where("credit_used > ?", 0).
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
	// 库存模式的整个用意就是把用过额度的账号留在池里，
	// 这个清理会一次性把它们全部推走，语义直接冲突，所以直接拒绝。
	if inventoryModeEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "库存模式已开启，额度已用账号会保留在账号池，无法执行清理。如需清理请先在设置中关闭库存模式"})
		return
	}

	count, err := CleanupUsedCreditAccountsManual()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "清理失败: " + err.Error()})
		return
	}

	AddOpLogWithCtx(c, "cleanup", "手动清理使用过额度的账号 "+strconv.Itoa(count)+" 个", "admin")
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "清理完成",
		"data":    gin.H{"cleaned": count},
	})
}
