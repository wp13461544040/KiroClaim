package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"
)

// setupCardDeleteTestDB 准备一个临时库，并在测试结束后还原全局 DB。
func setupCardDeleteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := database.DB
	t.Cleanup(func() { database.DB = oldDB })

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "app.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(&model.Card{}, &model.Account{}, &model.CardAccount{}, &model.OpLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.DB = db
	return db
}

// seedCardWithAccount 创建一张卡密、一个账号以及两者的绑定关系。
func seedCardWithAccount(t *testing.T, code string) (model.Card, model.Account) {
	t.Helper()
	usedAt := time.Now()
	card := model.Card{Code: code, UsedAt: &usedAt, AccountCount: 1}
	if err := database.DB.Create(&card).Error; err != nil {
		t.Fatalf("create card: %v", err)
	}
	account := model.Account{
		AccessToken:  "access-" + code,
		RefreshToken: "refresh-" + code,
		Status:       model.AccountStatusActive,
		Used:         true,
		UsedAt:       &usedAt,
	}
	if err := database.DB.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	return card, account
}

// countAccountRows 用 Unscoped 统计物理存在的行数，软删除的行也会被计入。
func countAccountRows(t *testing.T, accountID uint) int64 {
	t.Helper()
	var n int64
	if err := database.DB.Unscoped().Model(&model.Account{}).Where("id = ?", accountID).Count(&n).Error; err != nil {
		t.Fatalf("count account: %v", err)
	}
	return n
}

func decodeDeletedAccounts(t *testing.T, body []byte) float64 {
	t.Helper()
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DeletedAccounts float64 `json:"deletedAccounts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v, body=%s", err, body)
	}
	if resp.Code != 0 {
		t.Fatalf("response code = %d, want 0, body=%s", resp.Code, body)
	}
	return resp.Data.DeletedAccounts
}

// 删除卡密时绑定的账号必须被物理删除，而不是只置 deleted_at。
// 只置 deleted_at 会让账号从列表和库存统计中消失，却仍被一键清空计数。
func TestDeleteCardPhysicallyRemovesBoundAccount(t *testing.T) {
	setupCardDeleteTestDB(t)
	card, account := seedCardWithAccount(t, "DELETE-ONE")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(card.ID), 10)}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/admin/cards/"+strconv.FormatUint(uint64(card.ID), 10), nil)

	DeleteCard(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if got := decodeDeletedAccounts(t, recorder.Body.Bytes()); got != 1 {
		t.Fatalf("deletedAccounts = %v, want 1", got)
	}
	if n := countAccountRows(t, account.ID); n != 0 {
		t.Fatalf("账号仍物理存在，行数 = %d，want 0（说明漏了 Unscoped）", n)
	}

	var bindings int64
	if err := database.DB.Model(&model.CardAccount{}).Where("card_id = ?", card.ID).Count(&bindings).Error; err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if bindings != 0 {
		t.Fatalf("绑定关系残留 %d 条，want 0", bindings)
	}
}

func TestBatchDeleteCardsPhysicallyRemovesBoundAccounts(t *testing.T) {
	setupCardDeleteTestDB(t)
	cardA, accountA := seedCardWithAccount(t, "BATCH-A")
	cardB, accountB := seedCardWithAccount(t, "BATCH-B")

	payload, err := json.Marshal(map[string][]uint{"ids": {cardA.ID, cardB.ID}})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/cards/batch-delete", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")

	BatchDeleteCards(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if got := decodeDeletedAccounts(t, recorder.Body.Bytes()); got != 2 {
		t.Fatalf("deletedAccounts = %v, want 2", got)
	}
	for _, id := range []uint{accountA.ID, accountB.ID} {
		if n := countAccountRows(t, id); n != 0 {
			t.Fatalf("账号 %d 仍物理存在，行数 = %d，want 0", id, n)
		}
	}
}

// 清理接口：不带 confirm 只报数，带 confirm 才真删。
func TestPurgeSoftDeletedAccounts(t *testing.T) {
	setupCardDeleteTestDB(t)
	_, ghost := seedCardWithAccount(t, "GHOST")
	// 模拟历史遗留：不带 Unscoped 的删除只会把 deleted_at 置值
	if err := database.DB.Where("id = ?", ghost.ID).Delete(&model.Account{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if n := countAccountRows(t, ghost.ID); n != 1 {
		t.Fatalf("前置条件不成立：软删除后行数 = %d，want 1", n)
	}

	callPurge := func(body string) (int64, int64) {
		t.Helper()
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/purge-soft-deleted", bytes.NewBufferString(body))
		ctx.Request.Header.Set("Content-Type", "application/json")

		PurgeSoftDeletedAccounts(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Pending int64 `json:"pending"`
				Deleted int64 `json:"deleted"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Code != 0 {
			t.Fatalf("code = %d, want 0, body=%s", resp.Code, recorder.Body.String())
		}
		return resp.Data.Pending, resp.Data.Deleted
	}

	// 预览模式：报数但不删
	pending, deleted := callPurge(`{"confirm":false}`)
	if pending != 1 || deleted != 0 {
		t.Fatalf("预览模式 pending=%d deleted=%d，want 1/0", pending, deleted)
	}
	if n := countAccountRows(t, ghost.ID); n != 1 {
		t.Fatalf("预览模式不应删除数据，行数 = %d，want 1", n)
	}

	// 确认执行：物理删除
	pending, deleted = callPurge(`{"confirm":true}`)
	if pending != 1 || deleted != 1 {
		t.Fatalf("执行模式 pending=%d deleted=%d，want 1/1", pending, deleted)
	}
	if n := countAccountRows(t, ghost.ID); n != 0 {
		t.Fatalf("幽灵账号未被清理，行数 = %d，want 0", n)
	}
}
