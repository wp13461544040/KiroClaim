package database

import (
	"path/filepath"
	"testing"

	"github.com/huey1in/KiroClaim/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyCommerceProduct struct {
	ID            uint `gorm:"primaryKey"`
	Name          string
	InventoryMode string `gorm:"not null"`
}

func (legacyCommerceProduct) TableName() string { return "commerce_products" }

type legacyCommerceOrder struct {
	ID            uint   `gorm:"primaryKey"`
	OrderNo       string `gorm:"not null"`
	InventoryMode string `gorm:"not null"`
}

func (legacyCommerceOrder) TableName() string { return "commerce_orders" }

func TestDropDeprecatedCommerceInventoryModeColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&legacyCommerceProduct{}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn(&legacyCommerceProduct{}, "inventory_mode") {
		t.Fatal("legacy inventory_mode column was not created")
	}
	if err := dropDeprecatedCommerceColumns(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&model.CommerceProduct{}, "inventory_mode") {
		t.Fatal("inventory_mode column still exists")
	}
}

func TestDropDeprecatedCommerceOrderInventoryModeColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "migration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&legacyCommerceOrder{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&legacyCommerceOrder{OrderNo: "ORDER-1", InventoryMode: "pre_generated"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := dropDeprecatedCommerceColumns(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&model.CommerceOrder{}, "inventory_mode") {
		t.Fatal("commerce_orders.inventory_mode column still exists")
	}
	var count int64
	if err := db.Table("commerce_orders").Where("order_no = ?", "ORDER-1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected migrated order to remain, got %d", count)
	}
}
