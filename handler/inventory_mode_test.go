package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"
)

// setInventoryMode 临时切换库存模式，测试结束后自动还原。
func setInventoryMode(t *testing.T, enabled bool) {
	t.Helper()
	settingsMu.Lock()
	old := currentSettings.RetainCreditExhaustedAccounts
	currentSettings.RetainCreditExhaustedAccounts = enabled
	settingsMu.Unlock()
	t.Cleanup(func() {
		settingsMu.Lock()
		currentSettings.RetainCreditExhaustedAccounts = old
		settingsMu.Unlock()
	})
}

// 库存模式关闭时保持原有行为：用过额度就移入已使用，封禁账号不标记 used。
func TestBuildHealthUpdatesDefaultMarksCreditUsed(t *testing.T) {
	setInventoryMode(t, false)
	now := time.Now()

	updates := buildHealthUpdates(healthResult{
		status:      model.AccountStatusActive,
		creditUsed:  3,
		creditLimit: 100,
	}, now)
	if updates["used"] != true {
		t.Fatalf("库存模式关闭时用过额度应移入已使用，updates = %+v", updates)
	}

	// 封禁账号无论哪种模式都要移入已使用
	updates = buildHealthUpdates(healthResult{
		status:     model.AccountStatusSuspended,
		creditUsed: 0,
	}, now)
	if updates["used"] != true {
		t.Fatalf("库存模式关闭时封禁账号也应移入已使用，updates = %+v", updates)
	}
	if updates["used_at"] == nil {
		t.Fatalf("移入已使用时应记录 used_at，updates = %+v", updates)
	}
}

// 库存模式开启时只把封禁账号移入已使用，用过额度的留在账号池。
func TestBuildHealthUpdatesInventoryModeOnlyMarksSuspended(t *testing.T) {
	setInventoryMode(t, true)
	now := time.Now()

	// 用过额度但未封禁：必须留在池里
	updates := buildHealthUpdates(healthResult{
		status:      model.AccountStatusActive,
		creditUsed:  50,
		creditLimit: 100,
	}, now)
	if _, exists := updates["used"]; exists {
		t.Fatalf("库存模式下用过额度的账号应留在账号池，updates = %+v", updates)
	}
	// 额度信息本身仍要落库，否则前端看不到用量
	if updates["credit_used"] != float64(50) {
		t.Fatalf("额度用量仍应落库，updates = %+v", updates)
	}

	// 额度耗尽但未封禁：同样留在池里
	updates = buildHealthUpdates(healthResult{
		status:      model.AccountStatusActive,
		creditUsed:  100,
		creditLimit: 100,
	}, now)
	if _, exists := updates["used"]; exists {
		t.Fatalf("库存模式下额度耗尽也应留在账号池，updates = %+v", updates)
	}

	// 封禁：移入已使用
	updates = buildHealthUpdates(healthResult{
		status:     model.AccountStatusSuspended,
		creditUsed: 0,
	}, now)
	if updates["used"] != true {
		t.Fatalf("库存模式下封禁账号应移入已使用，updates = %+v", updates)
	}
	if updates["used_at"] == nil {
		t.Fatalf("移入已使用时应记录 used_at，updates = %+v", updates)
	}

	// 封禁且用过额度：封禁优先，仍要移走，不能因为库存模式留在池里
	updates = buildHealthUpdates(healthResult{
		status:      model.AccountStatusSuspended,
		creditUsed:  50,
		creditLimit: 100,
	}, now)
	if updates["used"] != true {
		t.Fatalf("封禁优先于库存模式，应移入已使用，updates = %+v", updates)
	}
}

// 老配置里没有这个字段时必须保持默认关闭，不能因为升级就改变发货行为。
func TestInventoryModeMissingFieldKeepsDefaultOff(t *testing.T) {
	s := AppSettings{RetainCreditExhaustedAccounts: false}
	var stored storedRuntimeSettings
	if err := json.Unmarshal([]byte(`{"dispatchHealthCheckEnabled":true}`), &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mergeStoredRuntimeSettings(&s, stored)
	if s.RetainCreditExhaustedAccounts {
		t.Fatal("老配置缺少该字段时应保持关闭")
	}

	// 显式 true 要能覆盖默认值并完成序列化往返
	if err := json.Unmarshal([]byte(`{"retainCreditExhaustedAccounts":true}`), &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mergeStoredRuntimeSettings(&s, stored)
	if !s.RetainCreditExhaustedAccounts {
		t.Fatal("显式 true 应覆盖默认值")
	}
}

// 库存模式开启时「清理额度已用」必须被拒绝，否则会把想保留的账号一次性推走。
func TestCleanupUsedCreditRejectedInInventoryMode(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	setInventoryMode(t, true)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/cleanup-used-credit", nil)

	CleanupUsedCreditAccountsAPI(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "库存模式") {
		t.Fatalf("错误信息应说明原因，body=%s", recorder.Body.String())
	}
}

// 回迁接口的安全条件：只放回真正误判的账号。
func TestRestoreCreditUsedAccountsSafety(t *testing.T) {
	db := setupDispatchPolicyTestDB(t)
	if err := db.AutoMigrate(&model.Card{}, &model.CardAccount{}, &model.OpLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	long := time.Now().Add(-time.Hour)
	// 建一个 used=true 的账号并指定 used_at，模拟历史数据
	seed := func(email string, opts map[string]interface{}) model.Account {
		t.Helper()
		acc := model.Account{
			AccessToken:  "access-" + email,
			RefreshToken: "refresh-" + email,
			Email:        email,
			Status:       model.AccountStatusActive,
			Used:         true,
		}
		if err := database.DB.Create(&acc).Error; err != nil {
			t.Fatalf("create %s: %v", email, err)
		}
		base := map[string]interface{}{"used": true, "used_at": long, "credit_used": 5.0, "credit_limit": 100.0}
		for k, v := range opts {
			base[k] = v
		}
		if err := database.DB.Model(&model.Account{}).Where("id = ?", acc.ID).Updates(base).Error; err != nil {
			t.Fatalf("update %s: %v", email, err)
		}
		return acc
	}

	// 应被回迁：用过额度、未耗尽、无绑定、used_at 够久
	target := seed("target@a.com", nil)
	// 不该回迁：已绑定卡密（真的发给买家了）
	bound := seed("bound@a.com", nil)
	card := model.Card{Code: "KIRO-BOUND", AccountCount: 1}
	if err := database.DB.Create(&card).Error; err != nil {
		t.Fatalf("create card: %v", err)
	}
	if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: bound.ID}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}
	// 不该回迁：刚刚预留（可能正在发货中）
	fresh := seed("fresh@a.com", map[string]interface{}{"used_at": time.Now()})
	// 不该回迁：额度已耗尽，回池也发不出去
	exhausted := seed("exhausted@a.com", map[string]interface{}{"credit_used": 100.0, "credit_limit": 100.0})
	// 不该回迁：封禁
	suspended := seed("suspended@a.com", map[string]interface{}{"status": model.AccountStatusSuspended})
	// 不该回迁：管理员主动清理过的（status=used）
	cleaned := seed("cleaned@a.com", map[string]interface{}{"status": model.AccountStatusUsed})
	// 不该回迁：从未用过额度
	untouched := seed("untouched@a.com", map[string]interface{}{"credit_used": 0.0})

	call := func(body string) (int64, int64) {
		t.Helper()
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/restore-credit-used", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")

		RestoreCreditUsedAccounts(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Pending  int64 `json:"pending"`
				Restored int64 `json:"restored"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Code != 0 {
			t.Fatalf("code = %d, body=%s", resp.Code, recorder.Body.String())
		}
		return resp.Data.Pending, resp.Data.Restored
	}

	// 预览模式：只报数不改数据
	pending, restored := call(`{"confirm":false}`)
	if pending != 1 || restored != 0 {
		t.Fatalf("预览 pending=%d restored=%d, want 1/0", pending, restored)
	}
	var stillUsed model.Account
	if err := database.DB.Where("id = ?", target.ID).Take(&stillUsed).Error; err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !stillUsed.Used {
		t.Fatal("预览模式不应改动数据")
	}

	// 执行回迁
	pending, restored = call(`{"confirm":true}`)
	if pending != 1 || restored != 1 {
		t.Fatalf("执行 pending=%d restored=%d, want 1/1", pending, restored)
	}

	isUsed := func(id uint, label string) bool {
		t.Helper()
		var acc model.Account
		if err := database.DB.Where("id = ?", id).Take(&acc).Error; err != nil {
			t.Fatalf("read %s: %v", label, err)
		}
		return acc.Used
	}

	if isUsed(target.ID, "target") {
		t.Fatal("误判账号应被放回账号池")
	}
	for _, c := range []struct {
		id    uint
		label string
	}{
		{bound.ID, "已绑定卡密的账号（放回会导致二次发货）"},
		{fresh.ID, "刚预留的账号（可能正在发货中）"},
		{exhausted.ID, "额度已耗尽的账号"},
		{suspended.ID, "封禁账号"},
		{cleaned.ID, "管理员主动清理过的账号"},
		{untouched.ID, "从未用过额度的账号"},
	} {
		if !isUsed(c.id, c.label) {
			t.Fatalf("%s 不应被放回账号池", c.label)
		}
	}
}

// 封禁账号会被标记 used=true，「清理封禁账号」不能再按 used=false 过滤，
// 否则刚刷出来的封禁账号一个都删不掉。改用绑定关系判断是否已发给买家。
func TestDeleteAccountsByStatusIgnoresUsedFlag(t *testing.T) {
	db := setupDispatchPolicyTestDB(t)
	if err := db.AutoMigrate(&model.Card{}, &model.CardAccount{}, &model.OpLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	seedSuspended := func(email string, used bool) model.Account {
		t.Helper()
		acc := model.Account{
			AccessToken:  "access-" + email,
			RefreshToken: "refresh-" + email,
			Email:        email,
			Status:       model.AccountStatusSuspended,
			Used:         used,
		}
		if err := database.DB.Create(&acc).Error; err != nil {
			t.Fatalf("create %s: %v", email, err)
		}
		return acc
	}

	// 封禁且已被标记 used=true，但没有卡密绑定 → 应该能删掉
	markedUsed := seedSuspended("marked@a.com", true)
	// 封禁且 used=false → 同样应该删掉
	notUsed := seedSuspended("notused@a.com", false)
	// 封禁但已绑定卡密（真的发给买家了）→ 必须保留
	bound := seedSuspended("bound@a.com", true)
	card := model.Card{Code: "KIRO-SUSPENDED", AccountCount: 1}
	if err := database.DB.Create(&card).Error; err != nil {
		t.Fatalf("create card: %v", err)
	}
	if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: bound.ID}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	// 直接验证删除语句的过滤条件，不走完整 handler（它会并发请求上游做健康检查）
	result := database.DB.Unscoped().
		Where("status = ?", model.AccountStatusSuspended).
		Where("id NOT IN (?)", database.DB.Model(&model.CardAccount{}).Select("account_id")).
		Delete(&model.Account{})
	if result.Error != nil {
		t.Fatalf("delete: %v", result.Error)
	}
	if result.RowsAffected != 2 {
		t.Fatalf("删除数量 = %d, want 2（被标记 used 的封禁账号也要能删）", result.RowsAffected)
	}

	exists := func(id uint) bool {
		t.Helper()
		var n int64
		if err := database.DB.Unscoped().Model(&model.Account{}).Where("id = ?", id).Count(&n).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		return n > 0
	}
	if exists(markedUsed.ID) {
		t.Fatal("已标记 used 但无绑定的封禁账号应被删除")
	}
	if exists(notUsed.ID) {
		t.Fatal("未分配的封禁账号应被删除")
	}
	if !exists(bound.ID) {
		t.Fatal("已绑定卡密的封禁账号不应被删除")
	}
}
