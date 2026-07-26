package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildUsageDetailPayloadParsesLiveUpstreamUsage(t *testing.T) {
	raw := map[string]interface{}{
		"daysUntilReset": nil,
		"nextDateReset":  1785542400.0,
		"subscriptionInfo": map[string]interface{}{
			"subscriptionTitle":            "KIRO FREE",
			"type":                         "Q_DEVELOPER_STANDALONE_FREE",
			"subscriptionManagementTarget": "PURCHASE",
			"upgradeCapability":            "UPGRADE_CAPABLE",
		},
		"usageBreakdownList": []interface{}{
			map[string]interface{}{
				"currentUsage":              0.0,
				"currentUsageWithPrecision": 0.5,
				"displayName":               "Credit",
				"displayNamePlural":         "Credits",
				"nextDateReset":             1785542400.0,
				"resourceType":              "CREDIT",
				"unit":                      "INVOCATIONS",
				"usageLimit":                50.0,
				"usageLimitWithPrecision":   50.0,
			},
		},
		"userInfo": map[string]interface{}{
			"email":  "CarlSwanson133104@outlook.com",
			"userId": "d-9067642ac7.644844d8-d0d1-7023-a97a-bec2d236de00",
		},
	}

	payload := buildUsageDetailPayload(7, &usageInfo{}, raw)
	if payload["id"] != uint(7) {
		t.Fatalf("id = %#v, want 7", payload["id"])
	}
	if payload["source"] != "upstream" {
		t.Fatalf("source = %#v, want upstream", payload["source"])
	}
	if payload["email"] != "CarlSwanson133104@outlook.com" {
		t.Fatalf("email = %#v", payload["email"])
	}

	subscription := payload["subscription"].(gin.H)
	if subscription["title"] != "KIRO FREE" {
		t.Fatalf("subscription title = %#v", subscription["title"])
	}
	if subscription["type"] != "Q_DEVELOPER_STANDALONE_FREE" {
		t.Fatalf("subscription type = %#v", subscription["type"])
	}

	usage := payload["usage"].(gin.H)
	if usage["current"] != 0.5 {
		t.Fatalf("usage current = %#v, want 0.5", usage["current"])
	}
	if usage["limit"] != 50.0 {
		t.Fatalf("usage limit = %#v, want 50", usage["limit"])
	}
	if _, ok := payload["breakdown"]; ok {
		t.Fatalf("payload should not expose usage breakdown")
	}
}
