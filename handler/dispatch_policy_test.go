package handler

import (
	"path/filepath"
	"testing"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDispatchPolicyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := database.DB
	oldSettings := GetCurrentSettings()
	t.Cleanup(func() {
		database.DB = oldDB
		settingsMu.Lock()
		currentSettings = oldSettings
		settingsMu.Unlock()
	})

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "app.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Account{}); err != nil {
		t.Fatalf("migrate account: %v", err)
	}
	database.DB = db
	settingsMu.Lock()
	currentSettings.DispatchHealthCheckEnabled = false
	settingsMu.Unlock()
	return db
}

func createDispatchPolicyAccount(t *testing.T, db *gorm.DB, account model.Account) model.Account {
	t.Helper()
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	return account
}

func TestDeliveryWithoutHealthCheckSelectsOldestDatabaseEligibleAccount(t *testing.T) {
	db := setupDispatchPolicyTestDB(t)

	oldest := createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "oldest-access",
		Status:       model.AccountStatusActive,
		Subscription: "KIRO FREE",
	})
	createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "newer-access",
		Status:       model.AccountStatusActive,
		Subscription: "KIRO FREE",
	})
	createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "suspended-access",
		Status:       model.AccountStatusSuspended,
		Subscription: "KIRO FREE",
	})
	createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "used-access",
		Status:       model.AccountStatusActive,
		Subscription: "KIRO FREE",
		Used:         true,
	})
	createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "credited-access",
		Status:       model.AccountStatusActive,
		Subscription: "KIRO FREE",
		CreditUsed:   1,
	})

	got, err := popAccount(0, "KIRO FREE")
	if err != nil {
		t.Fatalf("pop account: %v", err)
	}
	if got.ID != oldest.ID {
		t.Fatalf("selected account id = %d, want oldest eligible id %d", got.ID, oldest.ID)
	}
}

func TestDeliveryWithoutHealthCheckSelectsMultipleMatchingAccounts(t *testing.T) {
	db := setupDispatchPolicyTestDB(t)

	first := createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "first-access",
		Status:       model.AccountStatusActive,
		Subscription: "KIRO PRO",
	})
	second := createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "second-access",
		Status:       model.AccountStatusActive,
		Subscription: "KIRO PRO",
	})
	createDispatchPolicyAccount(t, db, model.Account{
		AccessToken:  "wrong-subscription",
		Status:       model.AccountStatusActive,
		Subscription: "KIRO FREE",
	})

	got, err := popMultipleAccounts(2, "KIRO PRO")
	if err != nil {
		t.Fatalf("pop multiple accounts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("selected account count = %d, want 2", len(got))
	}
	if got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("selected ids = [%d %d], want [%d %d]", got[0].ID, got[1].ID, first.ID, second.ID)
	}
}
