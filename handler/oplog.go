package handler

import (
	"net/http"
	"strconv"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"

	"github.com/gin-gonic/gin"
)

const maxUserAgentLen = 255

func AddOpLog(action, detail, operator string) {
	writeOpLog(action, detail, operator, "", "")
}

func AddOpLogWithCtx(c *gin.Context, action, detail, operator string) {
	ip, ua := "", ""
	if c != nil {
		ip = c.ClientIP()
		ua = c.GetHeader("User-Agent")
		if len(ua) > maxUserAgentLen {
			ua = ua[:maxUserAgentLen]
		}
	}
	writeOpLog(action, detail, operator, ip, ua)
}

func writeOpLog(action, detail, operator, ip, ua string) {
	entry := model.OpLog{
		Action:    action,
		Detail:    detail,
		Operator:  operator,
		ClientIP:  ip,
		UserAgent: ua,
	}
	database.DB.Create(&entry)
}

func ListOpLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	actionFilter := c.Query("action")
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	var total int64
	var logs []model.OpLog
	q := database.DB.Model(&model.OpLog{})
	if actionFilter != "" {
		q = q.Where("action = ?", actionFilter)
	}
	q.Count(&total)
	q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&logs)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"total": total, "page": page, "size": size, "list": logs,
	}})
}

// BatchDeleteOpLogs 批量删除操作日志
func BatchDeleteOpLogs(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请选择要删除的日志"})
		return
	}

	result := database.DB.Where("id IN ?", req.IDs).Delete(&model.OpLog{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}

	AddOpLogWithCtx(c, "delete", "批量删除操作日志 "+strconv.Itoa(len(req.IDs))+" 条，实际删除 "+strconv.FormatInt(result.RowsAffected, 10)+" 条", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除", "data": gin.H{"deleted": result.RowsAffected}})
}

// ClearOpLogs 清空操作日志
func ClearOpLogs(c *gin.Context) {
	result := database.DB.Where("1=1").Delete(&model.OpLog{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}

	// 注意：清空日志后，这条日志本身也会被记录
	AddOpLogWithCtx(c, "clear", "清空操作日志，共删除 "+strconv.FormatInt(result.RowsAffected, 10)+" 条", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "日志已清空", "data": gin.H{"deleted": result.RowsAffected}})
}
