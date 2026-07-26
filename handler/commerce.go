package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"
	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type commercePublicConfig struct {
	Instructions     string `json:"instructions"`
	QRImageURL       string `json:"qrImageUrl"`
	WalletAddress    string `json:"walletAddress"`
	RequireReference bool   `json:"requireReference"`
	RequirePayer     bool   `json:"requirePayer"`
	RequireNote      bool   `json:"requireNote"`
	RequireProof     bool   `json:"requireProof"`
}

type commercePrivateConfig struct {
	APIURL              string `json:"apiUrl"`
	Secret              string `json:"secret"`
	WeChatMchID         string `json:"wechatMchId,omitempty"`
	WeChatAppID         string `json:"wechatAppId,omitempty"`
	WeChatCertSerial    string `json:"wechatCertSerial,omitempty"`
	WeChatAPIV3Key      string `json:"wechatApiV3Key,omitempty"`
	WeChatPrivateKey    string `json:"wechatPrivateKey,omitempty"`
	AlipayAppID         string `json:"alipayAppId,omitempty"`
	AlipayPrivateKey    string `json:"alipayPrivateKey,omitempty"`
	AlipayPublicKey     string `json:"alipayPublicKey,omitempty"`
	EpayBaseURL         string `json:"epayBaseUrl,omitempty"`
	EpayPID             string `json:"epayPid,omitempty"`
	EpayKey             string `json:"epayKey,omitempty"`
	EpaySignType        string `json:"epaySignType,omitempty"`
	EpayPayType         string `json:"epayPayType,omitempty"`
	CryptoNetwork       string `json:"cryptoNetwork,omitempty"`
	CryptoWallet        string `json:"cryptoWallet,omitempty"`
	CryptoContract      string `json:"cryptoContract,omitempty"`
	CryptoAPIURL        string `json:"cryptoApiUrl,omitempty"`
	CryptoAPIKey        string `json:"cryptoApiKey,omitempty"`
	CryptoConfirmations int    `json:"cryptoConfirmations,omitempty"`
}

func commerceChannelTypeLabel(channelType string) string {
	return map[string]string{"manual": "人工收款", "wechat": "微信支付接口", "alipay": "支付宝接口", "third_party": "第三方支付接口", "crypto": "数字货币支付接口"}[channelType]
}

func commercePublicChannelName(channelType string, private commercePrivateConfig) string {
	if channelType == "third_party" {
		switch private.EpayPayType {
		case "alipay":
			return "支付宝"
		case "wxpay":
			return "微信支付"
		case "qqpay":
			return "QQ钱包"
		}
		return "易支付"
	}
	return map[string]string{"manual": "人工收款", "wechat": "微信支付", "alipay": "支付宝", "crypto": "USDT-TRC20"}[channelType]
}

func commerceOrderChannelPayload(channel model.CommercePaymentChannel) gin.H {
	private := parseJSON[commercePrivateConfig](channel.PrivateConfig)
	return gin.H{
		"name":         commercePublicChannelName(channel.ChannelType, private),
		"type":         channel.ChannelType,
		"publicConfig": parseJSON[commercePublicConfig](channel.PublicConfig),
	}
}

type commerceChannelSetting struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	ChannelType   string `json:"channelType"`
	Active        bool   `json:"active"`
	SortOrder     int    `json:"sortOrder"`
	PublicConfig  string `json:"publicConfig"`
	PrivateConfig string `json:"privateConfig"`
}

type commerceSettings struct {
	Enabled              bool                     `json:"enabled"`
	DefaultExpiryMinutes int                      `json:"defaultExpiryMinutes"`
	ManualExpiryMinutes  int                      `json:"manualExpiryMinutes"`
	StorageType          string                   `json:"storageType"`
	LocalPath            string                   `json:"localPath"`
	S3Endpoint           string                   `json:"s3Endpoint"`
	S3Region             string                   `json:"s3Region"`
	S3Bucket             string                   `json:"s3Bucket"`
	S3AccessKey          string                   `json:"s3AccessKey"`
	S3SecretKey          string                   `json:"s3SecretKey"`
	S3UseSSL             bool                     `json:"s3UseSSL"`
	MaxProofMB           int64                    `json:"maxProofMB"`
	SMTPHost             string                   `json:"smtpHost"`
	SMTPPort             int                      `json:"smtpPort"`
	SMTPUsername         string                   `json:"smtpUsername"`
	SMTPPassword         string                   `json:"smtpPassword"`
	SMTPFrom             string                   `json:"smtpFrom"`
	SMTPTLSMode          string                   `json:"smtpTlsMode"`
	SMTPUseTLS           bool                     `json:"smtpUseTls"`
	EmailOnComplete      bool                     `json:"emailOnComplete"`
	EmailIncludeCard     bool                     `json:"emailIncludeCard"`
	Channels             []commerceChannelSetting `json:"channels"`
}

func defaultCommerceSettings() commerceSettings {
	return commerceSettings{Enabled: true, DefaultExpiryMinutes: 30, ManualExpiryMinutes: 1440, StorageType: "local", LocalPath: "data/payment-proofs", MaxProofMB: 5, SMTPPort: 587, SMTPTLSMode: "starttls", EmailOnComplete: false}
}

func loadCommerceSettings() commerceSettings {
	s := defaultCommerceSettings()
	if database.DB == nil {
		return s
	}
	var kv model.KV
	if database.WhereKVKey(database.DB, model.KVCommerceSettings).First(&kv).Error == nil {
		raw := utils.Decrypt(kv.Value)
		_ = json.Unmarshal([]byte(raw), &s)
	}
	if s.DefaultExpiryMinutes <= 0 {
		s.DefaultExpiryMinutes = 30
	}
	if s.ManualExpiryMinutes <= 0 {
		s.ManualExpiryMinutes = 1440
	}
	if s.MaxProofMB <= 0 {
		s.MaxProofMB = 5
	}
	if s.LocalPath == "" {
		s.LocalPath = "data/payment-proofs"
	}
	return s
}

func commerceChannelModel(channel commerceChannelSetting) model.CommercePaymentChannel {
	return model.CommercePaymentChannel{Model: gorm.Model{ID: channel.ID}, Name: commerceChannelTypeLabel(channel.ChannelType), ChannelType: channel.ChannelType, Active: channel.Active, SortOrder: channel.SortOrder, PublicConfig: channel.PublicConfig, PrivateConfig: channel.PrivateConfig}
}
func findCommerceChannel(id uint, activeOnly bool) (*model.CommercePaymentChannel, bool) {
	s := loadCommerceSettings()
	for _, ch := range s.Channels {
		if ch.ID == id && (!activeOnly || ch.Active) {
			row := commerceChannelModel(ch)
			return &row, true
		}
	}
	return nil, false
}

func saveCommerceSettings(s commerceSettings) error {
	if s.SMTPTLSMode == "" {
		if s.SMTPUseTLS {
			s.SMTPTLSMode = "tls"
		} else {
			s.SMTPTLSMode = "none"
		}
	}
	sort.SliceStable(s.Channels, func(i, j int) bool {
		if s.Channels[i].SortOrder == s.Channels[j].SortOrder {
			return s.Channels[i].ID < s.Channels[j].ID
		}
		return s.Channels[i].SortOrder < s.Channels[j].SortOrder
	})
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return database.DB.Save(&model.KV{Key: model.KVCommerceSettings, Value: utils.Encrypt(string(b))}).Error
}

func parseJSON[T any](raw string) T {
	var value T
	_ = json.Unmarshal([]byte(raw), &value)
	return value
}

func PublicShopProducts(c *gin.Context) {
	s := loadCommerceSettings()
	if !s.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 1, "message": "商城未开启"})
		return
	}
	var products []model.CommerceProduct
	database.DB.Where("active = ?", true).Order("id ASC").Find(&products)
	settings := loadCommerceSettings()
	channelItems := make([]gin.H, 0, len(settings.Channels))
	for _, configured := range settings.Channels {
		if configured.Active {
			private := parseJSON[commercePrivateConfig](configured.PrivateConfig)
			channelItems = append(channelItems, gin.H{"id": configured.ID, "name": commercePublicChannelName(configured.ChannelType, private), "type": configured.ChannelType, "publicConfig": parseJSON[commercePublicConfig](configured.PublicConfig)})
		}
	}
	productItems := make([]gin.H, 0, len(products))
	for _, p := range products {
		var stock int64
		database.DB.Model(&model.CommerceProductCard{}).Where("product_id = ? AND status = ?", p.ID, model.ProductCardAvailable).Count(&stock)
		productItems = append(productItems, gin.H{"id": p.ID, "name": p.Name, "description": p.Description, "imageData": p.ImageData, "amount": p.BaseAmount, "currency": p.BaseCurrency, "subscription": p.Subscription, "accountCount": p.AccountCount, "stock": stock})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"products": productItems, "channels": channelItems}})
}

func CreateShopOrder(c *gin.Context) {
	settings := loadCommerceSettings()
	if !settings.Enabled {
		c.JSON(503, gin.H{"code": 1, "message": "商城未开启"})
		return
	}
	var req struct {
		ProductID     uint   `json:"productId"`
		ChannelID     uint   `json:"channelId"`
		Contact       string `json:"contact"`
		QueryPassword string `json:"queryPassword"`
		Quantity      int    `json:"quantity"`
	}
	if c.ShouldBindJSON(&req) != nil || req.ProductID == 0 || req.ChannelID == 0 || req.Quantity < 1 || strings.TrimSpace(req.Contact) == "" {
		c.JSON(400, gin.H{"code": 1, "message": "参数不完整"})
		return
	}
	queryPassword := strings.TrimSpace(req.QueryPassword)
	if len(queryPassword) < 6 || len([]byte(queryPassword)) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "查询订单密码长度需为 6 到 72 位"})
		return
	}
	var product model.CommerceProduct
	if database.DB.Where("id = ? AND active = ?", req.ProductID, true).First(&product).Error != nil {
		c.JSON(404, gin.H{"code": 1, "message": "商品不存在"})
		return
	}
	channelPtr, ok := findCommerceChannel(req.ChannelID, true)
	if !ok {
		c.JSON(404, gin.H{"code": 1, "message": "支付渠道不存在"})
		return
	}
	channel := *channelPtr
	expiry := settings.DefaultExpiryMinutes
	if channel.ChannelType == "manual" {
		expiry = settings.ManualExpiryMinutes
	}
	order, password, err := createCommerceOrder(database.DB, product, &channel, req.Contact, queryPassword, req.Quantity, expiry)
	if err != nil {
		c.JSON(409, gin.H{"code": 1, "message": "库存不足或订单创建失败: " + err.Error()})
		return
	}
	if channel.ChannelType != "manual" {
		if err := startEpayPayment(order, &channel, requestOrigin(c)); err != nil {
			releaseCommerceOrder(order.ID)
			c.JSON(http.StatusBadGateway, gin.H{"code": 1, "message": "支付初始化失败: " + err.Error()})
			return
		}
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"orderNo": order.OrderNo, "queryPassword": password, "status": order.Status, "quantity": order.Quantity, "amount": order.Amount, "currency": order.Currency, "expiresAt": order.ExpiresAt, "payUrl": order.PayURL, "payPayload": order.PayPayload, "channel": commerceOrderChannelPayload(channel)}})
}

func verifyOrderAccess(order *model.CommerceOrder, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(order.QueryPasswordHash), []byte(password)) == nil
}

func QueryShopOrder(c *gin.Context) {
	var req struct {
		OrderNo  string `json:"orderNo"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 1, "message": "参数错误"})
		return
	}
	expireCommerceOrders()
	var order model.CommerceOrder
	if database.DB.Where("order_no=?", strings.TrimSpace(req.OrderNo)).First(&order).Error != nil || !verifyOrderAccess(&order, req.Password) {
		c.JSON(404, gin.H{"code": 1, "message": "订单号或查询密码错误"})
		return
	}
	data := commerceOrderPayload(order, true)
	c.JSON(200, gin.H{"code": 0, "data": data})
}

func commerceOrderPayload(order model.CommerceOrder, includeCard bool) gin.H {
	data := gin.H{"orderNo": order.OrderNo, "productName": order.ProductName, "quantity": order.Quantity, "status": order.Status, "amount": order.Amount, "currency": order.Currency, "expiresAt": order.ExpiresAt, "paidAt": order.PaidAt, "completedAt": order.CompletedAt, "reviewNote": order.ReviewNote, "payUrl": order.PayURL, "payPayload": order.PayPayload}
	if includeCard {
		if codes, err := commerceOrderCardCodes(database.DB, order.ID); err == nil {
			data["cardCodes"] = codes
		}
	}
	return data
}

func startGatewayPayment(order *model.CommerceOrder, channel *model.CommercePaymentChannel) error {
	config := parseJSON[commercePrivateConfig](channel.PrivateConfig)
	if config.APIURL == "" || config.Secret == "" {
		return errors.New("自动支付渠道缺少 API URL 或密钥")
	}
	payload := gin.H{"order_no": order.OrderNo, "amount": order.Amount, "currency": order.Currency, "subject": order.ProductName, "notify_url": fmt.Sprintf("/api/shop/callback/%d", channel.ID), "expires_at": order.ExpiresAt.Unix()}
	body, _ := json.Marshal(payload)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sig := commerceHMAC(config.Secret, timestamp, body)
	req, err := http.NewRequest(http.MethodPost, config.APIURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kiro-Timestamp", timestamp)
	req.Header.Set("X-Kiro-Signature", sig)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gateway status %d", resp.StatusCode)
	}
	var result struct {
		ProviderOrderID string `json:"provider_order_id"`
		PayURL          string `json:"pay_url"`
		PayPayload      string `json:"pay_payload"`
	}
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return errors.New("invalid gateway response")
	}
	order.ProviderOrderID = result.ProviderOrderID
	order.PayURL = result.PayURL
	order.PayPayload = result.PayPayload
	return database.DB.Save(order).Error
}

func commerceHMAC(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp))
	m.Write([]byte("."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func ShopPaymentCallback(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("channelId"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	channelPtr, ok := findCommerceChannel(uint(id64), true)
	if !ok {
		c.JSON(404, gin.H{"code": 1})
		return
	}
	channel := *channelPtr
	if channel.ChannelType == "third_party" {
		handleEpayCallback(c, channel)
		return
	}
	c.String(http.StatusNotImplemented, "unsupported payment channel")
	return
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	timestamp := c.GetHeader("X-Kiro-Timestamp")
	ts, _ := strconv.ParseInt(timestamp, 10, 64)
	if time.Since(time.Unix(ts, 0)) > 5*time.Minute || time.Until(time.Unix(ts, 0)) > time.Minute {
		c.JSON(401, gin.H{"code": 1, "message": "回调已过期"})
		return
	}
	secret := parseJSON[commercePrivateConfig](channel.PrivateConfig).Secret
	expected := commerceHMAC(secret, timestamp, body)
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(c.GetHeader("X-Kiro-Signature")))) {
		c.JSON(401, gin.H{"code": 1, "message": "签名错误"})
		return
	}
	var event struct {
		EventID       string `json:"event_id"`
		OrderNo       string `json:"order_no"`
		TransactionID string `json:"transaction_id"`
		Status        string `json:"status"`
		Amount        int64  `json:"amount"`
		Currency      string `json:"currency"`
	}
	if json.Unmarshal(body, &event) != nil || event.EventID == "" || event.Status != "paid" {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	var order model.CommerceOrder
	if database.DB.Where("order_no=? AND channel_id=?", event.OrderNo, channel.ID).First(&order).Error != nil || order.Amount != event.Amount || !strings.EqualFold(order.Currency, event.Currency) {
		c.JSON(409, gin.H{"code": 1, "message": "订单金额或币种不匹配"})
		return
	}
	if err := database.DB.Create(&model.CommercePaymentEvent{ChannelID: channel.ID, EventID: event.EventID, OrderNo: event.OrderNo, Payload: string(body)}).Error; err != nil {
		c.JSON(200, gin.H{"code": 0, "message": "duplicate"})
		return
	}
	fulfilled, err := confirmAndFulfillCommerceOrder(database.DB, event.OrderNo, event.TransactionID, "gateway:"+channel.Name)
	if err != nil {
		c.JSON(500, gin.H{"code": 1, "message": err.Error()})
		return
	}
	go sendCommerceCompletionEmail(*fulfilled)
	c.JSON(200, gin.H{"code": 0})
}

func expireCommerceOrders() {
	now := time.Now()
	var orders []model.CommerceOrder
	database.DB.Where("status=? AND expires_at < ?", model.OrderPendingPayment, now).Find(&orders)
	for _, o := range orders {
		database.DB.Transaction(func(tx *gorm.DB) error {
			tx.Model(&model.CommerceOrder{}).Where("id=? AND status=?", o.ID, model.OrderPendingPayment).Update("status", model.OrderExpired)
			tx.Model(&model.CommerceProductCard{}).Where("order_id=? AND status=?", o.ID, model.ProductCardReserved).Updates(map[string]interface{}{"status": model.ProductCardAvailable, "order_id": 0})
			return nil
		})
	}
}

func AdminCommerceChannels(c *gin.Context) {
	settings := loadCommerceSettings()
	if c.Request.Method == http.MethodGet {
		items := make([]gin.H, 0, len(settings.Channels))
		for _, r := range settings.Channels {
			public := parseJSON[commercePublicConfig](r.PublicConfig)
			private := parseJSON[commercePrivateConfig](r.PrivateConfig)
			items = append(items, gin.H{"id": r.ID, "channelType": r.ChannelType, "active": r.Active, "sortOrder": r.SortOrder, "instructions": public.Instructions, "qrImageUrl": public.QRImageURL, "walletAddress": public.WalletAddress, "requireReference": public.RequireReference, "requirePayer": public.RequirePayer, "requireNote": public.RequireNote, "requireProof": public.RequireProof, "wechatMchId": private.WeChatMchID, "wechatAppId": private.WeChatAppID, "wechatCertSerial": private.WeChatCertSerial, "wechatApiV3KeyConfigured": private.WeChatAPIV3Key != "", "wechatPrivateKeyConfigured": private.WeChatPrivateKey != "", "alipayAppId": private.AlipayAppID, "alipayPrivateKeyConfigured": private.AlipayPrivateKey != "", "alipayPublicKeyConfigured": private.AlipayPublicKey != "", "epayBaseUrl": private.EpayBaseURL, "epayPid": private.EpayPID, "epaySignType": private.EpaySignType, "epayPayType": private.EpayPayType, "epayKeyConfigured": private.EpayKey != "", "cryptoNetwork": private.CryptoNetwork, "cryptoWallet": private.CryptoWallet, "cryptoContract": private.CryptoContract, "cryptoApiUrl": private.CryptoAPIURL, "cryptoApiKeyConfigured": private.CryptoAPIKey != "", "cryptoConfirmations": private.CryptoConfirmations})
		}
		c.JSON(200, gin.H{"code": 0, "data": items})
		return
	}
	var req struct {
		ID                  uint   `json:"id"`
		ChannelType         string `json:"channelType"`
		Active              bool   `json:"active"`
		SortOrder           int    `json:"sortOrder"`
		Instructions        string `json:"instructions"`
		QRImageURL          string `json:"qrImageUrl"`
		WalletAddress       string `json:"walletAddress"`
		RequireReference    bool   `json:"requireReference"`
		RequirePayer        bool   `json:"requirePayer"`
		RequireNote         bool   `json:"requireNote"`
		RequireProof        bool   `json:"requireProof"`
		WeChatMchID         string `json:"wechatMchId"`
		WeChatAppID         string `json:"wechatAppId"`
		WeChatCertSerial    string `json:"wechatCertSerial"`
		WeChatAPIV3Key      string `json:"wechatApiV3Key"`
		WeChatPrivateKey    string `json:"wechatPrivateKey"`
		AlipayAppID         string `json:"alipayAppId"`
		AlipayPrivateKey    string `json:"alipayPrivateKey"`
		AlipayPublicKey     string `json:"alipayPublicKey"`
		EpayBaseURL         string `json:"epayBaseUrl"`
		EpayPID             string `json:"epayPid"`
		EpayKey             string `json:"epayKey"`
		EpaySignType        string `json:"epaySignType"`
		EpayPayType         string `json:"epayPayType"`
		CryptoNetwork       string `json:"cryptoNetwork"`
		CryptoWallet        string `json:"cryptoWallet"`
		CryptoContract      string `json:"cryptoContract"`
		CryptoAPIURL        string `json:"cryptoApiUrl"`
		CryptoAPIKey        string `json:"cryptoApiKey"`
		CryptoConfirmations int    `json:"cryptoConfirmations"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	validTypes := map[string]bool{"manual": true, "wechat": true, "alipay": true, "third_party": true, "crypto": true}
	if !validTypes[req.ChannelType] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "支付类型无效"})
		return
	}
	index := -1
	maxID := uint(0)
	for i, row := range settings.Channels {
		if row.ID > maxID {
			maxID = row.ID
		}
		if row.ID == req.ID && req.ID > 0 {
			index = i
		}
	}
	var maxOrderChannelID uint
	database.DB.Model(&model.CommerceOrder{}).Select("COALESCE(MAX(channel_id), 0)").Scan(&maxOrderChannelID)
	if maxOrderChannelID > maxID {
		maxID = maxOrderChannelID
	}
	if index < 0 {
		req.ID = maxID + 1
		settings.Channels = append(settings.Channels, commerceChannelSetting{})
		index = len(settings.Channels) - 1
	}
	oldPrivate := parseJSON[commercePrivateConfig](settings.Channels[index].PrivateConfig)
	if req.WeChatAPIV3Key == "" {
		req.WeChatAPIV3Key = oldPrivate.WeChatAPIV3Key
	}
	if req.WeChatPrivateKey == "" {
		req.WeChatPrivateKey = oldPrivate.WeChatPrivateKey
	}
	if req.AlipayPrivateKey == "" {
		req.AlipayPrivateKey = oldPrivate.AlipayPrivateKey
	}
	if req.AlipayPublicKey == "" {
		req.AlipayPublicKey = oldPrivate.AlipayPublicKey
	}
	if req.EpayKey == "" {
		req.EpayKey = oldPrivate.EpayKey
	}
	if req.CryptoAPIKey == "" {
		req.CryptoAPIKey = oldPrivate.CryptoAPIKey
	}
	if req.ChannelType == "manual" {
		req.EpayBaseURL, req.EpayPID, req.EpayKey = "", "", ""
	}
	if req.ChannelType != "manual" {
		req.Instructions = ""
		req.QRImageURL = ""
		req.WalletAddress = ""
		req.RequireReference = false
		req.RequirePayer = false
		req.RequireNote = false
		req.RequireProof = false
	}
	publicJSON, _ := json.Marshal(commercePublicConfig{Instructions: strings.TrimSpace(req.Instructions), QRImageURL: strings.TrimSpace(req.QRImageURL), WalletAddress: strings.TrimSpace(req.WalletAddress), RequireReference: req.RequireReference, RequirePayer: req.RequirePayer, RequireNote: req.RequireNote, RequireProof: req.RequireProof})
	privateJSON, _ := json.Marshal(commercePrivateConfig{WeChatMchID: req.WeChatMchID, WeChatAppID: req.WeChatAppID, WeChatCertSerial: req.WeChatCertSerial, WeChatAPIV3Key: req.WeChatAPIV3Key, WeChatPrivateKey: req.WeChatPrivateKey, AlipayAppID: req.AlipayAppID, AlipayPrivateKey: req.AlipayPrivateKey, AlipayPublicKey: req.AlipayPublicKey, EpayBaseURL: req.EpayBaseURL, EpayPID: req.EpayPID, EpayKey: req.EpayKey, EpaySignType: req.EpaySignType, EpayPayType: req.EpayPayType, CryptoNetwork: req.CryptoNetwork, CryptoWallet: req.CryptoWallet, CryptoContract: req.CryptoContract, CryptoAPIURL: req.CryptoAPIURL, CryptoAPIKey: req.CryptoAPIKey, CryptoConfirmations: req.CryptoConfirmations})
	settings.Channels[index] = commerceChannelSetting{ID: req.ID, Name: commerceChannelTypeLabel(req.ChannelType), ChannelType: req.ChannelType, Active: req.Active, SortOrder: req.SortOrder, PublicConfig: string(publicJSON), PrivateConfig: string(privateJSON)}
	if err := saveCommerceSettings(settings); err != nil {
		c.JSON(500, gin.H{"code": 1, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"id": req.ID}})
}

func AdminCommerceChannelDelete(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "支付方式无效"})
		return
	}
	id := uint(id64)
	var orderCount int64
	database.DB.Model(&model.CommerceOrder{}).Where("channel_id = ?", id).Count(&orderCount)
	if orderCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 1, "message": "该支付方式已有关联订单，可停用但不能删除"})
		return
	}
	settings := loadCommerceSettings()
	found := false
	channels := make([]commerceChannelSetting, 0, len(settings.Channels))
	for _, channel := range settings.Channels {
		if channel.ID == id {
			found = true
			continue
		}
		channels = append(channels, channel)
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "支付方式不存在"})
		return
	}
	settings.Channels = channels
	if err := saveCommerceSettings(settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "支付方式已删除"})
}

func AdminCommerceOrders(c *gin.Context) {
	expireCommerceOrders()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	status := c.Query("status")
	q := database.DB.Model(&model.CommerceOrder{})
	if status != "" {
		q = q.Where("status=?", status)
	}
	var total int64
	var rows []model.CommerceOrder
	q.Count(&total)
	q.Order("id desc").Offset((page - 1) * 20).Limit(20).Find(&rows)
	c.JSON(200, gin.H{"code": 0, "data": gin.H{"total": total, "page": page, "list": rows}})
}

func AdminCommerceOrderDetail(c *gin.Context) {
	var order model.CommerceOrder
	if database.DB.Where("order_no = ?", c.Param("orderNo")).First(&order).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "订单不存在"})
		return
	}
	var proofs []model.CommercePaymentProof
	database.DB.Where("order_id = ?", order.ID).Order("id desc").Find(&proofs)
	items := make([]gin.H, 0, len(proofs))
	for _, proof := range proofs {
		var files []string
		_ = json.Unmarshal([]byte(proof.FilesJSON), &files)
		links := make([]string, len(files))
		for i := range files {
			links[i] = fmt.Sprintf("/admin/commerce/proofs/%d/%d", proof.ID, i)
		}
		items = append(items, gin.H{"id": proof.ID, "reference": proof.Reference, "payerInfo": proof.PayerInfo, "note": proof.Note, "files": links, "createdAt": proof.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"order": commerceOrderPayload(order, true), "proofs": items}})
}

func AdminCommerceReview(c *gin.Context) {
	var req struct {
		Action     string `json:"action"`
		Note       string `json:"note"`
		PaymentRef string `json:"paymentRef"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	var order model.CommerceOrder
	if database.DB.Where("order_no=?", c.Param("orderNo")).First(&order).Error != nil {
		c.JSON(404, gin.H{"code": 1})
		return
	}
	switch req.Action {
	case "approve", "fulfill":
		fulfilled, err := confirmAndFulfillCommerceOrder(database.DB, order.OrderNo, req.PaymentRef, "admin")
		if err != nil {
			c.JSON(409, gin.H{"code": 1, "message": err.Error()})
			return
		}
		go sendCommerceCompletionEmail(*fulfilled)
	case "reject":
		database.DB.Model(&order).Updates(map[string]interface{}{"status": model.OrderRejected, "review_note": req.Note})
		database.DB.Model(&model.CommerceProductCard{}).Where("order_id=? AND status=?", order.ID, model.ProductCardReserved).Updates(map[string]interface{}{"status": model.ProductCardAvailable, "order_id": 0})
	case "refund":
		database.DB.Model(&order).Updates(map[string]interface{}{"status": model.OrderRefunded, "review_note": req.Note})
	default:
		c.JSON(400, gin.H{"code": 1, "message": "未知操作"})
		return
	}
	AddOpLogWithCtx(c, "commerce_order", req.Action+" "+order.OrderNo, "admin")
	c.JSON(200, gin.H{"code": 0})
}

func AdminCommerceSettings(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		s := loadCommerceSettings()
		s.SMTPPassword = ""
		s.S3SecretKey = ""
		c.JSON(200, gin.H{"code": 0, "data": s})
		return
	}
	current := loadCommerceSettings()
	var req commerceSettings
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 1})
		return
	}
	if req.SMTPPassword == "" {
		req.SMTPPassword = current.SMTPPassword
	}
	if req.S3SecretKey == "" {
		req.S3SecretKey = current.S3SecretKey
	}
	if err := saveCommerceSettings(req); err != nil {
		c.JSON(500, gin.H{"code": 1, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0})
}
