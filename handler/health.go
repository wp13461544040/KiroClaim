package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/model"
	"github.com/huey1in/KiroClaim/utils"
)

var healthUpdateMu sync.Mutex

// 辅助解析。

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getNumber(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok && v != nil {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case uint64:
			return float64(n)
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func getNested(m map[string]interface{}, keys ...string) interface{} {
	current := interface{}(m)
	for _, k := range keys {
		cm, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = cm[k]
	}
	return current
}

func getNestedString(m map[string]interface{}, keys ...string) string {
	if s, ok := getNested(m, keys...).(string); ok {
		return s
	}
	return ""
}

// extractTimestamp 接收 Unix 秒/毫秒、RFC3339 字符串、数字字符串。
func extractTimestamp(val interface{}) *time.Time {
	if val == nil {
		return nil
	}
	var ts int64
	switch v := val.(type) {
	case float64:
		ts = int64(v)
	case float32:
		ts = int64(v)
	case int64:
		ts = v
	case uint64:
		ts = int64(v)
	case int:
		ts = int64(v)
	case string:
		if v == "" {
			return nil
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02T15:04:05.000Z", "2006-01-02"} {
			if t, err := time.Parse(layout, v); err == nil {
				return &t
			}
		}
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			ts = parsed
		} else {
			return nil
		}
	default:
		return nil
	}
	if ts <= 0 {
		return nil
	}
	if ts > 10000000000 { // 毫秒
		ts = ts / 1000
	}
	if ts < 1000000000 {
		return nil
	}
	t := time.Unix(ts, 0)
	return &t
}

// 健康检查主流程。

type healthResult struct {
	status       model.AccountStatus
	newToken     string
	newRefresh   string
	provider     string
	errMsg       string
	email        string
	subscription string
	creditUsed   float64
	creditLimit  float64
}

// applyHealthResult 将健康检查结果写入数据库。
func applyHealthResult(accountID uint, r healthResult) error {
	now := time.Now()
	updates := buildHealthUpdates(r, now)
	return persistHealthUpdates(accountID, updates)
}

func persistHealthUpdates(accountID uint, updates map[string]interface{}) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		healthUpdateMu.Lock()
		result := database.DB.Model(&model.Account{}).Where("id = ?", accountID).Updates(updates)
		healthUpdateMu.Unlock()

		if result.Error == nil {
			return nil
		}
		lastErr = result.Error
		if !isRetryableHealthUpdateError(lastErr) {
			return lastErr
		}
		time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
	}
	return lastErr
}

func isRetryableHealthUpdateError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
}

// verifyDispatchable 发货前预检：现场调用 ListAvailableModels，确认账号 token 可用。
// 返回 true 才允许把账号派发给卡密；超时或非 200 都视为不可发货。
func verifyDispatchable(accessToken string) bool {
	if accessToken == "" {
		return false
	}
	release := acquireUpstreamCheckSlot()
	defer release()

	settingsMu.RLock()
	timeoutSec := currentSettings.RequestTimeoutSeconds
	settingsMu.RUnlock()
	if timeoutSec <= 0 {
		timeoutSec = 15
	}
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	status, _ := probeListModels(client, accessToken)
	return status == http.StatusOK
}

// buildHealthUpdates 构建健康检查结果的更新字段。
//
// access_token / refresh_token 必须显式调用 utils.Encrypt；GORM map 更新不会触发
// Account.BeforeSave 钩子，不手动加密会把明文 token 写回数据库。
func buildHealthUpdates(r healthResult, now time.Time) map[string]interface{} {
	updates := map[string]interface{}{
		"status":          r.status,
		"last_checked_at": now,
	}
	if r.newToken != "" {
		updates["access_token"] = utils.Encrypt(r.newToken)
	}
	if r.newRefresh != "" {
		updates["refresh_token"] = utils.Encrypt(r.newRefresh)
	}
	if r.provider != "" {
		updates["provider"] = r.provider
	}
	if r.email != "" {
		updates["email"] = r.email
	}
	if r.subscription != "" {
		updates["subscription"] = r.subscription
	}
	if r.creditLimit > 0 {
		updates["credit_used"] = r.creditUsed
		updates["credit_limit"] = r.creditLimit
	}
	return updates
}

// checkAccountHealth 按 Kiro 流程执行三步检查：
//  1. 刷新 accessToken（按凭证特征尝试端点，并用成功端点回写 provider）
//  2. GET getUsageLimits 拉取 email / subscription / credit
//  3. GET ListAvailableModels 拉取账号可用模型
//
// 任何一步收到 HTTP 403 都判定为封号。
func checkAccountHealth(a model.Account) healthResult {
	if a.RefreshToken == "" {
		return healthResult{status: model.AccountStatusSuspended, errMsg: "缺少 refreshToken"}
	}
	release := acquireUpstreamCheckSlot()
	defer release()

	settingsMu.RLock()
	timeoutSec := currentSettings.RequestTimeoutSeconds
	settingsMu.RUnlock()
	if timeoutSec <= 0 {
		timeoutSec = 15
	}
	httpClient := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}

	// Step 1: refresh
	accessToken, newRefresh, provider, rStatus, rErr := refreshAccessToken(httpClient, a)
	if rErr != "" {
		if rStatus == http.StatusForbidden {
			return healthResult{status: model.AccountStatusSuspended, errMsg: "刷新 token 403: " + rErr}
		}
		// 临时错误（网络、5xx、非 403 的 4xx）保持 active，下一轮再重试。
		return healthResult{status: model.AccountStatusActive, errMsg: rErr}
	}

	// Step 2: getUsageLimits
	usage, uStatus, uErr := fetchUsageLimits(httpClient, accessToken)
	if uErr != "" {
		if uStatus == http.StatusForbidden {
			return healthResult{
				status:     model.AccountStatusSuspended,
				newToken:   accessToken,
				newRefresh: newRefresh,
				provider:   provider,
				errMsg:     "getUsageLimits 403: " + uErr,
			}
		}
		return healthResult{
			status:     model.AccountStatusActive,
			newToken:   accessToken,
			newRefresh: newRefresh,
			provider:   provider,
			errMsg:     uErr,
		}
	}

	// Step 3: ListAvailableModels
	if status, detail := probeListModels(httpClient, accessToken); status == http.StatusForbidden {
		return healthResult{
			status:       model.AccountStatusSuspended,
			newToken:     accessToken,
			newRefresh:   newRefresh,
			provider:     provider,
			email:        usage.email,
			subscription: usage.subscription,
			creditUsed:   usage.creditUsed,
			creditLimit:  usage.creditLimit,
			errMsg:       "ListAvailableModels 403: " + detail,
		}
	} else if status != http.StatusOK {
		return healthResult{
			status:       model.AccountStatusActive,
			newToken:     accessToken,
			newRefresh:   newRefresh,
			provider:     provider,
			email:        usage.email,
			subscription: usage.subscription,
			creditUsed:   usage.creditUsed,
			creditLimit:  usage.creditLimit,
			errMsg:       fmt.Sprintf("ListAvailableModels HTTP %d: %s", status, detail),
		}
	}

	return healthResult{
		status:       model.AccountStatusActive,
		newToken:     accessToken,
		newRefresh:   newRefresh,
		provider:     provider,
		email:        usage.email,
		subscription: usage.subscription,
		creditUsed:   usage.creditUsed,
		creditLimit:  usage.creditLimit,
	}
}

// Step 1: 刷新 accessToken。

// refreshAccessToken 根据凭证特征决定尝试顺序，并用实际成功端点判定 provider。
//   - idc: AWS OIDC token endpoint，需要 clientId/clientSecret
//   - social: Kiro desktop auth refreshToken endpoint，只需要 refreshToken
//
// 返回 accessToken、新 refreshToken（若未轮换则回退为旧值）、provider、HTTP 状态码、错误信息。
func refreshAccessToken(client *http.Client, a model.Account) (accessToken, newRefresh, provider string, statusCode int, errMsg string) {
	candidates := refreshCandidates(a)
	if len(candidates) == 0 {
		return "", "", "", 0, "缺少 refreshToken"
	}

	var lastProvider string
	var lastStatus int
	var lastErr string
	for _, candidate := range candidates {
		token, refresh, status, errText := requestRefreshToken(client, a, candidate)
		if errText == "" {
			return token, refresh, candidate.provider, status, ""
		}
		lastProvider = candidate.provider
		lastStatus = status
		lastErr = errText
		if status == http.StatusForbidden {
			return "", "", candidate.provider, status, errText
		}
	}
	if lastProvider != "" {
		lastErr = lastProvider + " 刷新失败: " + lastErr
	}
	return "", "", lastProvider, lastStatus, lastErr
}

type refreshCandidate struct {
	provider string
	url      string
	body     map[string]string
}

func refreshCandidates(a model.Account) []refreshCandidate {
	refreshToken := strings.TrimSpace(a.RefreshToken)
	if refreshToken == "" {
		return nil
	}

	hasIDCCredential := strings.TrimSpace(a.ClientId) != "" && strings.TrimSpace(a.ClientSecret) != ""
	candidates := make([]refreshCandidate, 0, 2)
	if hasIDCCredential {
		candidates = append(candidates, refreshCandidate{
			provider: model.AccountProviderIDC,
			url:      "https://oidc.us-east-1.amazonaws.com/token",
			body: map[string]string{
				"clientId":     a.ClientId,
				"clientSecret": a.ClientSecret,
				"refreshToken": refreshToken,
				"grantType":    "refresh_token",
			},
		})
	}
	candidates = append(candidates, refreshCandidate{
		provider: model.AccountProviderSocial,
		url:      "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken",
		body: map[string]string{
			"refreshToken": refreshToken,
		},
	})
	return candidates
}

func requestRefreshToken(client *http.Client, a model.Account, candidate refreshCandidate) (accessToken, newRefresh string, statusCode int, errMsg string) {
	body, _ := json.Marshal(candidate.body)
	req, err := http.NewRequest("POST", candidate.url, bytes.NewReader(body))
	if err != nil {
		return "", "", 0, "构建请求失败: " + err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, "刷新 token 请求失败: " + err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		detail := string(raw)
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return "", "", resp.StatusCode, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, detail)
	}

	var tr struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", "", resp.StatusCode, "解析刷新响应失败: " + err.Error()
	}
	if tr.AccessToken == "" {
		return "", "", resp.StatusCode, "刷新响应缺少 accessToken"
	}
	if tr.RefreshToken == "" {
		tr.RefreshToken = a.RefreshToken
	}
	return tr.AccessToken, tr.RefreshToken, resp.StatusCode, ""
}

// Step 2: 拉取用量。

type usageInfo struct {
	email        string
	subscription string
	planTier     string
	creditUsed   float64
	creditLimit  float64
	tokenExpiry  *time.Time // 试用到期 / token 有效期
}

// getUsageLimits 必须带 isEmailRequired=true 才会返回 userInfo.email。
// profileArn 为 Kiro AGENTIC_REQUEST 共享 profile，resourceType 固定。
const usageLimitsURL = "https://q.us-east-1.amazonaws.com/getUsageLimits?isEmailRequired=true&origin=AI_EDITOR&profileArn=arn%3Aaws%3Acodewhisperer%3Aus-east-1%3A638616132270%3Aprofile%2FAAAACCCCXXXX&resourceType=AGENTIC_REQUEST"

func fetchUsageLimits(client *http.Client, accessToken string) (*usageInfo, int, string) {
	req, _ := http.NewRequest("GET", usageLimitsURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "getUsageLimits 请求失败: " + err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		detail := string(raw)
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return nil, resp.StatusCode, fmt.Sprintf("getUsageLimits HTTP %d: %s", resp.StatusCode, detail)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, resp.StatusCode, "getUsageLimits JSON 解析失败: " + err.Error()
	}

	u := &usageInfo{}

	// 邮箱。
	u.email = firstString(obj,
		[]string{"email"},
		[]string{"userInfo", "email"},
		[]string{"user", "email"},
	)

	// 订阅名。
	u.subscription = firstString(obj,
		[]string{"subscriptionTitle"},
		[]string{"subscription"},
		[]string{"subscriptionInfo", "subscriptionTitle"},
		[]string{"subscription", "title"},
	)

	// Plan tier。
	u.planTier = firstString(obj,
		[]string{"planTier"},
		[]string{"tier"},
		[]string{"subscriptionInfo", "type"},
		[]string{"subscription", "type"},
	)
	if u.subscription == "" {
		u.subscription = u.planTier
	}

	// 额度：先尝试顶层 / usage 子对象。
	u.creditLimit, u.creditUsed = parseCredits(obj)

	// 若额度仍为 0，尝试 usageBreakdownList 中的 CREDIT 项和试用加成。
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
				if trial, ok := m["freeTrialInfo"].(map[string]interface{}); ok {
					status := getString(trial, "freeTrialStatus")
					if status == "ACTIVE" || status == "" {
						tl := getNumber(trial, "usageLimitWithPrecision")
						if tl == 0 {
							tl = getNumber(trial, "usageLimit")
						}
						tu := getNumber(trial, "currentUsageWithPrecision")
						if tu == 0 {
							tu = getNumber(trial, "currentUsage")
						}
						u.creditLimit += tl
						u.creditUsed += tu
					}
				}
				break
			}
		}
	}

	// token / 试用到期。
	for _, keys := range [][]string{
		{"tokenExpiresAt"}, {"tokenExpiry"}, {"expiresAt"}, {"credentialExpiry"},
		{"freeTrialExpiry"}, {"subscriptionInfo", "expiresAt"}, {"subscription", "expiresAt"},
	} {
		if ts := extractTimestamp(getNested(obj, keys...)); ts != nil {
			u.tokenExpiry = ts
			break
		}
	}
	// 到期时间兜底：usageBreakdownList[].freeTrialInfo.{freeTrialExpiry|expiresAt|...}
	if u.tokenExpiry == nil {
		if list, ok := obj["usageBreakdownList"].([]interface{}); ok {
			for _, it := range list {
				m, _ := it.(map[string]interface{})
				trial, ok := m["freeTrialInfo"].(map[string]interface{})
				if !ok {
					continue
				}
				for _, key := range []string{"freeTrialExpiry", "expiresAt", "expiryDate", "expiry", "endDate"} {
					if ts := extractTimestamp(trial[key]); ts != nil {
						u.tokenExpiry = ts
						break
					}
				}
				if u.tokenExpiry != nil {
					break
				}
			}
		}
	}

	return u, resp.StatusCode, ""
}

// parseCredits 从顶层 / usage 子对象提取额度。
func parseCredits(obj map[string]interface{}) (limit, used float64) {
	limit = getNumber(obj, "usageLimit")
	used = getNumber(obj, "currentUsage")
	if limit != 0 {
		return
	}
	limit = getNumber(obj, "creditLimit")
	used = getNumber(obj, "creditUsed")
	if limit != 0 {
		return
	}
	if sub, ok := obj["usage"].(map[string]interface{}); ok {
		limit = getNumber(sub, "limit")
		used = getNumber(sub, "used")
	}
	return
}

// firstString 沿多条候选路径找第一个非空字符串。
func firstString(obj map[string]interface{}, paths ...[]string) string {
	for _, p := range paths {
		if s, ok := getNested(obj, p...).(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// Step 3: 可用模型探测。

func probeListModels(client *http.Client, accessToken string) (int, string) {
	req, _ := http.NewRequest("GET", "https://q.us-east-1.amazonaws.com/ListAvailableModels?origin=AI_EDITOR", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	detail := string(raw)
	if len(detail) > 300 {
		detail = detail[:300]
	}
	return resp.StatusCode, detail
}
