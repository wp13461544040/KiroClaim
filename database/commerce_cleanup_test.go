package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/huey1in/KiroClaim/model"
	"gorm.io/gorm"
)

func TestCleanupEmptyCommerceProductsDeletesOnlyInactiveOrphans(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.CommerceProduct{}, &model.CommerceProductCard{}); err != nil {
		t.Fatal(err)
	}

	stale := model.CommerceProduct{Name: "Stale", Active: true, BaseAmount: 100, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	historical := model.CommerceProduct{Name: "Historical", Active: true, BaseAmount: 200, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	active := model.CommerceProduct{Name: "Active", Active: true, BaseAmount: 300, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	for _, product := range []*model.CommerceProduct{&stale, &historical, &active} {
		if err := db.Create(product).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.CommerceProduct{}).Where("id IN ?", []uint{stale.ID, historical.ID}).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: historical.ID, CardID: 99, Status: model.ProductCardSold}).Error; err != nil {
		t.Fatal(err)
	}

	if err := cleanupEmptyCommerceProducts(db); err != nil {
		t.Fatal(err)
	}
	var staleCount, historicalCount, activeCount int64
	db.Model(&model.CommerceProduct{}).Where("id = ?", stale.ID).Count(&staleCount)
	db.Model(&model.CommerceProduct{}).Where("id = ?", historical.ID).Count(&historicalCount)
	db.Model(&model.CommerceProduct{}).Where("id = ?", active.ID).Count(&activeCount)
	if staleCount != 0 || historicalCount != 1 || activeCount != 1 {
		t.Fatalf("counts stale=%d historical=%d active=%d", staleCount, historicalCount, activeCount)
	}
}
