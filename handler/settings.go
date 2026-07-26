package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/middleware"
	"github.com/huey1in/KiroClaim/model"
	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type AppSettings struct {
	MaxUpstreamCheckConcurrency int
	DispatchHealthCheckEnabled  bool
	RequestTimeoutSeconds       int
	RateLimitEnabled            bool
	RateLimitPerMin             int
	LoginFailLimit              int
	LoginLockMinutes            int
	CaptchaEnabled              bool
	CaptchaSiteKey              string
	CaptchaSecretKey            string
	CaptchaFreeCount            int
	MinResponseMs               int
	LogFileEnabled              bool
	LogFilePath                 string
	LogMaxSizeMB                int
	LogMaxBackups               int
	LogMaxAgeDays               int
	LogCompress                 bool
	AutoUpdateEnabled           bool
}

type storedRuntimeSettings struct {
	MaxUpstreamCheckConcurrency *int    `json:"maxUpstreamCheckConcurrency,omitempty"`
	MaxImportConcurrency        *int    `json:"maxImportConcurrency,omitempty"`
	DispatchHealthCheckEnabled  *bool   `json:"dispatchHealthCheckEnabled,omitempty"`
	RequestTimeoutSeconds       *int    `json:"requestTimeoutSeconds,omitempty"`
	RateLimitEnabled            *bool   `json:"rateLimitEnabled,omitempty"`
	RateLimitPerMin             *int    `json:"rateLimitPerMin,omitempty"`
	LoginFailLimit              *int    `json:"loginFailLimit,omitempty"`
	LoginLockMinutes            *int    `json:"loginLockMinutes,omitempty"`
	CaptchaEnabled              *bool   `json:"captchaEnabled,omitempty"`
	CaptchaSiteKey              *string `json:"captchaSiteKey,omitempty"`
	CaptchaSecretKey            *string `json:"captchaSecretKey,omitempty"`
	CaptchaFreeCount            *int    `json:"captchaFreeCount,omitempty"`
	MinResponseMs               *int    `json:"minResponseMs,omitempty"`
	LogFileEnabled              *bool   `json:"logFileEnabled,omitempty"`
	LogFilePath                 *string `json:"logFilePath,omitempty"`
	LogMaxSizeMB                *int    `json:"logMaxSizeMB,omitempty"`
	LogMaxBackups               *int    `json:"logMaxBackups,omitempty"`
	LogMaxAgeDays               *int    `json:"logMaxAgeDays,omitempty"`
	LogCompress                 *bool   `json:"logCompress,omitempty"`
	AutoUpdateEnabled           *bool   `json:"autoUpdateEnabled,omitempty"`
}

var (
	settingsMu      sync.RWMutex
	currentSettings AppSettings
	ApiRateLimiter  *middleware.RateLimiter
)

func LoadSettingsFromEnv() {
	logging := utils.DefaultLoggingConfigFromEnv()
	s := AppSettings{
		MaxUpstreamCheckConcurrency: 6,
		DispatchHealthCheckEnabled:  envBool("DISPATCH_HEALTH_CHECK_ENABLED", true),
		RequestTimeoutSeconds:       45,
		RateLimitEnabled:            envBool("RATE_LIMIT_ENABLED", true),
		RateLimitPerMin:             envInt("RATE_LIMIT_PER_MIN", 30),
		LoginFailLimit:              envInt("LOGIN_FAIL_LIMIT", 5),
		LoginLockMinutes:            envInt("LOGIN_LOCK_MINUTES", 15),
		CaptchaEnabled:              envBool("CAPTCHA_ENABLED", false),
		CaptchaSiteKey:              os.Getenv("CAPTCHA_SITE_KEY"),
		CaptchaSecretKey:            os.Getenv("CAPTCHA_SECRET_KEY"),
		CaptchaFreeCount:            envInt("CAPTCHA_FREE_COUNT", 3),
		MinResponseMs:               envInt("MIN_RESPONSE_MS", 150),
		LogFileEnabled:              logging.FileEnabled,
		LogFilePath:                 logging.FilePath,
		LogMaxSizeMB:                logging.MaxSizeMB,
		LogMaxBackups:               logging.MaxBackups,
		LogMaxAgeDays:               logging.MaxAgeDays,
		LogCompress:                 logging.Compress,
		AutoUpdateEnabled:           envBool("AUTO_UPDATE_ENABLED", false),
	}
	applyStoredRuntimeSettings(&s)
	normalizeSettings(&s)

	settingsMu.Lock()
	currentSettings = s
	settingsMu.Unlock()

	updateSecuritySettings(s)
}

func applyStoredRuntimeSettings(s *AppSettings) {
	if database.DB == nil {
		return
	}
	var kv model.KV
	result := database.WhereKVKey(database.DB, model.KVRuntimeSettings).Find(&kv)
	if result.Error != nil || result.RowsAffected == 0 || strings.TrimSpace(kv.Value) == "" {
		return
	}

	var stored storedRuntimeSettings
	if err := json.Unmarshal([]byte(kv.Value), &stored); err != nil {
		return
	}
	mergeStoredRuntimeSettings(s, stored)
}

func mergeStoredRuntimeSettings(s *AppSettings, stored storedRuntimeSettings) {
	if stored.MaxUpstreamCheckConcurrency != nil {
		s.MaxUpstreamCheckConcurrency = *stored.MaxUpstreamCheckConcurrency
	} else if stored.MaxImportConcurrency != nil {
		s.MaxUpstreamCheckConcurrency = *stored.MaxImportConcurrency
	}
	if stored.DispatchHealthCheckEnabled != nil {
		s.DispatchHealthCheckEnabled = *stored.DispatchHealthCheckEnabled
	}
	if stored.RequestTimeoutSeconds != nil {
		s.RequestTimeoutSeconds = *stored.RequestTimeoutSeconds
	}
	if stored.RateLimitEnabled != nil {
		s.RateLimitEnabled = *stored.RateLimitEnabled
	}
	if stored.RateLimitPerMin != nil {
		s.RateLimitPerMin = *stored.RateLimitPerMin
	}
	if stored.LoginFailLimit != nil {
		s.LoginFailLimit = *stored.LoginFailLimit
	}
	if stored.LoginLockMinutes != nil {
		s.LoginLockMinutes = *stored.LoginLockMinutes
	}
	if stored.CaptchaEnabled != nil {
		s.CaptchaEnabled = *stored.CaptchaEnabled
	}
	if stored.CaptchaSiteKey != nil {
		s.CaptchaSiteKey = *stored.CaptchaSiteKey
	}
	if stored.CaptchaSecretKey != nil {
		s.CaptchaSecretKey = *stored.CaptchaSecretKey
	}
	if stored.CaptchaFreeCount != nil {
		s.CaptchaFreeCount = *stored.CaptchaFreeCount
	}
	if stored.MinResponseMs != nil {
		s.MinResponseMs = *stored.MinResponseMs
	}
	if stored.LogFileEnabled != nil {
		s.LogFileEnabled = *stored.LogFileEnabled
	}
	if stored.LogFilePath != nil {
		s.LogFilePath = *stored.LogFilePath
	}
	if stored.LogMaxSizeMB != nil {
		s.LogMaxSizeMB = *stored.LogMaxSizeMB
	}
	if stored.LogMaxBackups != nil {
		s.LogMaxBackups = *stored.LogMaxBackups
	}
	if stored.LogMaxAgeDays != nil {
		s.LogMaxAgeDays = *stored.LogMaxAgeDays
	}
	if stored.LogCompress != nil {
		s.LogCompress = *stored.LogCompress
	}
	if stored.AutoUpdateEnabled != nil {
		s.AutoUpdateEnabled = *stored.AutoUpdateEnabled
	}
}

func normalizeSettings(s *AppSettings) {
	if s.MaxUpstreamCheckConcurrency <= 0 {
		s.MaxUpstreamCheckConcurrency = 6
	}
	if s.RequestTimeoutSeconds <= 0 {
		s.RequestTimeoutSeconds = 45
	}
	if s.RateLimitPerMin <= 0 {
		s.RateLimitPerMin = 30
	}
	if s.LoginFailLimit <= 0 {
		s.LoginFailLimit = 5
	}
	if s.LoginLockMinutes <= 0 {
		s.LoginLockMinutes = 15
	}
	if s.CaptchaFreeCount < 0 {
		s.CaptchaFreeCount = 3
	}
	if s.MinResponseMs < 0 {
		s.MinResponseMs = 0
	}
	logging := utils.NormalizeLoggingConfig(utils.LoggingConfig{
		FileEnabled: s.LogFileEnabled,
		FilePath:    s.LogFilePath,
		MaxSizeMB:   s.LogMaxSizeMB,
		MaxBackups:  s.LogMaxBackups,
		MaxAgeDays:  s.LogMaxAgeDays,
		Compress:    s.LogCompress,
	})
	s.LogFileEnabled = logging.FileEnabled
	s.LogFilePath = logging.FilePath
	s.LogMaxSizeMB = logging.MaxSizeMB
	s.LogMaxBackups = logging.MaxBackups
	s.LogMaxAgeDays = logging.MaxAgeDays
	s.LogCompress = logging.Compress
}

func boolPtr(v bool) *bool       { return &v }
func intPtr(v int) *int          { return &v }
func stringPtr(v string) *string { return &v }

func persistRuntimeSettings(s AppSettings) error {
	payload := storedRuntimeSettings{
		MaxUpstreamCheckConcurrency: intPtr(s.MaxUpstreamCheckConcurrency),
		DispatchHealthCheckEnabled:  boolPtr(s.DispatchHealthCheckEnabled),
		RequestTimeoutSeconds:       intPtr(s.RequestTimeoutSeconds),
		RateLimitEnabled:            boolPtr(s.RateLimitEnabled),
		RateLimitPerMin:             intPtr(s.RateLimitPerMin),
		LoginFailLimit:              intPtr(s.LoginFailLimit),
		LoginLockMinutes:            intPtr(s.LoginLockMinutes),
		CaptchaEnabled:              boolPtr(s.CaptchaEnabled),
		CaptchaSiteKey:              stringPtr(s.CaptchaSiteKey),
		CaptchaSecretKey:            stringPtr(s.CaptchaSecretKey),
		CaptchaFreeCount:            intPtr(s.CaptchaFreeCount),
		MinResponseMs:               intPtr(s.MinResponseMs),
		LogFileEnabled:              boolPtr(s.LogFileEnabled),
		LogFilePath:                 stringPtr(s.LogFilePath),
		LogMaxSizeMB:                intPtr(s.LogMaxSizeMB),
		LogMaxBackups:               intPtr(s.LogMaxBackups),
		LogMaxAgeDays:               intPtr(s.LogMaxAgeDays),
		LogCompress:                 boolPtr(s.LogCompress),
		AutoUpdateEnabled:           boolPtr(s.AutoUpdateEnabled),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return database.DB.Save(&model.KV{Key: model.KVRuntimeSettings, Value: string(b)}).Error
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func looksLikeTurnstileKey(key string) bool {
	key = strings.TrimSpace(key)
	if len(key) < 20 {
		return false
	}
	prefix := strings.ToLower(key[:2])
	return prefix == "0x" || prefix == "1x" || prefix == "2x" || prefix == "3x"
}

func GetCurrentSettings() AppSettings {
	settingsMu.RLock()
	s := currentSettings
	settingsMu.RUnlock()
	return s
}

func CaptchaInfo(c *gin.Context) {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	enabled := currentSettings.CaptchaEnabled &&
		strings.TrimSpace(currentSettings.CaptchaSecretKey) != "" &&
		strings.TrimSpace(currentSettings.CaptchaSiteKey) != ""
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"enabled":   enabled,
			"siteKey":   currentSettings.CaptchaSiteKey,
			"freeCount": currentSettings.CaptchaFreeCount,
		},
	})
}

func AdminSettings(c *gin.Context) {
	s := GetCurrentSettings()
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"maxUpstreamCheckConcurrency": s.MaxUpstreamCheckConcurrency,
			"dispatchHealthCheckEnabled":  s.DispatchHealthCheckEnabled,
			"requestTimeoutSeconds":       s.RequestTimeoutSeconds,
			"rateLimitEnabled":            s.RateLimitEnabled,
			"rateLimitPerMin":             s.RateLimitPerMin,
			"loginFailLimit":              s.LoginFailLimit,
			"loginLockMinutes":            s.LoginLockMinutes,
			"captchaEnabled":              s.CaptchaEnabled,
			"captchaSiteKey":              s.CaptchaSiteKey,
			"captchaSecretConfigured":     strings.TrimSpace(s.CaptchaSecretKey) != "",
			"captchaFreeCount":            s.CaptchaFreeCount,
			"minResponseMs":               s.MinResponseMs,
			"logFileEnabled":              s.LogFileEnabled,
			"logFilePath":                 s.LogFilePath,
			"logMaxSizeMB":                s.LogMaxSizeMB,
			"logMaxBackups":               s.LogMaxBackups,
			"logMaxAgeDays":               s.LogMaxAgeDays,
			"logCompress":                 s.LogCompress,
			"autoUpdateEnabled":           s.AutoUpdateEnabled,
		},
	})
}

func UpdateAdminSettings(c *gin.Context) {
	var req struct {
		MaxUpstreamCheckConcurrency int    `json:"maxUpstreamCheckConcurrency"`
		DispatchHealthCheckEnabled  bool   `json:"dispatchHealthCheckEnabled"`
		RequestTimeoutSeconds       int    `json:"requestTimeoutSeconds"`
		RateLimitEnabled            bool   `json:"rateLimitEnabled"`
		RateLimitPerMin             int    `json:"rateLimitPerMin"`
		LoginFailLimit              int    `json:"loginFailLimit"`
		LoginLockMinutes            int    `json:"loginLockMinutes"`
		CaptchaEnabled              bool   `json:"captchaEnabled"`
		CaptchaSiteKey              string `json:"captchaSiteKey"`
		CaptchaSecretKey            string `json:"captchaSecretKey"`
		CaptchaFreeCount            int    `json:"captchaFreeCount"`
		MinResponseMs               int    `json:"minResponseMs"`
		LogFileEnabled              bool   `json:"logFileEnabled"`
		LogFilePath                 string `json:"logFilePath"`
		LogMaxSizeMB                int    `json:"logMaxSizeMB"`
		LogMaxBackups               int    `json:"logMaxBackups"`
		LogMaxAgeDays               int    `json:"logMaxAgeDays"`
		LogCompress                 bool   `json:"logCompress"`
		AutoUpdateEnabled           bool   `json:"autoUpdateEnabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请求格式错误"})
		return
	}
	if req.MaxUpstreamCheckConcurrency <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "上游检查并发数必须大于 0"})
		return
	}
	if req.RequestTimeoutSeconds <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "上游请求超时必须大于 0"})
		return
	}
	if req.RateLimitPerMin <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "接口限流次数必须大于 0"})
		return
	}
	if req.LoginFailLimit <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "登录失败限制必须大于 0"})
		return
	}
	if req.LoginLockMinutes <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "登录锁定时间必须大于 0"})
		return
	}
	if req.CaptchaFreeCount < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "验证码宽限次数不能小于 0"})
		return
	}
	if req.MinResponseMs < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "最小响应延迟不能小于 0"})
		return
	}
	if req.LogMaxSizeMB <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "日志单文件大小必须大于 0"})
		return
	}
	if req.LogMaxBackups <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "日志保留文件数必须大于 0"})
		return
	}
	if req.LogMaxAgeDays <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "日志保留天数必须大于 0"})
		return
	}

	settingsMu.RLock()
	s := currentSettings
	settingsMu.RUnlock()

	s.MaxUpstreamCheckConcurrency = req.MaxUpstreamCheckConcurrency
	s.DispatchHealthCheckEnabled = req.DispatchHealthCheckEnabled
	s.RequestTimeoutSeconds = req.RequestTimeoutSeconds
	s.RateLimitEnabled = req.RateLimitEnabled
	s.RateLimitPerMin = req.RateLimitPerMin
	s.LoginFailLimit = req.LoginFailLimit
	s.LoginLockMinutes = req.LoginLockMinutes
	s.CaptchaEnabled = req.CaptchaEnabled
	s.CaptchaSiteKey = strings.TrimSpace(req.CaptchaSiteKey)
	if secret := strings.TrimSpace(req.CaptchaSecretKey); secret != "" {
		s.CaptchaSecretKey = secret
	}
	s.CaptchaFreeCount = req.CaptchaFreeCount
	s.MinResponseMs = req.MinResponseMs
	s.LogFileEnabled = req.LogFileEnabled
	s.LogFilePath = strings.TrimSpace(req.LogFilePath)
	s.LogMaxSizeMB = req.LogMaxSizeMB
	s.LogMaxBackups = req.LogMaxBackups
	s.LogMaxAgeDays = req.LogMaxAgeDays
	s.LogCompress = req.LogCompress
	s.AutoUpdateEnabled = req.AutoUpdateEnabled
	normalizeSettings(&s)

	if s.CaptchaEnabled {
		if strings.TrimSpace(s.CaptchaSiteKey) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "启用人机验证时必须填写 Site Key"})
			return
		}
		if !looksLikeTurnstileKey(s.CaptchaSiteKey) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "Site Key 格式不正确，请填写 Cloudflare Turnstile Site Key"})
			return
		}
		if strings.TrimSpace(s.CaptchaSecretKey) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "启用人机验证时必须填写 Secret Key"})
			return
		}
		if !looksLikeTurnstileKey(s.CaptchaSecretKey) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "Secret Key 格式不正确，请填写 Cloudflare Turnstile Secret Key"})
			return
		}
	}

	if err := utils.ApplyLoggingConfig(settingsLoggingConfig(s)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "日志配置不可用: " + err.Error()})
		return
	}
	if err := persistRuntimeSettings(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "保存配置失败: " + err.Error()})
		return
	}

	settingsMu.Lock()
	currentSettings = s
	settingsMu.Unlock()
	updateSecuritySettings(s)

	AddOpLogWithCtx(c, "settings", "更新系统设置", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "配置已保存并生效"})
}

func ReloadSettings() error {
	if err := godotenv.Overload(".env"); err != nil {
		return err
	}
	LoadSettingsFromEnv()
	return nil
}

func ReloadSettingsHandler(c *gin.Context) {
	if err := ReloadSettings(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "重载失败: " + err.Error()})
		return
	}
	AddOpLogWithCtx(c, "settings", "重载 .env 配置", "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已重载，初始化密钥和管理员账号不会从 .env 读取"})
}

func settingsLoggingConfig(s AppSettings) utils.LoggingConfig {
	return utils.LoggingConfig{
		FileEnabled: s.LogFileEnabled,
		FilePath:    s.LogFilePath,
		MaxSizeMB:   s.LogMaxSizeMB,
		MaxBackups:  s.LogMaxBackups,
		MaxAgeDays:  s.LogMaxAgeDays,
		Compress:    s.LogCompress,
	}
}

func updateSecuritySettings(s AppSettings) {
	_ = utils.ApplyLoggingConfig(settingsLoggingConfig(s))
	updateUpstreamCheckConcurrency(s.MaxUpstreamCheckConcurrency)

	if ApiRateLimiter != nil {
		if s.RateLimitEnabled && s.RateLimitPerMin > 0 {
			ApiRateLimiter.Update(s.RateLimitPerMin, time.Minute)
		} else {
			ApiRateLimiter.Update(0, time.Minute)
		}
	}
	if middleware.LoginLimit != nil {
		if s.LoginFailLimit > 0 {
			lockout := time.Duration(s.LoginLockMinutes) * time.Minute
			if lockout <= 0 {
				lockout = 15 * time.Minute
			}
			middleware.LoginLimit.Update(s.LoginFailLimit, 10*time.Minute, lockout)
		} else {
			middleware.LoginLimit.Update(0, 0, 0)
		}
	}
	captchaEnabled := s.CaptchaEnabled &&
		strings.TrimSpace(s.CaptchaSecretKey) != "" &&
		strings.TrimSpace(s.CaptchaSiteKey) != ""
	middleware.UpdateCaptchaConfig(middleware.CaptchaConfig{
		Enabled:       captchaEnabled,
		SecretKey:     s.CaptchaSecretKey,
		FreeCount:     s.CaptchaFreeCount,
		FreeWindowSec: 60,
	})
	middleware.UpdateMinDelay(s.MinResponseMs)
}
