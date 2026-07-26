package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var LoginLimit *LoginLimiter

var jwtSecret []byte

const legacyDefaultJWTSecret = "default_secret_please_change_in_production"

func InitJWT() {
	if len(jwtSecret) > 0 {
		return
	}
	secret, err := randomJWTSecret()
	if err != nil {
		log.Println("WARN: 生成临时 JWT 密钥失败，正在使用不安全的默认值")
		secret = legacyDefaultJWTSecret
	} else {
		log.Println("JWT 密钥尚未初始化，当前使用进程临时密钥")
	}
	SetJWTSecret(secret)
}

func SetJWTSecret(secret string) {
	jwtSecret = []byte(secret)
}

func randomJWTSecret() (string, error) {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

type Claims struct {
	Username string `json:"username"`
	Version  int    `json:"v"`
	jwt.RegisteredClaims
}

func GenerateToken(username string, version int) (string, error) {
	claims := Claims{
		Username: username,
		Version:  version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "kiroclaim",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

var TokenVersionLookup func(username string) (int, bool)

func ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}

	if TokenVersionLookup != nil {
		version, found := TokenVersionLookup(claims.Username)
		if !found || version != claims.Version {
			return nil, jwt.ErrSignatureInvalid
		}
	}

	return claims, nil
}

func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if LoginLimit != nil && LoginLimit.IsLocked(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code": 1, "message": "登录失败次数过多，请稍后再试",
			})
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 1, "message": "缺少认证 Token",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 1, "message": "Token 格式错误",
			})
			return
		}

		claims, err := ValidateToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code": 1, "message": "Token 无效或已过期",
			})
			return
		}

		if LoginLimit != nil {
			LoginLimit.ResetIP(ip)
		}

		c.Set("username", claims.Username)
		c.Next()
	}
}
