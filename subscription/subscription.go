package subscription

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const (
	subscriptionVersion = "0.12.155"
	defaultProfileARN   = "arn:aws:codewhisperer:us-east-1:638616132270:profile/AAAACCCCXXXX"
	socialProfileARN    = "arn:aws:codewhisperer:us-east-1:699475941385:profile/EHGA3GRVQMUK"
)

// Plan 订阅计划
type Plan struct {
	Name              string                 `json:"name"`
	QSubscriptionType string                 `json:"qSubscriptionType"`
	Description       map[string]interface{} `json:"description"`
	Pricing           map[string]interface{} `json:"pricing"`
}

// Account 账号凭据
type Account struct {
	Email        string `json:"email"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Region       string `json:"region"`
	Provider     string `json:"provider"`
}

// PlansResult 获取订阅计划的扩展结果
type PlansResult struct {
	Plans               []Plan `json:"plans"`
	CurrentSubscription string `json:"currentSubscription"`
}

func qEndpoint(region string) string {
	if strings.HasPrefix(region, "eu-") {
		return "https://q.eu-central-1.amazonaws.com"
	}
	return "https://q.us-east-1.amazonaws.com"
}

func oidcEndpoint(region string) string {
	if region == "" {
		region = "us-east-1"
	}
	return fmt.Sprintf("https://oidc.%s.amazonaws.com/token", region)
}

func cwEndpoint(region string) string {
	if strings.HasPrefix(region, "eu-") {
		return "https://q.eu-central-1.amazonaws.com"
	}
	return "https://codewhisperer.us-east-1.amazonaws.com"
}

func stableMachineID(email string) string {
	h := sha256.Sum256([]byte("kiro-device-" + email))
	return hex.EncodeToString(h[:])
}

func profileARN(provider string) string {
	if provider == "Github" || provider == "Google" {
		return socialProfileARN
	}
	return defaultProfileARN
}

func subUA(machineID string) string {
	return fmt.Sprintf("aws-sdk-js/1.0.0 ua/2.1 os/win32#10.0.19043 lang/js md/nodejs#22.22.0 api/codewhispererruntime#1.0.0 m/N,E KiroIDE-%s-%s", subscriptionVersion, machineID)
}

func subAmzUA(machineID string) string {
	return fmt.Sprintf("aws-sdk-js/1.0.0 KiroIDE-%s-%s", subscriptionVersion, machineID)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var httpClient = &http.Client{}

// RefreshAccessToken 用 refreshToken 换 accessToken
func RefreshAccessToken(acc Account) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"clientId":     acc.ClientID,
		"clientSecret": acc.ClientSecret,
		"refreshToken": acc.RefreshToken,
		"grantType":    "refresh_token",
	})
	req, _ := http.NewRequest("POST", oidcEndpoint(acc.Region), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("刷新 token 失败 HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 1000))
	}
	var tok map[string]interface{}
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return "", fmt.Errorf("解析 token 响应失败: %w", err)
	}
	at, _ := tok["accessToken"].(string)
	if at == "" {
		return "", fmt.Errorf("响应缺少 accessToken")
	}
	return at, nil
}

func doSubscriptionPost(acc Account, accessToken, path string, payload map[string]interface{}) ([]byte, int, error) {
	machineID := stableMachineID(acc.Email)
	endpoint := qEndpoint(acc.Region) + path

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", subUA(machineID))
	req.Header.Set("x-amz-user-agent", subAmzUA(machineID))
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// ListPlansEx 拉取账号可用的订阅计划，并尝试解析当前订阅类型
func ListPlansEx(acc Account, accessToken string) (*PlansResult, error) {
	body, status, err := doSubscriptionPost(acc, accessToken, "/listAvailableSubscriptions", map[string]interface{}{
		"profileArn": profileARN(acc.Provider),
	})
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", status, truncate(string(body), 1000))
	}
	var resp struct {
		SubscriptionPlans   []Plan `json:"subscriptionPlans"`
		CurrentSubscription string `json:"currentSubscription"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析计划列表失败: %w", err)
	}
	return &PlansResult{
		Plans:               resp.SubscriptionPlans,
		CurrentSubscription: resp.CurrentSubscription,
	}, nil
}

// CreateSubscriptionLink 取支付/试用链接
func CreateSubscriptionLink(acc Account, accessToken, subscriptionType string) (string, error) {
	payload := map[string]interface{}{
		"clientToken": uuid.NewString(),
		"profileArn":  profileARN(acc.Provider),
		"provider":    "STRIPE",
	}
	if subscriptionType != "" {
		payload["subscriptionType"] = subscriptionType
	}
	body, status, err := doSubscriptionPost(acc, accessToken, "/CreateSubscriptionToken", payload)
	if err != nil {
		return "", err
	}
	if status != 200 {
		var errResp map[string]interface{}
		_ = json.Unmarshal(body, &errResp)
		msg, _ := errResp["message"].(string)
		hint := ""
		switch {
		case status == 403:
			hint = "（账号已被封禁）"
		case status == 400 && strings.Contains(strings.ToLower(msg), "already"):
			hint = "（账号已存在订阅）"
		}
		if msg != "" {
			return "", fmt.Errorf("HTTP %d: %s%s", status, msg, hint)
		}
		return "", fmt.Errorf("HTTP %d: %s%s", status, truncate(string(body), 1000), hint)
	}
	var resp struct {
		EncodedVerificationURL string `json:"encodedVerificationUrl"`
		Message                string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if resp.EncodedVerificationURL == "" {
		if resp.Message != "" {
			return "", fmt.Errorf("%s", resp.Message)
		}
		return "", fmt.Errorf("响应未返回支付链接")
	}
	return resp.EncodedVerificationURL, nil
}

// IsSuspended 根据错误信息判断账号是否被封禁
func IsSuspended(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "http 403") ||
		strings.Contains(s, "accountsuspendedexception") ||
		strings.Contains(s, "account suspended") ||
		strings.Contains(s, "temporarily is suspended") ||
		strings.Contains(s, "temporarily suspended") ||
		strings.Contains(s, "locked your account") ||
		strings.Contains(s, "not authorized to access this feature") ||
		strings.Contains(s, "已封禁") ||
		strings.Contains(s, "http 423")
}

// resolveProfileArn 通过 ListAvailableProfiles 拿账号真实的 profile ARN
func resolveProfileArn(acc Account, accessToken string) string {
	machineID := stableMachineID(acc.Email)
	endpoint := cwEndpoint(acc.Region)
	req, _ := http.NewRequest("POST", endpoint+"/ListAvailableProfiles", strings.NewReader(`{"maxResults":10}`))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", subUA(machineID))
	req.Header.Set("x-amz-user-agent", subAmzUA(machineID))
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")

	resp, err := httpClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Profiles []struct {
			Arn string `json:"arn"`
		} `json:"profiles"`
	}
	_ = json.Unmarshal(body, &result)
	for _, p := range result.Profiles {
		if strings.TrimSpace(p.Arn) != "" {
			return p.Arn
		}
	}
	return ""
}

// GetUserInfo 通过 getUsageLimits 接口获取账号邮箱、当前订阅类型和用户 ID
func GetUserInfo(acc Account, accessToken string) (email, currentSubscription, userID string, err error) {
	arn := resolveProfileArn(acc, accessToken)
	if arn == "" {
		arn = profileARN(acc.Provider)
	}

	endpoint := cwEndpoint(acc.Region)
	urlStr := endpoint + "/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true"
	if arn != "" {
		urlStr += "&profileArn=" + url.QueryEscape(arn)
	}

	machineID := stableMachineID(acc.Email)
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", subUA(machineID))
	req.Header.Set("x-amz-user-agent", subAmzUA(machineID))
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var result struct {
		UserInfo struct {
			Email  string `json:"email"`
			UserID string `json:"userId"`
		} `json:"userInfo"`
		SubscriptionInfo *struct {
			SubscriptionType  string `json:"subscriptionType"`
			SubscriptionTitle string `json:"subscriptionTitle"`
			SubscriptionName  string `json:"subscriptionName"`
		} `json:"subscriptionInfo"`
	}
	_ = json.Unmarshal(body, &result)
	log.Printf("[SubTool][GetUserInfo] email=%q subscriptionInfo=%+v", result.UserInfo.Email, result.SubscriptionInfo)

	subType := ""
	if result.SubscriptionInfo != nil {
		subType = result.SubscriptionInfo.SubscriptionTitle
		if subType == "" {
			subType = result.SubscriptionInfo.SubscriptionName
		}
		if subType == "" {
			subType = result.SubscriptionInfo.SubscriptionType
		}
	}
	return result.UserInfo.Email, subType, result.UserInfo.UserID, nil
}

// ExtractEmailFromToken 从 JWT accessToken 的 payload 中解出 email
func ExtractEmailFromToken(token string) string {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	for _, key := range []string{"email", "username", "cognito:username"} {
		if v, ok := claims[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
