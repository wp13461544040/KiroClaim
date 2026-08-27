package handler

import (
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

// 导出单批读取的行数。凭证字段较大，批次不宜过大。
const exportScanBatchSize = 200

// GET /admin/accounts/export
// 查询参数与 ListAccounts 一致，额外支持：
//   - ids=1,2,3   只导出指定账号
//   - limit=1000  最多导出多少条
//
// 与列表接口分开的原因：
//  1. 列表接口刻意不返回 access_token / refresh_token / client_secret，
//     导出必须带上这些字段，两者不能共用同一个投影。
//  2. 导出用 keyset 分页流式写 response，不做 COUNT、不在内存里堆全量数据，
//     账号量大时内存占用与账号总数无关。
func ExportAccounts(c *gin.Context) {
	q := database.DB.Model(&model.Account{})

	// 指定 ID 导出优先，忽略其他筛选条件。
	idList := parseExportIDs(c.Query("ids"))
	if len(idList) > 0 {
		q = q.Where("id IN ?", idList)
	} else {
		q = applyExportFilters(c, q)
	}

	limit := 0
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = v
	}

	filename := "accounts_" + time.Now().Format("2006-01-02") + ".json"
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)

	writer := c.Writer
	enc := json.NewEncoder(writer)

	if _, err := writer.WriteString("["); err != nil {
		return
	}

	written := 0
	lastID := uint(0)
	aborted := false

	for {
		if aborted {
			break
		}

		batchSize := exportScanBatchSize
		if limit > 0 && limit-written < batchSize {
			batchSize = limit - written
		}
		if batchSize <= 0 {
			break
		}

		// keyset 分页：按 id 递增推进，避免深分页 OFFSET 逐行丢弃。
		var batch []model.Account
		tx := q.Session(&gorm.Session{}).Where("id > ?", lastID).
			Order("id ASC").Limit(batchSize)
		if err := tx.Find(&batch).Error; err != nil || len(batch) == 0 {
			break
		}

		for i := range batch {
			acc := batch[i]
			lastID = acc.ID

			if written > 0 {
				if _, err := writer.WriteString(","); err != nil {
					aborted = true
					break
				}
			}
			// AfterFind 已把凭证解密回明文，这里直接写出，与导入格式保持一致。
			item := map[string]string{
				"accessToken":  acc.AccessToken,
				"refreshToken": acc.RefreshToken,
				"clientId":     acc.ClientId,
				"clientSecret": acc.ClientSecret,
			}
			if err := enc.Encode(item); err != nil {
				aborted = true
				break
			}
			written++
		}

		// 拿到的行数少于批次容量说明已到末尾。
		if len(batch) < batchSize {
			break
		}
	}

	writer.WriteString("]")
	writer.Flush()

	scope := "全部"
	if len(idList) > 0 {
		scope = "选中"
	}
	AddOpLogWithCtx(c, "export", "导出"+scope+"账号 "+strconv.Itoa(written)+" 个", "admin")
}

// 复用与 ListAccounts 相同的筛选语义，保证导出结果与列表所见一致。
func applyExportFilters(c *gin.Context, q *gorm.DB) *gorm.DB {
	switch c.Query("used") {
	case "true":
		q = q.Where("used = ?", true)
	case "false":
		q = q.Where("used = ?", false)
	}
	if v := c.Query("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if v := c.Query("subscription"); v != "" {
		q = q.Where("subscription = ?", v)
	}
	if v := c.Query("keyword"); v != "" {
		q = q.Where("email LIKE ?", "%"+v+"%")
	}
	if v := c.Query("created_from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if v := c.Query("created_to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}
	return q
}

func parseExportIDs(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]uint, 0, len(parts))
	seen := make(map[uint]struct{}, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64)
		if err != nil || v == 0 {
			continue
		}
		id := uint(v)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
