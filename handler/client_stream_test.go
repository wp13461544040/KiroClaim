package handler

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLoadTokenStreamCardsKeepsFullAccountCountForPartialUsedCard(t *testing.T) {
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
	defer sqlDB.Close()

	if err := db.AutoMigrate(&model.Card{}, &model.Account{}, &model.CardAccount{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.DB = db

	usedAt := time.Now()
	card := model.Card{
		Code:         "PARTIAL-50",
		UsedAt:       &usedAt,
		AccountCount: 50,
		Subscription: "KIRO FREE",
	}
	if err := database.DB.Create(&card).Error; err != nil {
		t.Fatalf("create card: %v", err)
	}

	for i := 0; i < 2; i++ {
		account := model.Account{
			AccessToken:   "access",
			RefreshToken:  "refresh",
			Status:        model.AccountStatusActive,
			Subscription:  "KIRO FREE",
			LastCheckedAt: &usedAt,
			Used:          true,
		}
		if err := database.DB.Create(&account).Error; err != nil {
			t.Fatalf("create account %d: %v", i, err)
		}
		if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID}).Error; err != nil {
			t.Fatalf("create binding %d: %v", i, err)
		}
	}

	cards, errBody, status := loadTokenStreamCards([]string{card.Code})
	if errBody != nil {
		t.Fatalf("load token stream cards returned status %d: %#v", status, errBody)
	}
	if len(cards) != 1 {
		t.Fatalf("cards len = %d, want 1", len(cards))
	}
	if !cards[0].used {
		t.Fatalf("card should be marked used")
	}
	if cards[0].total != 50 {
		t.Fatalf("stream total = %d, want 50", cards[0].total)
	}
}

func TestDeleteAssignedAccountKeepsCardUsedAndReportsMissingBinding(t *testing.T) {
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
	defer sqlDB.Close()

	if err := db.AutoMigrate(&model.Card{}, &model.Account{}, &model.CardAccount{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.DB = db

	usedAt := time.Now()
	card := model.Card{Code: "DELETED-ACCOUNT", UsedAt: &usedAt, AccountCount: 1}
	if err := database.DB.Create(&card).Error; err != nil {
		t.Fatalf("create card: %v", err)
	}
	account := model.Account{
		AccessToken:   "access",
		RefreshToken:  "refresh",
		Status:        model.AccountStatusActive,
		LastCheckedAt: &usedAt,
		Used:          true,
	}
	if err := database.DB.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	result := deleteAccountsPhysically([]uint{account.ID})
	if result.Error != nil {
		t.Fatalf("delete account: %v", result.Error)
	}

	var accountCount int64
	if err := database.DB.Unscoped().Model(&model.Account{}).Where("id = ?", account.ID).Count(&accountCount).Error; err != nil {
		t.Fatalf("count account: %v", err)
	}
	if accountCount != 0 {
		t.Fatalf("account count = %d, want 0", accountCount)
	}

	var freshCard model.Card
	if err := database.DB.First(&freshCard, card.ID).Error; err != nil {
		t.Fatalf("reload card: %v", err)
	}
	if freshCard.UsedAt == nil {
		t.Fatalf("card used_at should be preserved")
	}

	var bindingCount int64
	if err := database.DB.Model(&model.CardAccount{}).Where("card_id = ?", card.ID).Count(&bindingCount).Error; err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("binding count = %d, want 1", bindingCount)
	}

	accounts, missing, err := cardBindingAccounts(card.ID)
	if err != nil {
		t.Fatalf("card binding accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("accounts len = %d, want 0", len(accounts))
	}
	if missing != 1 {
		t.Fatalf("missing = %d, want 1", missing)
	}
}

func TestUsedTokenCodeReturnsStoredAccountWithoutUpstreamCheck(t *testing.T) {
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
	defer sqlDB.Close()

	if err := db.AutoMigrate(&model.Card{}, &model.Account{}, &model.CardAccount{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.DB = db

	usedAt := time.Now()
	card := model.Card{Code: "USED-DIRECT", UsedAt: &usedAt, AccountCount: 1}
	if err := database.DB.Create(&card).Error; err != nil {
		t.Fatalf("create card: %v", err)
	}
	account := model.Account{
		AccessToken: "stored-access",
		Status:      model.AccountStatusActive,
		Used:        true,
	}
	if err := database.DB.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := database.DB.Create(&model.CardAccount{CardID: card.ID, AccountID: account.ID}).Error; err != nil {
		t.Fatalf("create binding: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/token/"+card.Code, nil)

	tokens, errResp, status := processOneCode(ctx, card.Code)
	if errResp != nil {
		t.Fatalf("process one code returned status %d: %#v", status, errResp)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens len = %d, want 1", len(tokens))
	}
	if tokens[0]["accessToken"] != "stored-access" {
		t.Fatalf("accessToken = %#v, want stored-access", tokens[0]["accessToken"])
	}
}
