package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// 对接用 API Key 认证。
//
// 与管理员 JWT 分开是有意的：外部程序（例如批量注册脚本）只需要投递账号和查库存，
// 不应该顺带获得清空号池、导出明文凭证、改系统配置的能力。
// 因此这里的 Key 只挂在 /openapi 分组上，拿到 Key 也访问不了 /admin/*。
type apiKeyConfig struct {
	mu      sync.RWMutex
	enabled bool
	key     string
}

var openAPIKeyConfig apiKeyConfig

// UpdateOpenAPIKey 在配置变更时热更新，无需重启。
func UpdateOpenAPIKey(enabled bool, key string) {
	openAPIKeyConfig.mu.Lock()
	openAPIKeyConfig.enabled = enabled
	openAPIKeyConfig.key = key
	openAPIKeyConfig.mu.Unlock()
}

// OpenAPIKeyConfigured 供设置接口回显状态，不暴露 Key 本身。
func OpenAPIKeyConfigured() bool {
	openAPIKeyConfig.mu.RLock()
	defer openAPIKeyConfig.mu.RUnlock()
	return strings.TrimSpace(openAPIKeyConfig.key) != ""
}

// APIKeyAuth 校验 X-API-Key 头，也接受 Authorization: Bearer <key>。
func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		openAPIKeyConfig.mu.RLock()
		enabled := openAPIKeyConfig.enabled
		expected := openAPIKeyConfig.key
		openAPIKeyConfig.mu.RUnlock()

		if !enabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": 1, "message": "对接接口未启用，请在后台设置中开启",
			})
			return
		}
		// 未配置 Key 时一律拒绝，避免"启用了但没设 Key"变成无鉴权入口
		if strings.TrimSpace(expected) == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": 1, "message": "对接接口未配置 API Key",
			})
			return
		}

		provided := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if provided == "" {
			if auth := c.GetHeader("Authorization"); auth != "" {
				if parts := strings.SplitN(auth, " ", 2); len(parts) == 2 && parts[0] == "Bearer" {
					provided = strings.TrimSpace(parts[1])
				}
			}
		}
		if provided == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 1, "message": "缺少 API Key，请通过 X-API-Key 请求头传递",
			})
			return
		}

		// 定长比较，避免通过响应时间差逐字节推断 Key
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 1, "message": "API Key 无效",
			})
			return
		}

		c.Next()
	}
}
