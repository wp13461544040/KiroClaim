package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/huey1in/KiroClaim/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	srcPath := flag.String("src", "app.db", "source sqlite database")
	dstPath := flag.String("dst", "app.db.repaired", "destination sqlite database")
	checkOnly := flag.Bool("check-only", false, "only run PRAGMA integrity_check")
	flag.Parse()

	src, err := gorm.Open(sqlite.Open("file:"+*srcPath+"?mode=ro"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("open source: %v", err)
	}
	if *checkOnly {
		checkIntegrity(src)
		return
	}

	_ = os.Remove(*dstPath)
	_ = os.Remove(*dstPath + "-wal")
	_ = os.Remove(*dstPath + "-shm")

	dst, err := gorm.Open(sqlite.Open(*dstPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("open destination: %v", err)
	}
	if err := dst.AutoMigrate(
		&model.Account{},
		&model.Card{},
		&model.CardAccount{},
		&model.KV{},
		&model.OpLog{},
		&model.CardLog{},
		&model.User{},
	); err != nil {
		log.Fatalf("migrate destination: %v", err)
	}

	checkIntegrity(src)

	tables := []string{"accounts", "cards", "card_accounts", "kvs", "op_logs", "card_logs", "users"}
	for _, table := range tables {
		copyTable(src, dst, table)
	}

	fmt.Printf("repair output: %s\n", *dstPath)
}

func checkIntegrity(db *gorm.DB) {
	var rows []string
	if err := db.Raw("PRAGMA integrity_check").Scan(&rows).Error; err != nil {
		fmt.Printf("integrity_check error: %v\n", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	fmt.Println("integrity_check:")
	for _, row := range rows {
		fmt.Println(row)
	}
}

func copyTable(src, dst *gorm.DB, table string) {
	srcSQL, err := src.DB()
	if err != nil {
		fmt.Printf("%s: source handle failed: %v\n", table, err)
		return
	}
	dstSQL, err := dst.DB()
	if err != nil {
		fmt.Printf("%s: destination handle failed: %v\n", table, err)
		return
	}

	columns, err := tableColumns(srcSQL, table)
	if err != nil {
		fmt.Printf("%s: read columns failed: %v\n", table, err)
		return
	}
	if len(columns) == 0 {
		fmt.Printf("%s: copied 0, skipped 0\n", table)
		return
	}

	query := "SELECT " + quoteList(columns) + " FROM " + quoteIdent(table)
	rows, err := srcSQL.Query(query)
	if err != nil {
		fmt.Printf("%s: read rows failed: %v\n", table, err)
		return
	}
	defer rows.Close()

	insertSQL := "INSERT INTO " + quoteIdent(table) + " (" + quoteList(columns) + ") VALUES (" + placeholders(len(columns)) + ")"
	copied, skipped := 0, 0
	for rows.Next() {
		values := make([]interface{}, len(columns))
		scans := make([]interface{}, len(columns))
		for i := range values {
			scans[i] = &values[i]
		}
		if err := rows.Scan(scans...); err != nil {
			skipped++
			continue
		}
		if _, err := dstSQL.Exec(insertSQL, values...); err != nil {
			skipped++
			continue
		}
		copied++
	}
	if err := rows.Err(); err != nil {
		fmt.Printf("%s: row scan stopped: %v\n", table, err)
	}
	fmt.Printf("%s: copied %d, skipped %d\n", table, copied, skipped)
}

func tableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

func quoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

func quoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = quoteIdent(item)
	}
	return strings.Join(quoted, ",")
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}
