package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/huey1in/KiroClaim/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func randomCommerceToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

func createCommerceOrder(db *gorm.DB, product model.CommerceProduct, channel *model.CommercePaymentChannel, contact, queryPassword string, quantity, expiryMinutes int) (*model.CommerceOrder, string, error) {
	if !product.Active || product.BaseAmount < 0 || quantity < 1 || strings.TrimSpace(contact) == "" {
		return nil, "", errors.New("invalid product or contact")
	}
	password := strings.TrimSpace(queryPassword)
	if len(password) < 6 || len([]byte(password)) > 72 {
		return nil, "", errors.New("query password must be 6 to 72 bytes")
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if product.BaseAmount > 0 && int64(quantity) > maxInt64/product.BaseAmount {
		return nil, "", errors.New("order amount is too large")
	}
	if expiryMinutes <= 0 {
		expiryMinutes = 30
	}
	orderToken, err := randomCommerceToken(8)
	if err != nil {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}
	order := &model.CommerceOrder{
		OrderNo:           "KC" + time.Now().Format("20060102150405") + orderToken,
		QueryPasswordHash: string(hash), Contact: strings.TrimSpace(contact), ProductID: product.ID,
		ProductName: product.Name, Subscription: product.Subscription,
		Quantity: quantity, AccountCount: product.AccountCount * quantity, Amount: product.BaseAmount * int64(quantity), Currency: strings.ToUpper(product.BaseCurrency),
		Status: model.OrderPendingPayment, ExpiresAt: time.Now().Add(time.Duration(expiryMinutes) * time.Minute),
	}
	if channel != nil {
		order.ChannelID = channel.ID
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		var stocks []model.CommerceProductCard
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("product_id = ? AND status = ?", product.ID, model.ProductCardAvailable).Order("id ASC").Limit(quantity).Find(&stocks).Error; err != nil {
			return err
		}
		if len(stocks) != quantity {
			return errors.New("insufficient stock")
		}
		for _, stock := range stocks {
			result := tx.Model(&model.CommerceProductCard{}).Where("id = ? AND status = ?", stock.ID, model.ProductCardAvailable).Updates(map[string]interface{}{"status": model.ProductCardReserved, "order_id": order.ID})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return order, password, nil
}

func confirmAndFulfillCommerceOrder(db *gorm.DB, orderNo, paymentRef, operator string) (*model.CommerceOrder, error) {
	var result model.CommerceOrder
	paymentAccepted := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&result).Error; err != nil {
			return err
		}
		if result.Status == model.OrderCompleted {
			return nil
		}
		if result.Status != model.OrderPendingPayment && result.Status != model.OrderPaymentReview && result.Status != model.OrderPaid && result.Status != model.OrderPaidAttention {
			return fmt.Errorf("order status %s cannot be fulfilled", result.Status)
		}
		paymentAccepted = true
		now := time.Now()
		result.Status = model.OrderFulfilling
		result.PaidAt = &now
		result.PaymentRef = paymentRef
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		var stocks []model.CommerceProductCard
		if err := tx.Where("order_id = ? AND status = ?", result.ID, model.ProductCardReserved).Order("id ASC").Find(&stocks).Error; err != nil {
			return err
		}
		if len(stocks) != result.Quantity {
			return fmt.Errorf("reserved stock count %d does not match quantity %d", len(stocks), result.Quantity)
		}
		cardIDs := make([]uint, 0, len(stocks))
		for _, stock := range stocks {
			cardIDs = append(cardIDs, stock.CardID)
		}
		var cards []model.Card
		if err := tx.Where("id IN ?", cardIDs).Find(&cards).Error; err != nil {
			return err
		}
		if len(cards) != len(cardIDs) {
			return errors.New("reserved card is missing")
		}
		stockIDs := make([]uint, 0, len(stocks))
		for _, stock := range stocks {
			stockIDs = append(stockIDs, stock.ID)
		}
		resultUpdate := tx.Model(&model.CommerceProductCard{}).Where("id IN ? AND status = ?", stockIDs, model.ProductCardReserved).Update("status", model.ProductCardSold)
		if resultUpdate.Error != nil {
			return resultUpdate.Error
		}
		if int(resultUpdate.RowsAffected) != len(stocks) {
			return errors.New("reserved stock changed before fulfillment")
		}
		if len(cardIDs) == 0 {
			return errors.New("no reserved cards")
		}
		result.CardID = cardIDs[0]
		result.Status = model.OrderCompleted
		result.CompletedAt = &now
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		return tx.Create(&model.CommerceOrderLog{OrderID: result.ID, Action: "fulfilled", Detail: "card delivered", Operator: operator}).Error
	})
	if err != nil && paymentAccepted && result.ID != 0 {
		now := time.Now()
		_ = db.Model(&model.CommerceOrder{}).Where("id = ?", result.ID).Updates(map[string]interface{}{
			"status": model.OrderPaidAttention, "paid_at": now, "payment_ref": paymentRef, "review_note": err.Error(),
		}).Error
		result.Status = model.OrderPaidAttention
		result.PaidAt = &now
		result.PaymentRef = paymentRef
		result.ReviewNote = err.Error()
	}
	return &result, err
}

func commerceOrderCardCodes(db *gorm.DB, orderID uint) ([]string, error) {
	var stocks []model.CommerceProductCard
	if err := db.Where("order_id = ? AND status = ?", orderID, model.ProductCardSold).Order("id ASC").Find(&stocks).Error; err != nil {
		return nil, err
	}
	if len(stocks) == 0 {
		return []string{}, nil
	}
	ids := make([]uint, 0, len(stocks))
	for _, stock := range stocks {
		ids = append(ids, stock.CardID)
	}
	var cards []model.Card
	if err := db.Where("id IN ?", ids).Find(&cards).Error; err != nil {
		return nil, err
	}
	if len(cards) != len(ids) {
		return nil, errors.New("order card is missing")
	}
	byID := make(map[uint]string, len(cards))
	for _, card := range cards {
		byID[card.ID] = card.Code
	}
	codes := make([]string, 0, len(ids))
	for _, id := range ids {
		if code, ok := byID[id]; ok {
			codes = append(codes, code)
		}
	}
	return codes, nil
}
