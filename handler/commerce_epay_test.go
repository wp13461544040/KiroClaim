package handler

import (
	"net/url"
	"testing"

	"github.com/huey1in/KiroClaim/model"
)

func TestBuildEpayPaymentRequestUsesOfficialSubmitProtocol(t *testing.T) {
	order := model.CommerceOrder{OrderNo: "KC123", ProductName: "Test Product", Amount: 1, Currency: "CNY"}
	config := commercePrivateConfig{
		EpayBaseURL: "https://pay.example.com",
		EpayPID:     "1000",
		EpayKey:     "merchant-secret",
		EpayPayType: "wxpay",
	}

	submitURL, fields, err := buildEpayPaymentRequest(order, 7, config, "https://shop.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if submitURL != "https://pay.example.com/submit.php" {
		t.Fatalf("unexpected submit URL: %s", submitURL)
	}
	expected := map[string]string{
		"pid":          "1000",
		"type":         "wxpay",
		"out_trade_no": "KC123",
		"notify_url":   "https://shop.example.com/api/shop/callback/7",
		"return_url":   "https://shop.example.com/?orderNo=KC123",
		"name":         "Test Product",
		"money":        "0.01",
		"sitename":     "KiroClaim",
		"sign_type":    "MD5",
		"sign":         "1b8c0d8c04d4a52c4a21f0ef5bd159ea",
	}
	for key, value := range expected {
		if fields.Get(key) != value {
			t.Fatalf("unexpected %s: got %q want %q", key, fields.Get(key), value)
		}
	}
}

func TestVerifyEpayNotificationAcceptsSignedSuccessfulPayment(t *testing.T) {
	values := url.Values{
		"pid":          {"1000"},
		"trade_no":     {"EPAY456"},
		"out_trade_no": {"KC123"},
		"type":         {"wxpay"},
		"name":         {"Test Product"},
		"money":        {"0.01"},
		"trade_status": {"TRADE_SUCCESS"},
		"sign":         {"c9663147df5af85b56a55850c77ed265"},
		"sign_type":    {"MD5"},
	}
	notification, err := verifyEpayNotification(values, commercePrivateConfig{EpayPID: "1000", EpayKey: "merchant-secret", EpayPayType: "wxpay"})
	if err != nil {
		t.Fatal(err)
	}
	if notification.OrderNo != "KC123" || notification.TradeNo != "EPAY456" || notification.Amount != 1 {
		t.Fatalf("unexpected notification: %+v", notification)
	}
}

func TestVerifyEpayNotificationRejectsTamperedSignature(t *testing.T) {
	values := url.Values{
		"pid":          {"1000"},
		"trade_no":     {"EPAY456"},
		"out_trade_no": {"KC123"},
		"type":         {"wxpay"},
		"name":         {"Test Product"},
		"money":        {"9.99"},
		"trade_status": {"TRADE_SUCCESS"},
		"sign":         {"c9663147df5af85b56a55850c77ed265"},
		"sign_type":    {"MD5"},
	}
	if _, err := verifyEpayNotification(values, commercePrivateConfig{EpayPID: "1000", EpayKey: "merchant-secret", EpayPayType: "wxpay"}); err == nil {
		t.Fatal("expected tampered EasyPay notification to fail")
	}
}

func TestBuildEpayPaymentRequestRejectsMissingConfiguration(t *testing.T) {
	order := model.CommerceOrder{OrderNo: "KC123", ProductName: "Test Product", Amount: 1, Currency: "CNY"}
	if _, _, err := buildEpayPaymentRequest(order, 7, commercePrivateConfig{}, "https://shop.example.com"); err == nil {
		t.Fatal("expected missing EasyPay configuration to fail")
	}
}

func TestCommerceOrderChannelPayloadUsesConfiguredEpayMethodName(t *testing.T) {
	channel := model.CommercePaymentChannel{
		Name:          "第三方支付接口",
		ChannelType:   "third_party",
		PrivateConfig: `{"epayPayType":"alipay"}`,
	}
	payload := commerceOrderChannelPayload(channel)
	if payload["name"] != "支付宝" || payload["type"] != "third_party" {
		t.Fatalf("unexpected channel payload: %#v", payload)
	}
}
