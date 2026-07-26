package handler

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPersistHealthUpdatesWritesSuspendedStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "app.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite handle: %v", err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&model.Account{}); err != nil {
		t.Fatalf("migrate account: %v", err)
	}
	database.DB = db

	account := model.Account{
		AccessToken:  "access",
		RefreshToken: "refresh",
		Status:       model.AccountStatusActive,
	}
	if err := database.DB.Create(&account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}

	err = persistHealthUpdates(account.ID, map[string]interface{}{
		"status":          model.AccountStatusSuspended,
		"last_checked_at": time.Now(),
	})
	if err != nil {
		t.Fatalf("persist health updates: %v", err)
	}

	var got model.Account
	if err := database.DB.First(&got, account.ID).Error; err != nil {
		t.Fatalf("reload account: %v", err)
	}
	if got.Status != model.AccountStatusSuspended {
		t.Fatalf("status = %q, want %q", got.Status, model.AccountStatusSuspended)
	}
}
