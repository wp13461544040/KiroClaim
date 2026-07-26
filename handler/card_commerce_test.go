package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

	"github.com/gin-gonic/gin"
)

func TestGenerateCardsCanListGeneratedBatchOnShop(t *testing.T) {
	db := commerceTestDB(t)
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })
	if err := db.Create(&model.Account{Email: "stock@example.com", Subscription: "KIRO PRO"}).Error; err != nil {
		t.Fatal(err)
	}

	imageData := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	body := bytes.NewBufferString(`{"count":2,"subscription":"KIRO PRO","account_count":1,"list_on_shop":true,"price":1999,"image_data":"` + imageData + `"}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/cards/generate", body)
	ctx.Request.Header.Set("Content-Type", "application/json")
	GenerateCards(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			ProductID uint `json:"productId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var product model.CommerceProduct
	if err := db.First(&product, response.Data.ProductID).Error; err != nil {
		t.Fatal(err)
	}
	if product.BaseAmount != 1999 || product.BaseCurrency != "CNY" || product.ImageData != imageData || !product.Active {
		t.Fatalf("product = %#v", product)
	}
	var stockCount int64
	db.Model(&model.CommerceProductCard{}).Where("product_id = ? AND status = ?", product.ID, model.ProductCardAvailable).Count(&stockCount)
	if stockCount != 2 {
		t.Fatalf("stock count = %d", stockCount)
	}
}

func TestGenerateCardsRejectsInvalidShopImage(t *testing.T) {
	db := commerceTestDB(t)
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })
	if err := db.Create(&model.Account{Email: "stock@example.com", Subscription: "KIRO PRO"}).Error; err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"count":1,"subscription":"KIRO PRO","account_count":1,"list_on_shop":true,"price":1999,"image_data":"data:image/png;base64,PGgxPng8L2gxPg=="}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/cards/generate", body)
	ctx.Request.Header.Set("Content-Type", "application/json")
	GenerateCards(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var cards int64
	db.Model(&model.Card{}).Count(&cards)
	if cards != 0 {
		t.Fatalf("cards = %d", cards)
	}
}

func TestPublicShopProductsReturnsProductImage(t *testing.T) {
	db := commerceTestDB(t)
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })
	imageData := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
	product := model.CommerceProduct{Name: "Image product", ImageData: imageData, Active: true, BaseAmount: 1999, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/shop/products", nil)
	PublicShopProducts(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Products []struct {
				ImageData string `json:"imageData"`
			} `json:"products"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Products) != 1 || response.Data.Products[0].ImageData != imageData {
		t.Fatalf("products = %#v", response.Data.Products)
	}
}

func TestDelistCommerceProductGroupStockDeletesAvailableCardsAndPreservesSoldCards(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Delist", Active: true, BaseAmount: 1999, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	availableCard := model.Card{Code: "DELIST-AVAILABLE", Subscription: "KIRO PRO", AccountCount: 1}
	soldCard := model.Card{Code: "DELIST-SOLD", Subscription: "KIRO PRO", AccountCount: 1}
	if err := db.Create(&availableCard).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&soldCard).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: availableCard.ID, Status: model.ProductCardAvailable}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: soldCard.ID, Status: model.ProductCardSold, OrderID: 10}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := delistCommerceProductGroupStock(db, product.BaseAmount, product.Subscription, product.AccountCount)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 || result.PreservedSold != 1 {
		t.Fatalf("result = %#v", result)
	}
	var freshProduct model.CommerceProduct
	if err := db.First(&freshProduct, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if freshProduct.Active {
		t.Fatal("product should be inactive")
	}
	var availableCount int64
	db.Model(&model.Card{}).Where("id = ?", availableCard.ID).Count(&availableCount)
	if availableCount != 0 {
		t.Fatal("available card should be deleted")
	}
	var soldCount int64
	db.Model(&model.Card{}).Where("id = ?", soldCard.ID).Count(&soldCount)
	if soldCount != 1 {
		t.Fatal("sold card should be preserved")
	}
}

func TestDelistCommerceProductGroupStockRejectsReservedInventory(t *testing.T) {
	db := commerceTestDB(t)
	product := model.CommerceProduct{Name: "Reserved", Active: true, BaseAmount: 999, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	card := model.Card{Code: "DELIST-RESERVED", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&card).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardReserved, OrderID: 99}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := delistCommerceProductGroupStock(db, product.BaseAmount, product.Subscription, product.AccountCount); err == nil {
		t.Fatal("reserved inventory should block delisting")
	}
}

func TestDelistCommerceProductGroupStockDeletesAllMatchingBatches(t *testing.T) {
	db := commerceTestDB(t)
	matching := []model.CommerceProduct{
		{Name: "Batch A", Active: true, BaseAmount: 1999, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 1},
		{Name: "Batch B", Active: true, BaseAmount: 1999, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 1},
	}
	for i := range matching {
		if err := db.Create(&matching[i]).Error; err != nil {
			t.Fatal(err)
		}
		card := model.Card{Code: "GROUP-MATCH-" + strconv.Itoa(i+1), Subscription: "KIRO PRO", AccountCount: 1}
		if err := db.Create(&card).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.CommerceProductCard{ProductID: matching[i].ID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
			t.Fatal(err)
		}
	}

	unrelated := model.CommerceProduct{Name: "Other subscription", Active: true, BaseAmount: 1999, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&unrelated).Error; err != nil {
		t.Fatal(err)
	}
	unrelatedCard := model.Card{Code: "GROUP-UNRELATED", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&unrelatedCard).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CommerceProductCard{ProductID: unrelated.ID, CardID: unrelatedCard.ID, Status: model.ProductCardAvailable}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := delistCommerceProductGroupStock(db, 1999, "KIRO PRO", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 2 || result.Products != 2 {
		t.Fatalf("result = %#v", result)
	}
	var matchingProducts int64
	db.Model(&model.CommerceProduct{}).Where("id IN ?", []uint{matching[0].ID, matching[1].ID}).Count(&matchingProducts)
	if matchingProducts != 0 {
		t.Fatalf("empty matching products = %d", matchingProducts)
	}
	var unrelatedCount int64
	db.Model(&model.Card{}).Where("id = ?", unrelatedCard.ID).Count(&unrelatedCount)
	if unrelatedCount != 1 {
		t.Fatal("same-price product with a different subscription must be preserved")
	}
}

func TestListCardsOmitsInactivePriceGroupsFromFilterOptions(t *testing.T) {
	db := commerceTestDB(t)
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })
	inactive := model.CommerceProduct{Name: "Old price", Active: false, BaseAmount: 100, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	active := model.CommerceProduct{Name: "Current price", Active: true, BaseAmount: 200, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&inactive).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&inactive).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&active).Error; err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/cards?page=1&size=20", nil)
	ListCards(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Filters struct {
				ShopPriceGroups []struct {
					Amount int64 `json:"amount"`
				} `json:"shopPriceGroups"`
			} `json:"filters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Filters.ShopPriceGroups) != 1 || response.Data.Filters.ShopPriceGroups[0].Amount != active.BaseAmount {
		t.Fatalf("price groups = %#v", response.Data.Filters.ShopPriceGroups)
	}
}

func TestDelistCommerceProductGroupStockRejectsReservedBatch(t *testing.T) {
	db := commerceTestDB(t)
	availableProduct := model.CommerceProduct{Name: "Available batch", Active: true, BaseAmount: 2999, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 2}
	reservedProduct := model.CommerceProduct{Name: "Reserved batch", Active: true, BaseAmount: 2999, BaseCurrency: "CNY", Subscription: "KIRO PRO", AccountCount: 2}
	if err := db.Create(&availableProduct).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&reservedProduct).Error; err != nil {
		t.Fatal(err)
	}
	for i, item := range []struct {
		productID uint
		status    string
		orderID   uint
	}{{availableProduct.ID, model.ProductCardAvailable, 0}, {reservedProduct.ID, model.ProductCardReserved, 42}} {
		card := model.Card{Code: "GROUP-RESERVED-" + strconv.Itoa(i+1), Subscription: "KIRO PRO", AccountCount: 2}
		if err := db.Create(&card).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.CommerceProductCard{ProductID: item.productID, CardID: card.ID, Status: item.status, OrderID: item.orderID}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if _, err := delistCommerceProductGroupStock(db, 2999, "KIRO PRO", 2); !errors.Is(err, errCommerceProductReservedInventory) {
		t.Fatalf("error = %v", err)
	}
	var active int64
	db.Model(&model.CommerceProduct{}).Where("id IN ? AND active = ?", []uint{availableProduct.ID, reservedProduct.ID}, true).Count(&active)
	if active != 2 {
		t.Fatalf("active products = %d", active)
	}
}

func TestListCardsFiltersByShopProduct(t *testing.T) {
	db := commerceTestDB(t)
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = oldDB })
	firstProduct := model.CommerceProduct{Name: "First", Active: true, BaseAmount: 100, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	secondProduct := model.CommerceProduct{Name: "Second", Active: true, BaseAmount: 200, BaseCurrency: "CNY", Subscription: "KIRO FREE", AccountCount: 1}
	if err := db.Create(&firstProduct).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&secondProduct).Error; err != nil {
		t.Fatal(err)
	}
	for i, productID := range []uint{firstProduct.ID, secondProduct.ID} {
		card := model.Card{Code: "FILTER-CARD-" + strconv.Itoa(i+1), Subscription: "KIRO FREE", AccountCount: 1}
		if err := db.Create(&card).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.CommerceProductCard{ProductID: productID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/cards?page=1&size=20&shop_product_id="+strconv.FormatUint(uint64(firstProduct.ID), 10), nil)
	ListCards(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			List []struct {
				ID            uint `json:"ID"`
				ShopProductID uint `json:"ShopProductID"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.List) != 1 || response.Data.List[0].ShopProductID != firstProduct.ID {
		t.Fatalf("filtered cards = %#v", response.Data.List)
	}

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/admin/cards?page=1&size=20&shop_price=100&shop_subscription=KIRO%20FREE&shop_account_count=1", nil)
	ListCards(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("group filter status %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.List) != 1 || response.Data.List[0].ShopProductID != firstProduct.ID {
		t.Fatalf("group-filtered cards = %#v", response.Data.List)
	}
}
