package handler

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"
	"gorm.io/gorm"
)

type epayNotification struct {
	OrderNo string
	TradeNo string
	Amount  int64
}

func startEpayPayment(order *model.CommerceOrder, channel *model.CommercePaymentChannel, origin string) error {
	if channel.ChannelType != "third_party" {
		return fmt.Errorf("%s 支付接口尚未接入真实下单", channel.Name)
	}
	config := parseJSON[commercePrivateConfig](channel.PrivateConfig)
	submitURL, fields, err := buildEpayPaymentRequest(*order, channel.ID, config, origin)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	order.PayURL = submitURL
	order.PayPayload = string(payload)
	return database.DB.Save(order).Error
}

func requestOrigin(c *gin.Context) string {
	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		proto = "http"
		if c.Request.TLS != nil {
			proto = "https"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	return proto + "://" + host
}

func releaseCommerceOrder(orderID uint) {
	_ = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.CommerceProductCard{}).Where("order_id = ? AND status = ?", orderID, model.ProductCardReserved).Updates(map[string]interface{}{"status": model.ProductCardAvailable, "order_id": 0}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.CommerceOrder{}, orderID).Error
	})
}

func handleEpayCallback(c *gin.Context, channel model.CommercePaymentChannel) {
	config := parseJSON[commercePrivateConfig](channel.PrivateConfig)
	notification, err := verifyEpayNotification(c.Request.URL.Query(), config)
	if err != nil {
		c.String(400, "fail")
		return
	}
	var order model.CommerceOrder
	if database.DB.Where("order_no = ? AND channel_id = ?", notification.OrderNo, channel.ID).First(&order).Error != nil || order.Amount != notification.Amount {
		c.String(400, "fail")
		return
	}
	eventID := "epay:" + notification.TradeNo
	var event model.CommercePaymentEvent
	if database.DB.Where("channel_id = ? AND event_id = ?", channel.ID, eventID).First(&event).Error != nil {
		if err := database.DB.Create(&model.CommercePaymentEvent{ChannelID: channel.ID, EventID: eventID, OrderNo: notification.OrderNo, Payload: c.Request.URL.RawQuery}).Error; err != nil {
			var existing model.CommercePaymentEvent
			if database.DB.Where("channel_id = ? AND event_id = ?", channel.ID, eventID).First(&existing).Error != nil {
				c.String(500, "fail")
				return
			}
		}
	}
	fulfilled, err := confirmAndFulfillCommerceOrder(database.DB, notification.OrderNo, notification.TradeNo, "epay")
	if err != nil {
		c.String(500, "fail")
		return
	}
	if fulfilled != nil {
		go sendCommerceCompletionEmail(*fulfilled)
	}
	c.String(200, "success")
}

func buildEpayPaymentRequest(order model.CommerceOrder, channelID uint, config commercePrivateConfig, origin string) (string, url.Values, error) {
	if strings.TrimSpace(config.EpayBaseURL) == "" || strings.TrimSpace(config.EpayPID) == "" || config.EpayKey == "" {
		return "", nil, errors.New("易支付缺少网关地址、PID 或商户密钥")
	}
	payType := strings.TrimSpace(config.EpayPayType)
	if payType != "alipay" && payType != "wxpay" && payType != "qqpay" {
		return "", nil, errors.New("易支付支付方式无效")
	}
	if !strings.EqualFold(order.Currency, "CNY") || order.Amount < 0 {
		return "", nil, errors.New("易支付仅支持人民币订单")
	}

	endpoint, err := url.Parse(strings.TrimSpace(config.EpayBaseURL))
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "", nil, errors.New("易支付网关地址无效")
	}
	if !strings.HasSuffix(strings.ToLower(endpoint.Path), "/submit.php") {
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/submit.php"
	}
	endpoint.RawQuery = ""
	endpoint.Fragment = ""

	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" || originURL.Host == "" {
		return "", nil, errors.New("商城公开访问地址无效")
	}

	fields := url.Values{
		"pid":          {strings.TrimSpace(config.EpayPID)},
		"type":         {payType},
		"out_trade_no": {order.OrderNo},
		"notify_url":   {fmt.Sprintf("%s/api/shop/callback/%d", origin, channelID)},
		"return_url":   {fmt.Sprintf("%s/?orderNo=%s", origin, url.QueryEscape(order.OrderNo))},
		"name":         {order.ProductName},
		"money":        {formatMinorAmount(order.Amount)},
		"sitename":     {"KiroClaim"},
	}
	fields.Set("sign", epaySign(fields, config.EpayKey))
	fields.Set("sign_type", "MD5")
	return endpoint.String(), fields, nil
}

func epaySign(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for name := range values {
		if name == "sign" || name == "sign_type" || values.Get(name) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+values.Get(name))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:])
}

func verifyEpayNotification(values url.Values, config commercePrivateConfig) (epayNotification, error) {
	var notification epayNotification
	if values.Get("pid") == "" || values.Get("trade_no") == "" || values.Get("out_trade_no") == "" || values.Get("money") == "" || values.Get("sign") == "" {
		return notification, errors.New("易支付通知参数不完整")
	}
	if values.Get("pid") != strings.TrimSpace(config.EpayPID) {
		return notification, errors.New("易支付商户号不匹配")
	}
	if values.Get("sign_type") != "" && !strings.EqualFold(values.Get("sign_type"), "MD5") {
		return notification, errors.New("易支付签名类型不支持")
	}
	if values.Get("trade_status") != "TRADE_SUCCESS" {
		return notification, errors.New("易支付订单未支付成功")
	}
	if config.EpayPayType != "" && values.Get("type") != config.EpayPayType {
		return notification, errors.New("易支付支付方式不匹配")
	}
	if !strings.EqualFold(epaySign(values, config.EpayKey), values.Get("sign")) {
		return notification, errors.New("易支付签名验证失败")
	}
	amount, err := parseEpayAmount(values.Get("money"))
	if err != nil {
		return notification, err
	}
	notification.OrderNo = values.Get("out_trade_no")
	notification.TradeNo = values.Get("trade_no")
	notification.Amount = amount
	return notification, nil
}

func parseEpayAmount(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") {
		return 0, errors.New("易支付金额无效")
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("易支付金额无效")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, errors.New("易支付金额无效")
	}
	decimal := int64(0)
	if len(parts) == 2 {
		if parts[1] == "" || len(parts[1]) > 2 {
			return 0, errors.New("易支付金额无效")
		}
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		decimal, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, errors.New("易支付金额无效")
		}
	}
	return whole*100 + decimal, nil
}

func formatMinorAmount(amount int64) string {
	return fmt.Sprintf("%d.%02d", amount/100, amount%100)
}
