package handler

import (
	"net/http"
	"strconv"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

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
