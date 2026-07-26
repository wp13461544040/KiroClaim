package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errCommerceProductReservedInventory = errors.New("商品存在待支付订单预留库存")

type delistCommerceProductResult struct {
	Deleted       int64
	PreservedSold int64
	Products      int64
}

func delistCommerceProductGroupStock(db *gorm.DB, amount int64, subscription string, accountCount int) (delistCommerceProductResult, error) {
	return delistCommerceProductStockWhere(
		db,
		"base_amount = ? AND base_currency = ? AND subscription = ? AND account_count = ?",
		amount,
		"CNY",
		strings.TrimSpace(subscription),
		accountCount,
	)
}

func delistCommerceProductStockWhere(db *gorm.DB, query string, args ...interface{}) (delistCommerceProductResult, error) {
	var result delistCommerceProductResult
	err := db.Transaction(func(tx *gorm.DB) error {
		var products []model.CommerceProduct
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(query, args...).Find(&products).Error; err != nil {
			return err
		}
		if len(products) == 0 {
			return gorm.ErrRecordNotFound
		}
		productIDs := make([]uint, 0, len(products))
		for _, product := range products {
			productIDs = append(productIDs, product.ID)
		}
		result.Products = int64(len(productIDs))

		var reserved int64
		if err := tx.Model(&model.CommerceProductCard{}).
			Where("product_id IN ? AND status = ?", productIDs, model.ProductCardReserved).
			Count(&reserved).Error; err != nil {
			return err
		}
		if reserved > 0 {
			return errCommerceProductReservedInventory
		}

		if err := tx.Model(&model.CommerceProductCard{}).
			Where("product_id IN ? AND status = ?", productIDs, model.ProductCardSold).
			Count(&result.PreservedSold).Error; err != nil {
			return err
		}

		var available []model.CommerceProductCard
		if err := tx.Where("product_id IN ? AND status = ?", productIDs, model.ProductCardAvailable).
			Order("id ASC").Find(&available).Error; err != nil {
			return err
		}
		cardIDs := make([]uint, 0, len(available))
		for _, relation := range available {
			cardIDs = append(cardIDs, relation.CardID)
		}

		if len(cardIDs) > 0 {
			var accountIDs []uint
			if err := tx.Model(&model.CardAccount{}).Where("card_id IN ?", cardIDs).Distinct().Pluck("account_id", &accountIDs).Error; err != nil {
				return err
			}
			if len(accountIDs) > 0 {
				if err := tx.Model(&model.Account{}).Where("id IN ?", accountIDs).Updates(map[string]interface{}{"used": false, "used_at": nil}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("card_id IN ?", cardIDs).Delete(&model.CardAccount{}).Error; err != nil {
				return err
			}
			if err := tx.Where("product_id IN ? AND status = ?", productIDs, model.ProductCardAvailable).
				Delete(&model.CommerceProductCard{}).Error; err != nil {
				return err
			}
			deleted := tx.Where("id IN ?", cardIDs).Delete(&model.Card{})
			if deleted.Error != nil {
				return deleted.Error
			}
			result.Deleted = deleted.RowsAffected
		}

		var soldProductIDs []uint
		if err := tx.Model(&model.CommerceProductCard{}).
			Where("product_id IN ? AND status = ?", productIDs, model.ProductCardSold).
			Distinct().Pluck("product_id", &soldProductIDs).Error; err != nil {
			return err
		}
		if len(soldProductIDs) > 0 {
			if err := tx.Model(&model.CommerceProduct{}).Where("id IN ?", soldProductIDs).Update("active", false).Error; err != nil {
				return err
			}
		}
		if len(soldProductIDs) < len(productIDs) {
			unsoldProductIDs := make([]uint, 0, len(productIDs)-len(soldProductIDs))
			soldSet := make(map[uint]struct{}, len(soldProductIDs))
			for _, id := range soldProductIDs {
				soldSet[id] = struct{}{}
			}
			for _, id := range productIDs {
				if _, ok := soldSet[id]; !ok {
					unsoldProductIDs = append(unsoldProductIDs, id)
				}
			}
			if len(unsoldProductIDs) > 0 {
				if err := tx.Unscoped().Where("id IN ?", unsoldProductIDs).Delete(&model.CommerceProduct{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	return result, err
}

func DelistCommerceProductGroupCards(c *gin.Context) {
	var req struct {
		Amount       int64  `json:"amount"`
		Subscription string `json:"subscription" binding:"required"`
		AccountCount int    `json:"account_count" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < 0 || strings.TrimSpace(req.Subscription) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "商城价格档位参数无效"})
		return
	}
	result, err := delistCommerceProductGroupStock(database.DB, req.Amount, req.Subscription, req.AccountCount)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "商城价格档位不存在"})
		return
	}
	if errors.Is(err, errCommerceProductReservedInventory) {
		c.JSON(http.StatusConflict, gin.H{"code": 1, "message": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	AddOpLogWithCtx(c, "commerce_delist", "下架商城价格档位 "+req.Subscription+" / "+strconv.Itoa(req.AccountCount)+" 个账号 / "+strconv.FormatInt(req.Amount, 10)+" 分，涉及商品 "+strconv.FormatInt(result.Products, 10)+" 个，删除未售卡密 "+strconv.FormatInt(result.Deleted, 10)+" 张", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "价格档位已下架，未售库存已删除", "data": gin.H{"deleted": result.Deleted, "preservedSold": result.PreservedSold, "products": result.Products}})
}
