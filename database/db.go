package database

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/wp13461544040/KiroClaim/model"
	"github.com/wp13461544040/KiroClaim/utils"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func WhereKVKey(db *gorm.DB, key string) *gorm.DB {
	return db.Where(&model.KV{Key: key})
}

// withSQLitePragmas 把连接级 pragma 追加到 DSN，保证每条新连接都带上这些设置。
//
// synchronous 是每连接生效的：连接建立后再执行 PRAGMA 只作用于恰好服务那条语句的
// 连接，池里其余连接仍是默认的 FULL。journal_mode 写在数据库文件头因此是持久的，
// busy_timeout 由驱动在每条新连接上默认设为 5000，这两项本来就没问题。
// 已有 _pragma 参数时原样返回，避免覆盖用户自定义配置。
func withSQLitePragmas(dsn string) string {
	if strings.Contains(dsn, "_pragma=") {
		return dsn
	}
	pragmas := "_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	if strings.Contains(dsn, "?") {
		return dsn + "&" + pragmas
	}
	return dsn + "?" + pragmas
}

// Init 初始化数据库连接。
// DB_TYPE 支持 sqlite（默认）和 mysql。
// SQLite: DB_PATH=app.db（默认）
// MySQL: DB_DSN=user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
func Init(dsn string) {
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}

	var err error
	var dialector gorm.Dialector

	switch dbType {
	case "mysql":
		mysqlDSN := os.Getenv("DB_DSN")
		if mysqlDSN == "" {
			log.Fatal("使用 MySQL 时必须设置 DB_DSN 环境变量")
		}
		dialector = mysql.Open(mysqlDSN)
		log.Printf("使用 MySQL 数据库: %s", maskDSN(mysqlDSN))

	case "sqlite":
		if dsn == "" {
			dsn = "app.db"
		}
		// pragma 必须写进 DSN。busy_timeout / synchronous 是每连接生效的，
		// 连接建立后再执行 PRAGMA 只作用于恰好服务那条语句的连接，
		// 之后新建的连接会退回 SQLite 默认值（busy_timeout=0）并在并发写时立刻报 database is locked。
		dialector = sqlite.Open(withSQLitePragmas(dsn))
		log.Printf("使用 SQLite 数据库: %s", dsn)

	default:
		log.Fatalf("不支持的数据库类型: %s（支持 sqlite 或 mysql）", dbType)
	}

	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	if dbType == "mysql" {
		sqlDB, err := DB.DB()
		if err == nil {
			sqlDB.SetMaxOpenConns(25)
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetConnMaxLifetime(5 * time.Minute)
			log.Println("MySQL 连接池已配置: MaxOpen=25, MaxIdle=10, MaxLifetime=5m")
		}
	}

	if dbType == "sqlite" {
		// WAL 下读不阻塞写，所以不把连接数收到 1，否则列表和导出的并发读也会排队。
		// 写冲突由 busy_timeout 加应用层的 healthUpdateMu 兜住。
		// 这里只固定空闲连接数，避免高并发时反复建连（每条新连接都要重跑一遍 pragma）。
		if sqlDB, err := DB.DB(); err == nil {
			sqlDB.SetMaxOpenConns(8)
			sqlDB.SetMaxIdleConns(8)
			sqlDB.SetConnMaxLifetime(0)
		}
		log.Println("SQLite 已配置: journal_mode=WAL, synchronous=NORMAL, busy_timeout=5s, MaxOpen=8, MaxIdle=8")
	}
	if err := dropDeprecatedCommerceColumns(DB); err != nil {
		log.Fatalf("清理废弃商城字段失败: %v", err)
	}

	err = DB.AutoMigrate(
		&model.Account{},
		&model.Card{},
		&model.CardAccount{},
		&model.KV{},
		&model.OpLog{},
		&model.CardLog{},
		&model.User{},
		&model.CommerceProduct{},
		&model.CommerceProductCard{},
		&model.CommerceOrder{},
		&model.CommercePaymentProof{},
		&model.CommercePaymentEvent{},
		&model.CommerceOrderLog{},
	)
	if err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	if err := cleanupEmptyCommerceProducts(DB); err != nil {
		log.Fatalf("cleanup empty commerce products failed: %v", err)
	}

	loadCryptoKeyFromKV()
	migrateEncryptedCredentials()

	log.Println("数据库初始化完成")
}

func cleanupEmptyCommerceProducts(db *gorm.DB) error {
	return db.Exec("DELETE FROM commerce_products WHERE active = ? AND id NOT IN (SELECT DISTINCT product_id FROM commerce_product_cards)", false).Error
}

func dropDeprecatedCommerceColumns(db *gorm.DB) error {
	if db.Migrator().HasTable(&model.CommerceProduct{}) && db.Migrator().HasColumn(&model.CommerceProduct{}, "inventory_mode") {
		if err := db.Migrator().DropColumn(&model.CommerceProduct{}, "inventory_mode"); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable(&model.CommerceOrder{}) && db.Migrator().HasColumn(&model.CommerceOrder{}, "inventory_mode") {
		if err := db.Migrator().DropColumn(&model.CommerceOrder{}, "inventory_mode"); err != nil {
			return err
		}
	}
	return dropDeprecatedCommercePriceTable(db)
}

func dropDeprecatedCommercePriceTable(db *gorm.DB) error {
	if db.Migrator().HasTable("commerce_product_prices") {
		if err := db.Migrator().DropTable("commerce_product_prices"); err != nil {
			return err
		}
	}
	if db.Migrator().HasTable("commerce_payment_channels") {
		return db.Migrator().DropTable("commerce_payment_channels")
	}
	return nil
}

func loadCryptoKeyFromKV() {
	if utils.CryptoEnabled() {
		return
	}
	var kv model.KV
	result := WhereKVKey(DB, model.KVEncryptionKey).Find(&kv)
	if result.Error != nil || result.RowsAffected == 0 {
		return
	}
	if err := utils.SetCryptoKey(kv.Value); err != nil {
		log.Printf("从 KV 加载账号凭证加密密钥失败: %v", err)
	}
}

func migrateEncryptedCredentials() {
	if !utils.CryptoEnabled() {
		return
	}
	var flag model.KV
	const flagKey = "accounts_encrypted_v1"
	if result := WhereKVKey(DB, flagKey).Find(&flag); result.Error == nil && result.RowsAffected > 0 {
		return
	}

	var accounts []model.Account
	if err := DB.Find(&accounts).Error; err != nil {
		log.Printf("凭证加密迁移扫描失败: %v", err)
		return
	}
	changed := 0
	for _, a := range accounts {
		if utils.IsEncrypted(a.RefreshToken) &&
			utils.IsEncrypted(a.AccessToken) &&
			utils.IsEncrypted(a.ClientSecret) {
			continue
		}
		if err := DB.Save(&a).Error; err != nil {
			log.Printf("凭证加密迁移写回失败: account id=%d err=%v", a.ID, err)
			continue
		}
		changed++
	}
	if changed > 0 {
		log.Printf("凭证加密迁移完成: %d 条账号已重写为密文", changed)
	}
	DB.Save(&model.KV{Key: flagKey, Value: "1"})
}

func maskDSN(dsn string) string {
	if len(dsn) > 20 {
		return dsn[:10] + "****" + dsn[len(dsn)-10:]
	}
	return "****"
}
