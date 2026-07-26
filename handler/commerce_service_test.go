package handler

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/huey1in/KiroClaim/model"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func commerceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commerce.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Account{}, &model.Card{}, &model.CardAccount{}, &model.OpLog{}, &model.CommerceProduct{}, &model.CommerceProductCard{}, &model.CommerceOrder{}, &model.CommerceOrderLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCreateCommerceOrderReservesOnePreGeneratedCardAndHashesPassword(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Pro", Active: true, BaseAmount: 2990, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	card := model.Card{Code: "KIRO-TEST-CARD", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&card).Error; err != nil {
		t.Fatal(err)
	}
	stock := model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}
	if err := db.Create(&stock).Error; err != nil {
		t.Fatal(err)
	}

	order, password, err := createCommerceOrder(db, product, nil, "buyer@example.com", "query-pass", 1, 30)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if password == "" || bcrypt.CompareHashAndPassword([]byte(order.QueryPasswordHash), []byte(password)) != nil {
		t.Fatal("query password was not returned and stored as a bcrypt hash")
	}
	var reserved model.CommerceProductCard
	if err := db.First(&reserved, stock.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reserved.Status != model.ProductCardReserved || reserved.OrderID != order.ID {
		t.Fatalf("stock = status %q order %d", reserved.Status, reserved.OrderID)
	}
}

func TestFulfillCommerceOrderIsIdempotent(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Free", Active: true, BaseAmount: 990, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	card := model.Card{Code: "IDEMPOTENT-CARD", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&card).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
		t.Fatal(err)
	}
	order, _, err := createCommerceOrder(db, product, nil, "buyer@example.com", "query-pass", 1, 30)
	if err != nil {
		t.Fatal(err)
	}

	first, err := confirmAndFulfillCommerceOrder(db, order.OrderNo, "tx-1", "test")
	if err != nil {
		t.Fatalf("first fulfillment: %v", err)
	}
	second, err := confirmAndFulfillCommerceOrder(db, order.OrderNo, "tx-1", "test")
	if err != nil {
		t.Fatalf("second fulfillment: %v", err)
	}
	if first.CardID == 0 || first.CardID != second.CardID {
		t.Fatalf("card ids = %d and %d", first.CardID, second.CardID)
	}
	var count int64
	if err := db.Model(&model.Card{}).Where("id = ?", first.CardID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("card count = %d", count)
	}
}

func TestCreateCommerceOrderPreventsExistingCardOversell(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Free", Active: true, BaseAmount: 990, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	card := model.Card{Code: "ONLY-STOCK", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&card).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := createCommerceOrder(db, product, nil, "first@example.com", "query-pass", 1, 30); err != nil {
		t.Fatalf("first order: %v", err)
	}
	if _, _, err := createCommerceOrder(db, product, nil, "second@example.com", "query-pass", 1, 30); err == nil {
		t.Fatal("second order should be rejected because the only account is reserved")
	}
}

func TestFailedPaidFulfillmentPersistsPaidAttention(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Pro", Active: true, BaseAmount: 2990, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	card := model.Card{Code: "BROKEN-STOCK", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&card).Error; err != nil {
		t.Fatal(err)
	}
	stock := model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}
	if err := db.Create(&stock).Error; err != nil {
		t.Fatal(err)
	}
	order, _, err := createCommerceOrder(db, product, nil, "buyer@example.com", "query-pass", 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&card).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := confirmAndFulfillCommerceOrder(db, order.OrderNo, "paid-ref", "test"); err == nil {
		t.Fatal("fulfillment should fail when reserved card is missing")
	}
	var fresh model.CommerceOrder
	if err := db.First(&fresh, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fresh.Status != model.OrderPaidAttention {
		t.Fatalf("status = %q, want %q", fresh.Status, model.OrderPaidAttention)
	}
}

func TestCreateCommerceOrderReservesRequestedQuantityAndCalculatesTotal(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Bundle", Active: true, BaseAmount: 125, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 2}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		card := model.Card{Code: fmt.Sprintf("QUANTITY-CARD-%d", i+1), Subscription: "KIRO FREE", AccountCount: 2}
		if err := db.Create(&card).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
			t.Fatal(err)
		}
	}

	order, _, err := createCommerceOrder(db, product, nil, "buyer@example.com", "query-pass", 2, 30)
	if err != nil {
		t.Fatal(err)
	}
	if order.Quantity != 2 || order.Amount != 250 || order.AccountCount != 4 {
		t.Fatalf("order quantity=%d amount=%d accountCount=%d", order.Quantity, order.Amount, order.AccountCount)
	}
	var reserved int64
	db.Model(&model.CommerceProductCard{}).Where("order_id = ? AND status = ?", order.ID, model.ProductCardReserved).Count(&reserved)
	if reserved != 2 {
		t.Fatalf("reserved stock = %d, want 2", reserved)
	}
}

func TestCreateCommerceOrderUsesBuyerQueryPassword(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Password", Active: true, BaseAmount: 100, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	card := model.Card{Code: "PASSWORD-CARD", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&card).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
		t.Fatal(err)
	}

	order, password, err := createCommerceOrder(db, product, nil, "buyer@example.com", "buyer-password", 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	if password != "buyer-password" || bcrypt.CompareHashAndPassword([]byte(order.QueryPasswordHash), []byte(password)) != nil {
		t.Fatal("buyer query password was not returned and stored as a bcrypt hash")
	}
}

func TestFulfillCommerceOrderDeliversEveryReservedCard(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Bundle", Active: true, BaseAmount: 500, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		card := model.Card{Code: fmt.Sprintf("DELIVERY-CARD-%d", i+1), Subscription: "KIRO PRO", AccountCount: 1}
		if err := db.Create(&card).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
			t.Fatal(err)
		}
	}
	order, _, err := createCommerceOrder(db, product, nil, "buyer@example.com", "query-pass", 2, 30)
	if err != nil {
		t.Fatal(err)
	}
	fulfilled, err := confirmAndFulfillCommerceOrder(db, order.OrderNo, "paid-ref", "test")
	if err != nil {
		t.Fatal(err)
	}
	var sold int64
	db.Model(&model.CommerceProductCard{}).Where("order_id = ? AND status = ?", fulfilled.ID, model.ProductCardSold).Count(&sold)
	if sold != 2 {
		t.Fatalf("sold stock = %d, want 2", sold)
	}
}
