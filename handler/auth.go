package handler

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/middleware"
	"github.com/huey1in/KiroClaim/model"
	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type tokenVersionEntry struct {
	version  int
	cachedAt time.Time
}

type setupCheckItem struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Icon     string `json:"icon"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
}

var (
	tokenVerCache   = make(map[string]tokenVersionEntry)
	tokenVerCacheMu sync.RWMutex
	adminSetupMu    sync.Mutex
)

const tokenVerCacheTTL = 60 * time.Second

func init() {
	middleware.TokenVersionLookup = lookupTokenVersion
}

func lookupTokenVersion(username string) (int, bool) {
	tokenVerCacheMu.RLock()
	if e, ok := tokenVerCache[username]; ok && time.Since(e.cachedAt) < tokenVerCacheTTL {
		tokenVerCacheMu.RUnlock()
		return e.version, true
	}
	tokenVerCacheMu.RUnlock()

	var user model.User
	if err := database.DB.Select("token_version").Where("username = ?", username).First(&user).Error; err != nil {
		return 0, false
	}
	tokenVerCacheMu.Lock()
	tokenVerCache[username] = tokenVersionEntry{version: user.TokenVersion, cachedAt: time.Now()}
	tokenVerCacheMu.Unlock()
	return user.TokenVersion, true
}

func invalidateTokenVersion(username string) {
	tokenVerCacheMu.Lock()
	delete(tokenVerCache, username)
	tokenVerCacheMu.Unlock()
}

func BootstrapAdminUser() {
	var count int64
	if err := database.DB.Model(&model.User{}).Count(&count).Error; err != nil {
		log.Printf("WARN: 读取 users 表失败: %v", err)
		return
	}
	if count > 0 {
		return
	}
	log.Println("未检测到管理员账号，请打开管理后台完成首次初始化")
}

func LoadRuntimeSecrets() {
	var jwtSecret model.KV
	if result := database.WhereKVKey(database.DB, model.KVJWTSecret).Find(&jwtSecret); result.Error == nil && result.RowsAffected > 0 && strings.TrimSpace(jwtSecret.Value) != "" {
		middleware.SetJWTSecret(jwtSecret.Value)
		log.Println("JWT 密钥已从本地 KV 加载")
	}

	var encryptionKey model.KV
	if result := database.WhereKVKey(database.DB, model.KVEncryptionKey).Find(&encryptionKey); result.Error == nil && result.RowsAffected > 0 && strings.TrimSpace(encryptionKey.Value) != "" {
		if err := utils.SetCryptoKey(encryptionKey.Value); err != nil {
			log.Printf("账号凭证加密密钥加载失败: %v", err)
		}
	}
}

func AdminSetupStatus(c *gin.Context) {
	var count int64
	if err := database.DB.Model(&model.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "读取管理员状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"initialized": count > 0},
	})
}

func AdminSetupChecks(c *gin.Context) {
	checks, ready := collectSetupChecks()
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"ready":  ready,
			"checks": checks,
		},
	})
}

func AdminSetup(c *gin.Context) {
	if checks, ready := collectSetupChecks(); !ready {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    1,
			"message": "本地环境配置检查未通过，请修复后重新检测",
			"data":    gin.H{"checks": checks},
		})
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "请输入管理员账号和密码"})
		return
	}

	username := strings.TrimSpace(req.Username)
	password := strings.TrimSpace(req.Password)
	if len(username) < 3 || len(username) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "用户名长度需要为 3-64 个字符"})
		return
	}
	if len(password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "密码至少需要 8 个字符"})
		return
	}

	adminSetupMu.Lock()
	defer adminSetupMu.Unlock()

	var count int64
	if err := database.DB.Model(&model.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "读取管理员状态失败"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": 1, "message": "管理员账号已初始化，请直接登录"})
		return
	}

	if err := ensureRuntimeSecrets(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "生成初始化密钥失败"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "密码哈希失败"})
		return
	}

	user := model.User{
		Username:     username,
		PasswordHash: string(hash),
		TokenVersion: 1,
	}
	if err := database.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "初始化管理员账号失败"})
		return
	}

	tokenString, err := middleware.GenerateToken(user.Username, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "生成 Token 失败"})
		return
	}

	log.Printf("已初始化管理员账号: %s", username)
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "管理员账号已创建",
		"data": gin.H{
			"token":    tokenString,
			"username": user.Username,
		},
	})
}

func collectSetupChecks() ([]setupCheckItem, bool) {
	checks := []setupCheckItem{
		checkServerRuntime(),
		checkServerCPU(),
		checkServerPort(),
		checkDatabaseType(),
		checkDatabaseConfig(),
		checkDatabaseConnection(),
	}
	ready := true
	for _, item := range checks {
		if item.Required && !item.OK {
			ready = false
			break
		}
	}
	return checks, ready
}

func checkServerRuntime() setupCheckItem {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}
	return setupCheckItem{
		Key:      "server_runtime",
		Label:    "服务器配置",
		Icon:     "server",
		OK:       true,
		Required: true,
		Message:  hostname + " · " + runtime.GOOS + "/" + runtime.GOARCH + " · Go " + runtime.Version(),
	}
}

func checkServerCPU() setupCheckItem {
	cores := runtime.NumCPU()
	if cores <= 0 {
		return setupCheckItem{
			Key: "server_cpu", Label: "CPU", Icon: "cpu", OK: false, Required: true,
			Message: "无法读取 CPU 核心数",
		}
	}
	return setupCheckItem{
		Key: "server_cpu", Label: "CPU", Icon: "cpu", OK: true, Required: true,
		Message: strconv.Itoa(cores) + " 个逻辑核心",
	}
}

func checkServerPort() setupCheckItem {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "9527"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return setupCheckItem{
			Key: "server_port", Label: "服务端口", Icon: "port", OK: false, Required: true,
			Message: "PORT 必须是 1-65535 之间的数字",
		}
	}
	return setupCheckItem{
		Key: "server_port", Label: "服务端口", Icon: "port", OK: true, Required: true,
		Message: "本地服务监听端口 " + port,
	}
}

func checkDatabaseType() setupCheckItem {
	dbType := strings.ToLower(strings.TrimSpace(os.Getenv("DB_TYPE")))
	if dbType == "" {
		dbType = "sqlite"
	}
	switch dbType {
	case "sqlite":
		return setupCheckItem{
			Key: "database_type", Label: "数据库类型", Icon: "sqlite", OK: true, Required: true,
			Message: "开发环境使用 SQLite",
		}
	case "mysql":
		return setupCheckItem{
			Key: "database_type", Label: "数据库类型", Icon: "mysql", OK: true, Required: true,
			Message: "生产环境使用 MySQL",
		}
	case "postgres", "postgresql":
		return setupCheckItem{
			Key: "database_type", Label: "数据库类型", Icon: "postgres", OK: false, Required: true,
			Message: "已识别 PostgreSQL，但当前服务尚未启用 postgres 驱动",
		}
	default:
		return setupCheckItem{
			Key: "database_type", Label: "数据库类型", Icon: "database", OK: false, Required: true,
			Message: "DB_TYPE 只支持 sqlite 或 mysql",
		}
	}
}

func checkDatabaseConfig() setupCheckItem {
	dbType := strings.ToLower(strings.TrimSpace(os.Getenv("DB_TYPE")))
	if dbType == "" {
		dbType = "sqlite"
	}
	switch dbType {
	case "sqlite":
		dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
		if dbPath == "" {
			dbPath = "app.db"
		}
		abs, err := filepath.Abs(dbPath)
		if err != nil {
			return setupCheckItem{
				Key: "database_config", Label: "数据库配置", Icon: "database", OK: false, Required: true,
				Message: "DB_PATH 路径无效",
			}
		}
		dir := filepath.Dir(abs)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return setupCheckItem{
				Key: "database_config", Label: "数据库配置", Icon: "database", OK: false, Required: true,
				Message: "无法创建数据库目录",
			}
		}
		tmp, err := os.CreateTemp(dir, ".setup-write-test-*")
		if err != nil {
			return setupCheckItem{
				Key: "database_config", Label: "数据库配置", Icon: "database", OK: false, Required: true,
				Message: "数据库目录不可写",
			}
		}
		tmp.Close()
		os.Remove(tmp.Name())
		return setupCheckItem{
			Key: "database_config", Label: "数据库配置", Icon: "database", OK: true, Required: true,
			Message: "SQLite 路径可写",
		}
	case "mysql":
		if strings.TrimSpace(os.Getenv("DB_DSN")) == "" {
			return setupCheckItem{
				Key: "database_config", Label: "数据库配置", Icon: "mysql", OK: false, Required: true,
				Message: "MySQL 需要配置 DB_DSN",
			}
		}
		return setupCheckItem{
			Key: "database_config", Label: "数据库配置", Icon: "mysql", OK: true, Required: true,
			Message: "MySQL DSN 已配置",
		}
	default:
		return setupCheckItem{
			Key: "database_config", Label: "数据库配置", Icon: "database", OK: false, Required: true,
			Message: "DB_TYPE 只支持 sqlite 或 mysql",
		}
	}
}

func checkDatabaseConnection() setupCheckItem {
	if database.DB == nil {
		return setupCheckItem{
			Key: "database_connection", Label: "数据库连接", Icon: "plug", OK: false, Required: true,
			Message: "数据库尚未初始化",
		}
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		return setupCheckItem{
			Key: "database_connection", Label: "数据库连接", Icon: "plug", OK: false, Required: true,
			Message: "无法获取数据库连接",
		}
	}
	if err := sqlDB.Ping(); err != nil {
		return setupCheckItem{
			Key: "database_connection", Label: "数据库连接", Icon: "plug", OK: false, Required: true,
			Message: "数据库连接失败",
		}
	}
	return setupCheckItem{
		Key: "database_connection", Label: "数据库连接", Icon: "plug", OK: true, Required: true,
		Message: "数据库连接正常",
	}
}

func ensureRuntimeSecrets() error {
	jwtSecret, err := ensureKVSecret(model.KVJWTSecret, 48)
	if err != nil {
		return err
	}
	middleware.SetJWTSecret(jwtSecret)

	encryptionKey, err := ensureKVSecret(model.KVEncryptionKey, 32)
	if err != nil {
		return err
	}
	return utils.SetCryptoKey(encryptionKey)
}

func ensureKVSecret(key string, size int) (string, error) {
	var kv model.KV
	if result := database.WhereKVKey(database.DB, key).Find(&kv); result.Error == nil && result.RowsAffected > 0 && strings.TrimSpace(kv.Value) != "" {
		return kv.Value, nil
	}
	value, err := utils.GenerateBase64Secret(size)
	if err != nil {
		return "", err
	}
	if err := database.DB.Save(&model.KV{Key: key, Value: value}).Error; err != nil {
		return "", err
	}
	return value, nil
}

func AdminLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误"})
		return
	}

	ip := c.ClientIP()
	if middleware.LoginLimit != nil && middleware.LoginLimit.IsLocked(ip) {
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 1, "message": "登录失败次数过多，请稍后再试"})
		return
	}

	var user model.User
	if err := database.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		recordLoginFailure(ip)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 1, "message": "用户名或密码错误"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		recordLoginFailure(ip)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 1, "message": "用户名或密码错误"})
		return
	}
	if middleware.LoginLimit != nil {
		middleware.LoginLimit.ResetIP(ip)
	}

	tokenString, err := middleware.GenerateToken(user.Username, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "生成 Token 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "登录成功",
		"data": gin.H{
			"token":    tokenString,
			"username": user.Username,
		},
	})
}

func recordLoginFailure(ip string) {
	if middleware.LoginLimit != nil {
		middleware.LoginLimit.RecordFailure(ip)
	}
}

func AdminMe(c *gin.Context) {
	username, _ := c.Get("username")
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"username": username},
	})
}

func AdminLogout(c *gin.Context) {
	username, _ := c.Get("username")
	uname, _ := username.(string)
	if uname == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 1, "message": "未登录"})
		return
	}
	if err := database.DB.Model(&model.User{}).
		Where("username = ?", uname).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	invalidateTokenVersion(uname)
	AddOpLogWithCtx(c, "logout", "管理员登出 "+uname, "admin")
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "已登出"})
}

func AdminChangePassword(c *gin.Context) {
	username, _ := c.Get("username")
	uname, _ := username.(string)
	if uname == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 1, "message": "未登录"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": err.Error()})
		return
	}

	var user model.User
	if err := database.DB.Where("username = ?", uname).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 1, "message": "用户不存在"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 1, "message": "旧密码错误"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "密码哈希失败"})
		return
	}

	newVersion := user.TokenVersion + 1
	if err := database.DB.Model(&user).Updates(map[string]interface{}{
		"password_hash": string(hash),
		"token_version": newVersion,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	invalidateTokenVersion(uname)

	newToken, _ := middleware.GenerateToken(uname, newVersion)
	AddOpLogWithCtx(c, "settings", "管理员改密 "+uname, "admin")
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "密码已更新",
		"data":    gin.H{"token": newToken},
	})
}
