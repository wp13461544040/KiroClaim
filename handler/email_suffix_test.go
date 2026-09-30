package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/wp13461544040/KiroClaim/database"
	"github.com/wp13461544040/KiroClaim/model"
)

// seedSuffixAccount 按指定邮箱和订阅建一个可派发的账号。
// setupDispatchPolicyTestDB 已把 database.DB 指向临时库，这里直接用全局句柄。
func seedSuffixAccount(t *testing.T, email string, subscription string) model.Account {
	t.Helper()
	account := model.Account{
		AccessToken:  "access-" + email,
		RefreshToken: "refresh-" + email,
		Email:        email,
		Status:       model.AccountStatusActive,
		Subscription: subscription,
	}
	if err := database.DB.Create(&account).Error; err != nil {
		t.Fatalf("create account %s: %v", email, err)
	}
	return account
}

func TestNormalizeEmailSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"gmail.com", "gmail.com"},
		{"  GMail.COM  ", "gmail.com"},
		{"@gmail.com", "gmail.com"},
		{"@Outlook.COM", "outlook.com"},
		{"my-domain.co.uk", "my-domain.co.uk"},
		{"", ""},
		{"   ", ""},
		{"@", ""},
		// 通配符和引号必须被拒掉，否则会被内联进 ORDER BY 的 SQL
		{"gmail%.com", ""},
		{"gmail_com", ""},
		{"gmail.com' OR '1'='1", ""},
		{"a b.com", ""},
	}
	for _, tc := range cases {
		if got := normalizeEmailSuffix(tc.in); got != tc.want {
			t.Errorf("normalizeEmailSuffix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// 指定后缀偏好时，即使命中的账号入库更晚，也要优先派发。
func TestPopAccountPrefersEmailSuffix(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	// 先建 outlook（更早入库，FIFO 下本该优先）
	seedSuffixAccount(t, "early@outlook.com", "KIRO PRO")
	wanted := seedSuffixAccount(t, "later@gmail.com", "KIRO PRO")

	got, err := popAccount(0, "KIRO PRO", "gmail.com")
	if err != nil {
		t.Fatalf("pop account: %v", err)
	}
	if got.ID != wanted.ID {
		t.Fatalf("派发账号 id = %d（%s），want %d（gmail 后缀应优先于更早入库的 outlook）", got.ID, got.Email, wanted.ID)
	}
}

// 偏好后缀没有可用账号时，必须回退到其他后缀而不是失败。
func TestPopAccountFallsBackWhenSuffixUnavailable(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	only := seedSuffixAccount(t, "someone@outlook.com", "KIRO PRO")

	got, err := popAccount(0, "KIRO PRO", "gmail.com")
	if err != nil {
		t.Fatalf("偏好后缀无账号时不应失败，应回退: %v", err)
	}
	if got.ID != only.ID {
		t.Fatalf("派发账号 id = %d, want %d", got.ID, only.ID)
	}
}

// 空偏好（老卡密）必须保持原有的 FIFO 行为。
func TestPopAccountEmptySuffixKeepsFifo(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	first := seedSuffixAccount(t, "first@gmail.com", "KIRO PRO")
	seedSuffixAccount(t, "second@outlook.com", "KIRO PRO")

	got, err := popAccount(0, "KIRO PRO", "")
	if err != nil {
		t.Fatalf("pop account: %v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("派发账号 id = %d, want %d（空偏好应维持 FIFO）", got.ID, first.ID)
	}
}

// 多号卡：偏好账号不够时混合发放，先给偏好的再用其他后缀补齐。
func TestPopMultipleAccountsMixesWhenSuffixInsufficient(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	outlookA := seedSuffixAccount(t, "a@outlook.com", "KIRO PRO")
	gmailOnly := seedSuffixAccount(t, "b@gmail.com", "KIRO PRO")
	outlookB := seedSuffixAccount(t, "c@outlook.com", "KIRO PRO")

	got, err := popMultipleAccounts(3, "KIRO PRO", "gmail.com")
	if err != nil {
		t.Fatalf("偏好不足时应混合发放而非失败: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("派发数量 = %d, want 3", len(got))
	}
	// 第一个必须是唯一的 gmail 账号，后两个是按 FIFO 补齐的 outlook
	if got[0].ID != gmailOnly.ID {
		t.Fatalf("首个账号 id = %d（%s），want %d（gmail 应排最前）", got[0].ID, got[0].Email, gmailOnly.ID)
	}
	if got[1].ID != outlookA.ID || got[2].ID != outlookB.ID {
		t.Fatalf("补齐账号 ids = [%d %d], want [%d %d]（应按 FIFO 补齐）",
			got[1].ID, got[2].ID, outlookA.ID, outlookB.ID)
	}
}

// 偏好账号充足时应全部使用偏好后缀。
func TestPopMultipleAccountsAllPreferredWhenEnough(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	seedSuffixAccount(t, "a@outlook.com", "KIRO PRO")
	g1 := seedSuffixAccount(t, "b@gmail.com", "KIRO PRO")
	g2 := seedSuffixAccount(t, "c@gmail.com", "KIRO PRO")

	got, err := popMultipleAccounts(2, "KIRO PRO", "gmail.com")
	if err != nil {
		t.Fatalf("pop multiple accounts: %v", err)
	}
	if got[0].ID != g1.ID || got[1].ID != g2.ID {
		t.Fatalf("派发 ids = [%d %d], want [%d %d]（应全为 gmail）", got[0].ID, got[1].ID, g1.ID, g2.ID)
	}
}

// 验证 SUBSTR + INSTR 聚合在 SQLite 下真的可用，且可用数口径正确。
func TestAccountEmailSuffixStats(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	seedSuffixAccount(t, "a@gmail.com", "KIRO PRO")
	seedSuffixAccount(t, "b@gmail.com", "KIRO PRO")
	seedSuffixAccount(t, "c@outlook.com", "KIRO PRO")
	// 大写邮箱应与小写归并到同一个后缀
	seedSuffixAccount(t, "d@GMAIL.COM", "KIRO PRO")
	// 已分配的账号计入 totalCount 但不计入 unusedCount
	used := seedSuffixAccount(t, "e@gmail.com", "KIRO PRO")
	if err := database.DB.Model(&model.Account{}).Where("id = ?", used.ID).Update("used", true).Error; err != nil {
		t.Fatalf("mark used: %v", err)
	}
	// 没有 @ 的脏数据应被排除
	seedSuffixAccount(t, "broken-email", "KIRO PRO")

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/accounts/email-suffix-stats", nil)

	AccountEmailSuffixStats(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			Suffix      string `json:"suffix"`
			UnusedCount int64  `json:"unusedCount"`
			TotalCount  int64  `json:"totalCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v, body=%s", err, recorder.Body.String())
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0, body=%s", resp.Code, recorder.Body.String())
	}

	stats := map[string][2]int64{}
	for _, it := range resp.Data {
		stats[it.Suffix] = [2]int64{it.UnusedCount, it.TotalCount}
	}
	// gmail: 4 个（含 1 个已分配、1 个大写），可用 3 个
	if got := stats["gmail.com"]; got != [2]int64{3, 4} {
		t.Fatalf("gmail.com 统计 = unused %d / total %d, want 3 / 4", got[0], got[1])
	}
	if got := stats["outlook.com"]; got != [2]int64{1, 1} {
		t.Fatalf("outlook.com 统计 = unused %d / total %d, want 1 / 1", got[0], got[1])
	}
	if _, exists := stats["broken-email"]; exists {
		t.Fatalf("无 @ 的脏数据不应出现在后缀统计里: %+v", resp.Data)
	}
	// 可用数多的排前面
	if len(resp.Data) > 0 && resp.Data[0].Suffix != "gmail.com" {
		t.Fatalf("首项 = %q, want gmail.com（可用数最多应排最前）", resp.Data[0].Suffix)
	}
}

// 账号池按邮箱后缀筛选。
func TestListAccountsFiltersByEmailSuffix(t *testing.T) {
	setupDispatchPolicyTestDB(t)
	seedSuffixAccount(t, "a@gmail.com", "KIRO PRO")
	seedSuffixAccount(t, "b@GMAIL.COM", "KIRO PRO")
	seedSuffixAccount(t, "c@outlook.com", "KIRO PRO")

	query := func(suffix string) int64 {
		t.Helper()
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/accounts?email_suffix="+suffix, nil)

		ListAccounts(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Total int64 `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.Data.Total
	}

	if got := query("gmail.com"); got != 2 {
		t.Fatalf("gmail.com 筛选结果 = %d, want 2（应大小写不敏感）", got)
	}
	if got := query("outlook.com"); got != 1 {
		t.Fatalf("outlook.com 筛选结果 = %d, want 1", got)
	}
	// 非法后缀被归一化成空串，应视为不筛选而返回全部
	if got := query("bad%25suffix"); got != 3 {
		t.Fatalf("非法后缀筛选结果 = %d, want 3（应忽略筛选）", got)
	}
}

// 生成卡密时指定的后缀要落库，并能从管理端列表读回。
func TestGenerateCardsPersistsEmailSuffix(t *testing.T) {
	db := setupDispatchPolicyTestDB(t)
	if err := db.AutoMigrate(&model.Card{}, &model.CardAccount{}, &model.OpLog{}, &model.CommerceProduct{}, &model.CommerceProductCard{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// GenerateCards 会校验订阅在账号表里真实存在
	seedSuffixAccount(t, "a@gmail.com", "KIRO PRO")

	// 返回生成出来的卡密号，便于精确查回对应记录，
	// 不依赖排序（GORM 的 First 会自行追加主键排序）。
	generate := func(body string) (*httptest.ResponseRecorder, string) {
		t.Helper()
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/cards/generate", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		GenerateCards(ctx)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				Codes []string `json:"codes"`
			} `json:"data"`
		}
		_ = json.Unmarshal(recorder.Body.Bytes(), &resp)
		if len(resp.Data.Codes) > 0 {
			return recorder, resp.Data.Codes[0]
		}
		return recorder, ""
	}

	readCard := func(code string) model.Card {
		t.Helper()
		var card model.Card
		if err := database.DB.Where("code = ?", code).Take(&card).Error; err != nil {
			t.Fatalf("read card %s: %v", code, err)
		}
		return card
	}

	// 指定合法后缀，应被归一化成小写且去掉 @
	rec, code := generate(`{"count":1,"account_count":1,"subscription":"KIRO PRO","email_suffix":"@GMail.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := readCard(code).EmailSuffix; got != "gmail.com" {
		t.Fatalf("落库的 EmailSuffix = %q, want %q", got, "gmail.com")
	}

	// 留空表示不限后缀
	rec, code = generate(`{"count":1,"account_count":1,"subscription":"KIRO PRO"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("留空后缀应成功, status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := readCard(code).EmailSuffix; got != "" {
		t.Fatalf("留空时 EmailSuffix = %q, want 空串", got)
	}

	// 非法格式必须报错而不是静默变成不限，否则偏好失效很难排查
	rec, _ = generate(`{"count":1,"account_count":1,"subscription":"KIRO PRO","email_suffix":"bad suffix%"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法后缀 status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

// 后缀偏好是管理端内部信息，不能出现在下发给兑换用户的载荷里。
func TestEmailSuffixNotExposedToRedeemResponse(t *testing.T) {
	account := &model.Account{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ClientId:     "client-id",
		ClientSecret: "client-secret",
		Email:        "someone@gmail.com",
		Subscription: "KIRO PRO",
	}

	for name, payload := range map[string]gin.H{
		"buildTokenEntry":  buildTokenEntry(account),
		"buildAccountResp": buildAccountResp(account),
	} {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		var fields map[string]interface{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("%s unmarshal: %v", name, err)
		}
		for _, banned := range []string{"emailSuffix", "email_suffix", "EmailSuffix"} {
			if _, exists := fields[banned]; exists {
				t.Fatalf("%s 的响应泄露了字段 %q: %s", name, banned, raw)
			}
		}
	}
}
