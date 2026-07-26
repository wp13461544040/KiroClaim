package database

import (
	"strings"
	"testing"

	"github.com/huey1in/KiroClaim/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestWhereKVKeyQuotesMySQLReservedColumn(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "user:password@tcp(127.0.0.1:3306)/kiroclaim",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}

	var kv model.KV
	query := WhereKVKey(db, model.KVCommerceSettings).Find(&kv)
	sql := query.Statement.SQL.String()
	if !strings.Contains(sql, "`key` = ?") {
		t.Fatalf("KVS key column is not quoted for MySQL: %s", sql)
	}
	if strings.Contains(sql, "WHERE key = ?") {
		t.Fatalf("KVS query still uses the unquoted MySQL reserved word: %s", sql)
	}
}
