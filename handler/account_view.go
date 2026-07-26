package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"
	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
)

func accountRequestClient() *http.Client {
	settingsMu.RLock()
	timeoutSec := currentSettings.RequestTimeoutSeconds
	settingsMu.RUnlock()
	if timeoutSec <= 0 {
		timeoutSec = 15
	}
	return &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
}

func loadAccountForView(c *gin.Context) (*model.Account, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "无效的账号 ID"})
		return nil, false
	}
	var account model.Account
	if err := database.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "账号不存在"})
		return nil, false
	}
	return &account, true
}

func refreshAccountTokenForView(c *gin.Context, account *model.Account, client *http.Client) (string, bool) {
	accessToken, newRefresh, provider, statusCode, errMsg := refreshAccessToken(client, *account)
	if statusCode == http.StatusForbidden {
		database.DB.Model(&model.Account{}).Where("id = ?", account.ID).Update("status", model.AccountStatusSuspended)
	}
	if errMsg != "" {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": friendlyUpstreamError("刷新 token", statusCode, errMsg), "upstreamStatus": statusCode})
		return "", false
	}

	updates := map[string]interface{}{"access_token": utils.Encrypt(accessToken)}
	if newRefresh != "" && newRefresh != account.RefreshToken {
		updates["refresh_token"] = utils.Encrypt(newRefresh)
		account.RefreshToken = newRefresh
	}
	if provider != "" {
		updates["provider"] = provider
		account.Provider = provider
	}
	database.DB.Model(&model.Account{}).Where("id = ?", account.ID).Updates(updates)
	account.AccessToken = accessToken
	return accessToken, true
}

func friendlyUpstreamError(step string, statusCode int, detail string) string {
	if statusCode > 0 {
		return fmt.Sprintf("%s失败，上游返回 HTTP %d：%s", step, statusCode, detail)
	}
	return fmt.Sprintf("%s失败：%s", step, detail)
}

func AccountDetail(c *gin.Context) {
	account, ok := loadAccountForView(c)
	if !ok {
		return
	}
	release := acquireUpstreamCheckSlot()
	defer release()

	client := accountRequestClient()
	accessToken, ok := refreshAccountTokenForView(c, account, client)
	if !ok {
		return
	}

	usage, raw, statusCode, errMsg := fetchUsageLimitsDetail(client, accessToken)
	if statusCode == http.StatusForbidden {
		database.DB.Model(&model.Account{}).Where("id = ?", account.ID).Update("status", model.AccountStatusSuspended)
	}
	if errMsg != "" {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": friendlyUpstreamError("获取账号用量", statusCode, errMsg), "upstreamStatus": statusCode})
		return
	}

	updates := buildHealthUpdates(healthResult{
		status:       model.AccountStatusActive,
		email:        usage.email,
		subscription: usage.subscription,
		creditUsed:   usage.creditUsed,
		creditLimit:  usage.creditLimit,
	}, time.Now())
	_ = persistHealthUpdates(account.ID, updates)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": buildUsageDetailPayload(account.ID, usage, raw),
	})
}

func AccountModels(c *gin.Context) {
	account, ok := loadAccountForView(c)
	if !ok {
		return
	}
	release := acquireUpstreamCheckSlot()
	defer release()

	client := accountRequestClient()
	accessToken, ok := refreshAccountTokenForView(c, account, client)
	if !ok {
		return
	}

	payload, statusCode, errMsg := fetchAvailableModels(client, accessToken)
	if statusCode == http.StatusForbidden {
		database.DB.Model(&model.Account{}).Where("id = ?", account.ID).Update("status", model.AccountStatusSuspended)
	}
	if errMsg != "" {
		c.JSON(http.StatusOK, gin.H{"code": 1, "message": friendlyUpstreamError("获取可用模型", statusCode, errMsg), "upstreamStatus": statusCode})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": payload})
}

func RefreshAccount(c *gin.Context) {
	account, ok := loadAccountForView(c)
	if !ok {
		return
	}

	result := checkAccountHealth(*account)
	if err := applyHealthResult(account.ID, result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "保存刷新结果失败: " + err.Error()})
		return
	}

	var fresh model.Account
	if err := database.DB.First(&fresh, account.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "读取刷新结果失败: " + err.Error()})
		return
	}

	message := "刷新成功"
	code := 0
	if result.status == model.AccountStatusSuspended {
		message = "账号已被判定封禁"
	} else if result.errMsg != "" {
		code = 1
		message = "刷新失败：" + result.errMsg
	}

	AddOpLogWithCtx(c, "refresh", "刷新账号 ID:"+strconv.Itoa(int(account.ID))+"，状态: "+string(fresh.Status), "admin")
	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"message": message,
		"data": gin.H{
			"id":            fresh.ID,
			"status":        fresh.Status,
			"email":         fresh.Email,
			"subscription":  fresh.Subscription,
			"creditUsed":    fresh.CreditUsed,
			"creditLimit":   fresh.CreditLimit,
			"lastCheckedAt": fresh.LastCheckedAt,
		},
	})
}

func fetchUsageLimitsDetail(client *http.Client, accessToken string) (*usageInfo, map[string]interface{}, int, string) {
	req, _ := http.NewRequest("GET", usageLimitsURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, 0, "请求失败: " + err.Error()
	}
	defer resp.Body.Close()
	rawBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		detail := string(rawBytes)
		if len(detail) > 500 {
			detail = detail[:500]
		}
		return nil, nil, resp.StatusCode, detail
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		return nil, nil, resp.StatusCode, "JSON 解析失败: " + err.Error()
	}
	usage := parseUsageInfo(raw)
	return usage, raw, resp.StatusCode, ""
}

func parseUsageInfo(obj map[string]interface{}) *usageInfo {
	u := &usageInfo{}
	u.email = firstString(obj,
		[]string{"email"},
		[]string{"userInfo", "email"},
		[]string{"user", "email"},
	)
	u.subscription = firstString(obj,
		[]string{"subscriptionTitle"},
		[]string{"subscription"},
		[]string{"subscriptionInfo", "subscriptionTitle"},
		[]string{"subscription", "title"},
	)
	u.planTier = firstString(obj,
		[]string{"planTier"},
		[]string{"tier"},
		[]string{"subscriptionInfo", "type"},
		[]string{"subscription", "type"},
	)
	if u.subscription == "" {
		u.subscription = u.planTier
	}
	u.creditLimit, u.creditUsed = parseCredits(obj)

	if u.creditLimit == 0 {
		if list, ok := obj["usageBreakdownList"].([]interface{}); ok {
			for _, it := range list {
				m, ok := it.(map[string]interface{})
				if !ok {
					continue
				}
				rt := getString(m, "resourceType")
				dn := getString(m, "displayName")
				if rt != "CREDIT" && rt != "" && dn != "Credits" {
					continue
				}
				limit := getNumber(m, "usageLimitWithPrecision")
				if limit == 0 {
					limit = getNumber(m, "usageLimit")
				}
				used := getNumber(m, "currentUsageWithPrecision")
				if used == 0 {
					used = getNumber(m, "currentUsage")
				}
				u.creditLimit = limit
				u.creditUsed = used
				break
			}
		}
	}

	for _, keys := range [][]string{
		{"tokenExpiresAt"}, {"tokenExpiry"}, {"expiresAt"}, {"credentialExpiry"},
		{"freeTrialExpiry"}, {"subscriptionInfo", "expiresAt"}, {"subscription", "expiresAt"},
	} {
		if ts := extractTimestamp(getNested(obj, keys...)); ts != nil {
			u.tokenExpiry = ts
			break
		}
	}
	if u.tokenExpiry == nil {
		if list, ok := obj["usageBreakdownList"].([]interface{}); ok {
			for _, it := range list {
				m, _ := it.(map[string]interface{})
				if ts := extractTimestamp(m["nextDateReset"]); ts != nil {
					u.tokenExpiry = ts
					break
				}
			}
		}
	}
	return u
}

func buildUsageDetailPayload(accountID uint, usage *usageInfo, raw map[string]interface{}) gin.H {
	userInfo, _ := raw["userInfo"].(map[string]interface{})
	subscriptionInfo, _ := raw["subscriptionInfo"].(map[string]interface{})
	usageItem := pickCreditUsageItem(raw)
	current := usage.creditUsed
	limit := usage.creditLimit
	if usageItem != nil {
		current = firstExistingNumber(usageItem, "currentUsageWithPrecision", "currentUsage")
		limit = firstExistingNumber(usageItem, "usageLimitWithPrecision", "usageLimit")
	}

	email := strings.TrimSpace(getString(userInfo, "email"))
	if email == "" {
		email = "-"
	}

	return gin.H{
		"id":             accountID,
		"source":         "upstream",
		"fetchedAt":      time.Now().UTC().Format(time.RFC3339),
		"email":          email,
		"userId":         firstNonEmpty(getString(userInfo, "userId"), "-"),
		"daysUntilReset": raw["daysUntilReset"],
		"nextDateReset":  raw["nextDateReset"],
		"subscription": gin.H{
			"title":             firstNonEmpty(getString(subscriptionInfo, "subscriptionTitle"), usage.subscription, "-"),
			"type":              firstNonEmpty(getString(subscriptionInfo, "type"), usage.planTier, "-"),
			"managementTarget":  getString(subscriptionInfo, "subscriptionManagementTarget"),
			"upgradeCapability": getString(subscriptionInfo, "upgradeCapability"),
		},
		"usage": gin.H{
			"current": current,
			"limit":   limit,
			"percent": usagePercent(current, limit),
		},
	}
}

func pickCreditUsageItem(raw map[string]interface{}) map[string]interface{} {
	list, ok := raw["usageBreakdownList"].([]interface{})
	if !ok {
		return nil
	}
	var fallback map[string]interface{}
	for _, it := range list {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		if fallback == nil {
			fallback = m
		}
		resourceType := strings.ToUpper(strings.TrimSpace(getString(m, "resourceType")))
		displayName := strings.ToLower(strings.TrimSpace(getString(m, "displayName")))
		displayNamePlural := strings.ToLower(strings.TrimSpace(getString(m, "displayNamePlural")))
		if resourceType == "CREDIT" || displayName == "credit" || displayNamePlural == "credits" {
			return m
		}
	}
	return fallback
}

func firstExistingNumber(m map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if _, ok := m[key]; ok {
			return getNumber(m, key)
		}
	}
	return 0
}

func fetchAvailableModels(client *http.Client, accessToken string) (gin.H, int, string) {
	req, _ := http.NewRequest("GET", "https://q.us-east-1.amazonaws.com/ListAvailableModels?origin=AI_EDITOR", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "请求失败: " + err.Error()
	}
	defer resp.Body.Close()
	rawBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		detail := string(rawBytes)
		if len(detail) > 500 {
			detail = detail[:500]
		}
		return nil, resp.StatusCode, detail
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		return nil, resp.StatusCode, "JSON 解析失败: " + err.Error()
	}

	models := make([]gin.H, 0)
	if list, ok := raw["models"].([]interface{}); ok {
		for _, it := range list {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			models = append(models, gin.H{
				"modelId":             getString(m, "modelId"),
				"modelName":           getString(m, "modelName"),
				"description":         getString(m, "description"),
				"rateMultiplier":      getNumber(m, "rateMultiplier"),
				"rateUnit":            getString(m, "rateUnit"),
				"supportedInputTypes": m["supportedInputTypes"],
				"tokenLimits":         m["tokenLimits"],
				"promptCaching":       m["promptCaching"],
			})
		}
	}

	defaultModelID := ""
	if defaultModel, ok := raw["defaultModel"].(map[string]interface{}); ok {
		defaultModelID = getString(defaultModel, "modelId")
	}
	return gin.H{
		"defaultModelId": defaultModelID,
		"models":         models,
		"nextToken":      raw["nextToken"],
	}, resp.StatusCode, ""
}

func usagePercent(current, limit float64) float64 {
	if limit <= 0 {
		return 0
	}
	pct := current / limit * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
