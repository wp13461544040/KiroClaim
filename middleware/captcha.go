package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type CaptchaConfig struct {
	Enabled       bool
	SecretKey     string
	FreeCount     int
	FreeWindowSec int
}

var (
	captchaMu     sync.RWMutex
	captchaConfig = CaptchaConfig{
		Enabled:       false,
		FreeCount:     3,
		FreeWindowSec: 60,
	}

	minDelayMu sync.RWMutex
	minDelayMs = 150
)

func UpdateCaptchaConfig(c CaptchaConfig) {
	captchaMu.Lock()
	captchaConfig = c
	captchaMu.Unlock()
}

func GetCaptchaConfig() CaptchaConfig {
	captchaMu.RLock()
	defer captchaMu.RUnlock()
	return captchaConfig
}

func UpdateMinDelay(ms int) {
	if ms < 0 {
		ms = 0
	}
	minDelayMu.Lock()
	minDelayMs = ms
	minDelayMu.Unlock()
}

func GetMinDelay() int {
	minDelayMu.RLock()
	defer minDelayMu.RUnlock()
	return minDelayMs
}

type ipVisit struct {
	count   int
	firstAt time.Time
}

var (
	visitMu sync.Mutex
	visits  = make(map[string]*ipVisit)
)

func consumeFreeVisit(ip string, freeCount, windowSec int) bool {
	visitMu.Lock()
	defer visitMu.Unlock()

	v, ok := visits[ip]
	now := time.Now()
	if !ok || now.Sub(v.firstAt) > time.Duration(windowSec)*time.Second {
		visits[ip] = &ipVisit{count: 1, firstAt: now}
		return true
	}
	v.count++
	return v.count <= freeCount
}

func verifyTurnstile(secret, token, remoteIP string) bool {
	if secret == "" || token == "" {
		return false
	}
	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", form)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	return result.Success
}

func CaptchaMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := GetCaptchaConfig()
		if !cfg.Enabled || cfg.SecretKey == "" {
			c.Next()
			return
		}

		ip := c.ClientIP()
		token := c.Query("captcha_token")
		if token == "" {
			token = c.GetHeader("X-Captcha-Token")
		}

		if token == "" && consumeFreeVisit(ip, cfg.FreeCount, cfg.FreeWindowSec) {
			c.Next()
			return
		}

		if token == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "请求过于频繁，请完成人机验证后重试",
				"captcha": "required",
			})
			c.Abort()
			return
		}

		if !verifyTurnstile(cfg.SecretKey, token, ip) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "人机验证失败，请刷新后重试",
				"captcha": "invalid",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func MinDelayMiddleware(_ int) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		ms := GetMinDelay()
		if ms <= 0 {
			return
		}
		elapsed := time.Since(start)
		target := time.Duration(ms) * time.Millisecond
		if elapsed < target {
			time.Sleep(target - elapsed)
		}
	}
}

func init() {
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			visitMu.Lock()
			now := time.Now()
			for ip, v := range visits {
				if now.Sub(v.firstAt) > 10*time.Minute {
					delete(visits, ip)
				}
			}
			visitMu.Unlock()
		}
	}()
}
