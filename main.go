package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/huey1in/KiroClaim/database"
	"github.com/huey1in/KiroClaim/handler"
	"github.com/huey1in/KiroClaim/middleware"
	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	envFile := ".env"
	if err := godotenv.Load(envFile); err != nil {
		if exePath, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exePath)
			envFile = filepath.Join(exeDir, ".env")
			if err := godotenv.Load(envFile); err != nil {
				log.Println("未找到 .env 文件，使用系统环境变量")
			} else {
				log.Printf("已加载配置文件: %s", envFile)
			}
		} else {
			log.Println("未找到 .env 文件，使用系统环境变量")
		}
	} else {
		log.Printf("已加载配置文件: %s", envFile)
	}

	utils.InitLogging()
	utils.InitCrypto()

	dsn := os.Getenv("DB_PATH")
	if dsn == "" {
		dsn = "app.db"
	}
	database.Init(dsn)

	handler.LoadRuntimeSecrets()
	handler.BootstrapAdminUser()
	middleware.InitJWT()

	port := os.Getenv("PORT")
	if port == "" {
		port = "9527"
	}

	handler.LoadSettingsFromEnv()
	settings := handler.GetCurrentSettings()
	apiLimiter := middleware.NewRateLimiter(settings.RateLimitPerMin, time.Minute)
	middleware.LoginLimit = middleware.NewLoginLimiter(
		settings.LoginFailLimit,
		10*time.Minute,
		time.Duration(settings.LoginLockMinutes)*time.Minute,
	)
	handler.ApiRateLimiter = apiLimiter

	if !settings.RateLimitEnabled {
		apiLimiter.Update(0, time.Minute)
	}

	r := gin.Default()
	r.Use(middleware.RequestIDMiddleware())
	r.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" || path == "/admin" || path == "/setup" || path == "/redeem" ||
			strings.HasPrefix(path, "/static/") || path == "/favicon.ico" {
			c.Header("Cache-Control", "no-cache")
		}
		c.Next()
	})

	r.Static("/static", "./static")
	r.StaticFile("/favicon.ico", "./static/favicon.ico")
	registerPageRoutes(r)

	r.GET("/admin/setup/status", handler.AdminSetupStatus)
	r.GET("/admin/setup/checks", handler.AdminSetupChecks)
	r.POST("/admin/setup", handler.AdminSetup)
	r.POST("/admin/login", handler.AdminLogin)

	r.GET("/api/captcha/info", handler.CaptchaInfo)

	if mu, mp := os.Getenv("METRICS_USER"), os.Getenv("METRICS_PASS"); mu != "" && mp != "" {
		r.GET("/admin/metrics", middleware.BasicAuthMiddleware(mu, mp), gin.WrapH(promhttp.Handler()))
		log.Println("Prometheus metrics 已挂载到 /admin/metrics（Basic Auth）")
	}

	captchaMw := middleware.CaptchaMiddleware()
	minDelayMw := middleware.MinDelayMiddleware(settings.MinResponseMs)
	idempotentMw := middleware.IdempotencyMiddleware(10 * time.Minute)
	api := r.Group("/api", middleware.RateLimitMiddleware(apiLimiter))
	{
		api.POST("/activate", captchaMw, minDelayMw, idempotentMw, handler.Activate)
		api.GET("/status", captchaMw, minDelayMw, handler.Status)
		api.GET("/shop/products", handler.PublicShopProducts)
		api.POST("/shop/orders", idempotentMw, handler.CreateShopOrder)
		api.POST("/shop/orders/query", handler.QueryShopOrder)
		api.POST("/shop/orders/proof", handler.SubmitShopPaymentProof)
	}
	r.POST("/api/shop/callback/:channelId", handler.ShopPaymentCallback)
	r.GET("/api/shop/callback/:channelId", handler.ShopPaymentCallback)

	r.GET("/token/:code", middleware.RateLimitMiddleware(apiLimiter), captchaMw, minDelayMw, handler.GetToken)

	admin := r.Group("/admin", middleware.AdminAuth())
	{
		admin.GET("/me", handler.AdminMe)

		admin.POST("/accounts/import", handler.ImportAccounts)
		admin.GET("/accounts/import/status/:taskId", handler.ImportStatus)
		admin.GET("/accounts", handler.ListAccounts)
		admin.GET("/accounts/subscription-stats", handler.AccountSubscriptionStats)
		admin.GET("/accounts/:id/detail", handler.AccountDetail)
		admin.GET("/accounts/:id/models", handler.AccountModels)
		admin.POST("/accounts/:id/refresh", handler.RefreshAccount)
		admin.DELETE("/accounts/:id", handler.DeleteAccount)
		admin.POST("/accounts/batch-delete", handler.BatchDeleteAccounts)
		admin.POST("/accounts/delete-by-status", handler.DeleteAccountsByStatus)
		admin.POST("/accounts/clear-all", handler.ClearAllAccounts)
		admin.POST("/accounts/clear-assigned", handler.ClearAssignedAccounts)
		admin.GET("/pool/stats", handler.PoolStats)

		admin.POST("/cards/generate", handler.GenerateCards)
		admin.GET("/cards", handler.ListCards)
		admin.POST("/cards/shop-products/delist-group", handler.DelistCommerceProductGroupCards)
		admin.DELETE("/cards/:id", handler.DeleteCard)
		admin.POST("/cards/batch-delete", handler.BatchDeleteCards)
		admin.GET("/cards/:id/logs", handler.ListCardLogs)

		admin.GET("/oplogs", handler.ListOpLogs)

		admin.POST("/logout", handler.AdminLogout)
		admin.POST("/password", handler.AdminChangePassword)

		admin.GET("/settings", handler.AdminSettings)
		admin.POST("/settings", handler.UpdateAdminSettings)
		admin.POST("/settings/reload", handler.ReloadSettingsHandler)
		admin.GET("/version", handler.AdminVersion)
		admin.POST("/version/update", handler.AdminVersionUpdate)

		admin.GET("/commerce/channels", handler.AdminCommerceChannels)
		admin.POST("/commerce/channels", handler.AdminCommerceChannels)
		admin.DELETE("/commerce/channels/:id", handler.AdminCommerceChannelDelete)
		admin.GET("/commerce/orders", handler.AdminCommerceOrders)
		admin.GET("/commerce/orders/:orderNo", handler.AdminCommerceOrderDetail)
		admin.POST("/commerce/orders/:orderNo/review", handler.AdminCommerceReview)
		admin.GET("/commerce/proofs/:proofId/:index", handler.AdminCommerceProof)
		admin.GET("/commerce/settings", handler.AdminCommerceSettings)
		admin.POST("/commerce/settings", handler.AdminCommerceSettings)
	}

	handler.StartAutoUpdateScheduler()

	go func() {
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		for range hup {
			if err := handler.ReloadSettings(); err != nil {
				log.Printf("SIGHUP 重载失败: %v", err)
			} else {
				log.Println("SIGHUP 已重载 .env 配置")
			}
		}
	}()

	server := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		log.Printf("KiroClaim 启动，监听 :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，开始优雅关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown 异常: %v", err)
	}
	log.Println("已优雅退出")
}

func registerPageRoutes(r gin.IRoutes) {
	r.GET("/", func(c *gin.Context) { c.File("./static/shop.html") })
	r.GET("/admin", func(c *gin.Context) { c.File("./static/index.html") })
	r.GET("/setup", func(c *gin.Context) { c.File("./static/setup.html") })
	r.GET("/redeem", func(c *gin.Context) { c.File("./static/redeem.html") })
}
