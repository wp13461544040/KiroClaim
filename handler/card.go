package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type cardListItem struct {
	ID                 uint
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Code               string
	UsedAt             *time.Time
	AccountCount       int
	Subscription       string
	Status             string
	ShopListed         bool
	ShopProductID      uint
	ShopProductName    string
	ShopPrice          int64
	ShopCurrency       string
	ShopProductActive  bool
	ShopRelationStatus string
}

func buildCardListItem(card model.Card) cardListItem {
	return cardListItem{
		ID:           card.ID,
		CreatedAt:    card.CreatedAt,
		UpdatedAt:    card.UpdatedAt,
		Code:         card.Code,
		UsedAt:       card.UsedAt,
		AccountCount: card.AccountCount,
		Subscription: card.Subscription,
		Status:       cardStatusFromUsedAt(card.UsedAt),
	}
}

func GenerateCards(c *gin.Context) {
	var req struct {
		Count        int    `json:"count" binding:"required,min=1,max=500"`
		Subscription string `json:"subscription"`
		AccountCount int    `json:"account_count" binding:"required,min=1"`
		ListOnShop   bool   `json:"list_on_shop"`
		Price        int64  `json:"price"`
		ImageData    string `json:"image_data"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error()})
		return
	}

	subscription := strings.TrimSpace(req.Subscription)
	if subscription == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请选择账号订阅"})
		return
	}
	var subscriptionCount int64
	if err := database.DB.Model(&model.Account{}).Where("subscription = ?", subscription).Count(&subscriptionCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "订阅校验失败: " + err.Error()})
		return
	}
	if subscriptionCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "账号订阅不存在"})
		return
	}
	if req.ListOnShop && req.Price < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "商城售价不能小于 0"})
		return
	}
	if req.ListOnShop {
		if err := validateCommerceProductImage(req.ImageData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error()})
			return
		}
	}

	codes := make([]string, 0, req.Count)
	var product model.CommerceProduct
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if req.ListOnShop {
			product = model.CommerceProduct{Name: fmt.Sprintf("%s %d 账号卡密", subscription, req.AccountCount), ImageData: req.ImageData, Active: true, BaseAmount: req.Price, BaseCurrency: "CNY", Subscription: subscription, AccountCount: req.AccountCount}
			if err := tx.Create(&product).Error; err != nil {
				return err
			}
		}
		for i := 0; i < req.Count; i++ {
			code := "KIRO-" + generateCode("upper", 12, "-", 4)
			card := model.Card{Code: code, AccountCount: req.AccountCount, Subscription: subscription}
			if err := tx.Create(&card).Error; err != nil {
				return err
			}
			if req.ListOnShop {
				if err := tx.Create(&model.CommerceProductCard{ProductID: product.ID, CardID: card.ID, Status: model.ProductCardAvailable}).Error; err != nil {
					return err
				}
			}
			codes = append(codes, code)
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "写入失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "生成成功", "data": gin.H{"codes": codes, "count": len(codes), "shopListed": req.ListOnShop, "productId": product.ID}})
	AddOpLogWithCtx(c, "generate", "生成卡密 "+strconv.Itoa(len(codes))+" 张", "admin")
}

func validateCommerceProductImage(value string) error {
	if value == "" {
		return nil
	}
	const maxImageBytes = 2 * 1024 * 1024
	prefixes := []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/webp;base64,"}
	encoded := ""
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			encoded = strings.TrimPrefix(value, prefix)
			break
		}
	}
	if encoded == "" {
		return errors.New("商城商品图片仅支持 PNG、JPEG 或 WebP")
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > maxImageBytes {
		return errors.New("商城商品图片不能超过 2 MB")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return errors.New("商城商品图片数据无效")
	}
	if len(decoded) > maxImageBytes {
		return errors.New("商城商品图片不能超过 2 MB")
	}
	contentType := http.DetectContentType(decoded)
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		return errors.New("商城商品图片数据无效")
	}
	return nil
}

func ListCards(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	statusFilter := c.Query("status")
	keyword := c.Query("keyword")
	createdFrom := c.Query("created_from")
	createdTo := c.Query("created_to")
	subscription := strings.TrimSpace(c.Query("subscription"))
	shopStatus := strings.TrimSpace(c.Query("shop_status"))
	shopProductID, _ := strconv.ParseUint(c.DefaultQuery("shop_product_id", "0"), 10, 64)
	shopPrice, shopPriceErr := strconv.ParseInt(c.Query("shop_price"), 10, 64)
	shopSubscription := strings.TrimSpace(c.Query("shop_subscription"))
	shopAccountCount, shopAccountCountErr := strconv.Atoi(c.Query("shop_account_count"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 1000 {
		size = 1000
	}
	if shopPriceErr != nil && c.Query("shop_price") != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "商城售价筛选无效"})
		return
	}
	if shopAccountCountErr != nil && c.Query("shop_account_count") != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "商城账号数量筛选无效"})
		return
	}
	if shopAccountCount < 0 {
		shopAccountCount = 0
	}
	if shopStatus != "" && shopStatus != "listed" && shopStatus != "unlisted" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "商城状态筛选无效"})
		return
	}

	var total int64
	var cards []model.Card
	q := database.DB.Model(&model.Card{})

	if statusFilter != "" {
		switch statusFilter {
		case cardStatusUnused:
			q = q.Where("used_at IS NULL")
		case cardStatusActive:
			q = q.Where("used_at IS NOT NULL")
		default:
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "状态只能是 unused / active"})
			return
		}
	}
	if keyword != "" {
		q = q.Where("code LIKE ?", "%"+keyword+"%")
	}
	if createdFrom != "" {
		if t, err := time.Parse("2006-01-02", createdFrom); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if createdTo != "" {
		if t, err := time.Parse("2006-01-02", createdTo); err == nil {
			q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}
	if subscription != "" {
		q = q.Where("subscription = ?", subscription)
	}
	shopCardIDs := database.DB.Model(&model.CommerceProductCard{}).Select("card_id")
	if shopProductID > 0 {
		q = q.Where("id IN (?)", shopCardIDs.Where("product_id = ?", uint(shopProductID)))
	}
	if (shopPriceErr == nil && c.Query("shop_price") != "") || shopSubscription != "" || shopAccountCount > 0 {
		shopGroupQuery := database.DB.Table("commerce_product_cards AS cpc").Select("cpc.card_id").Joins("JOIN commerce_products AS cp ON cp.id = cpc.product_id")
		if c.Query("shop_price") != "" {
			shopGroupQuery = shopGroupQuery.Where("cp.base_amount = ?", shopPrice)
		}
		if shopSubscription != "" {
			shopGroupQuery = shopGroupQuery.Where("cp.subscription = ?", shopSubscription)
		}
		if shopAccountCount > 0 {
			shopGroupQuery = shopGroupQuery.Where("cp.account_count = ?", shopAccountCount)
		}
		q = q.Where("id IN (?)", shopGroupQuery)
	}
	if shopStatus == "listed" {
		q = q.Where("id IN (?)", database.DB.Table("commerce_product_cards AS cpc").Select("cpc.card_id").Joins("JOIN commerce_products AS cp ON cp.id = cpc.product_id").Where("cp.active = ?", true))
	} else if shopStatus == "unlisted" {
		q = q.Where("id NOT IN (?)", database.DB.Table("commerce_product_cards AS cpc").Select("cpc.card_id").Joins("JOIN commerce_products AS cp ON cp.id = cpc.product_id").Where("cp.active = ?", true))
	}

	q.Count(&total)
	q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&cards)

	list := make([]cardListItem, 0, len(cards))
	cardIDs := make([]uint, 0, len(cards))
	for _, card := range cards {
		cardIDs = append(cardIDs, card.ID)
	}
	var shopRelations []model.CommerceProductCard
	if len(cardIDs) > 0 {
		database.DB.Where("card_id IN ?", cardIDs).Find(&shopRelations)
	}
	shopByCard := make(map[uint]model.CommerceProductCard, len(shopRelations))
	productIDs := make([]uint, 0, len(shopRelations))
	for _, relation := range shopRelations {
		shopByCard[relation.CardID] = relation
		productIDs = append(productIDs, relation.ProductID)
	}
	var shopProducts []model.CommerceProduct
	if len(productIDs) > 0 {
		database.DB.Where("id IN ?", productIDs).Find(&shopProducts)
	}
	productByID := make(map[uint]model.CommerceProduct, len(shopProducts))
	for _, product := range shopProducts {
		productByID[product.ID] = product
	}
	for _, card := range cards {
		item := buildCardListItem(card)
		if relation, ok := shopByCard[card.ID]; ok {
			item.ShopListed = true
			item.ShopProductID = relation.ProductID
			item.ShopRelationStatus = relation.Status
			if product, exists := productByID[relation.ProductID]; exists {
				item.ShopProductName = product.Name
				item.ShopPrice = product.BaseAmount
				item.ShopCurrency = product.BaseCurrency
				item.ShopProductActive = product.Active
			}
		}
		list = append(list, item)
	}

	var subscriptions []string
	database.DB.Model(&model.Card{}).Where("subscription <> ''").Distinct("subscription").Order("subscription ASC").Pluck("subscription", &subscriptions)
	var allProducts []model.CommerceProduct
	database.DB.Where("active = ?", true).Order("id DESC").Find(&allProducts)
	shopProductItems := make([]gin.H, 0, len(allProducts))
	shopPriceGroups := make([]gin.H, 0)
	groupIndex := make(map[string]int)
	for _, product := range allProducts {
		var available, reserved, sold int64
		database.DB.Model(&model.CommerceProductCard{}).Where("product_id = ? AND status = ?", product.ID, model.ProductCardAvailable).Count(&available)
		database.DB.Model(&model.CommerceProductCard{}).Where("product_id = ? AND status = ?", product.ID, model.ProductCardReserved).Count(&reserved)
		database.DB.Model(&model.CommerceProductCard{}).Where("product_id = ? AND status = ?", product.ID, model.ProductCardSold).Count(&sold)
		shopProductItems = append(shopProductItems, gin.H{"id": product.ID, "name": product.Name, "amount": product.BaseAmount, "currency": product.BaseCurrency, "active": product.Active, "available": available, "reserved": reserved, "sold": sold})

		groupKey := fmt.Sprintf("%d|%s|%d", product.BaseAmount, product.Subscription, product.AccountCount)
		if index, ok := groupIndex[groupKey]; ok {
			group := shopPriceGroups[index]
			group["products"] = group["products"].(int) + 1
			group["available"] = group["available"].(int64) + available
			group["reserved"] = group["reserved"].(int64) + reserved
			group["sold"] = group["sold"].(int64) + sold
			if product.Active {
				group["active"] = true
				group["activeProducts"] = group["activeProducts"].(int) + 1
			}
		} else {
			activeProducts := 0
			if product.Active {
				activeProducts = 1
			}
			groupIndex[groupKey] = len(shopPriceGroups)
			shopPriceGroups = append(shopPriceGroups, gin.H{
				"key": groupKey, "amount": product.BaseAmount, "currency": product.BaseCurrency,
				"subscription": product.Subscription, "accountCount": product.AccountCount,
				"products": 1, "activeProducts": activeProducts, "active": product.Active,
				"available": available, "reserved": reserved, "sold": sold,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"total": total, "page": page, "size": size, "list": list, "filters": gin.H{"subscriptions": subscriptions, "shopProducts": shopProductItems, "shopPriceGroups": shopPriceGroups},
	}})
}

func DeleteCard(c *gin.Context) {
	id := c.Param("id")
	cardID64, err := strconv.ParseUint(id, 10, 64)
	if err != nil || cardID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的 ID"})
		return
	}
	if err := releaseAccountsForCards([]uint{uint(cardID64)}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	if err := database.DB.Delete(&model.Card{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	AddOpLogWithCtx(c, "delete", "删除卡密 ID:"+id, "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除"})
}

func BatchDeleteCards(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请选择要删除的卡密"})
		return
	}
	if err := releaseAccountsForCards(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	result := database.DB.Where("id IN ?", req.IDs).Delete(&model.Card{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": result.Error.Error()})
		return
	}
	AddOpLogWithCtx(c, "delete", "批量删除卡密 "+strconv.Itoa(len(req.IDs))+" 张，实际删除 "+strconv.FormatInt(result.RowsAffected, 10)+" 张", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已删除", "data": gin.H{"deleted": result.RowsAffected}})
}

func generateCode(charset string, length int, separator string, groupSize int) string {
	var alphabet string
	switch charset {
	case "upper":
		alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	case "alnum":
		alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	default:
		b := make([]byte, (length+1)/2)
		rand.Read(b)
		raw := hex.EncodeToString(b)[:length]
		if separator == "" {
			return raw
		}
		return splitGroups(raw, groupSize, separator)
	}

	result := make([]byte, 0, length)
	buf := make([]byte, length*2)
	for len(result) < length {
		rand.Read(buf)
		for _, c := range buf {
			if len(result) >= length {
				break
			}
			idx := int(c) % len(alphabet)
			result = append(result, alphabet[idx])
		}
	}
	raw := string(result)
	if separator == "" {
		return raw
	}
	return splitGroups(raw, groupSize, separator)
}

func splitGroups(s string, size int, sep string) string {
	var parts []string
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		parts = append(parts, s[i:end])
	}
	return strings.Join(parts, sep)
}

func ListCardLogs(c *gin.Context) {
	cardID := c.Param("id")
	var logs []model.CardLog
	database.DB.Where("card_id = ?", cardID).Order("id desc").Find(&logs)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": logs})
}
