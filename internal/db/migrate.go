package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"

	appmigrations "fetch-bilibili/migrations"

	"github.com/pressly/goose/v3"
)

var gooseSetBaseFS = func(base fs.FS) {
	goose.SetBaseFS(base)
}
var gooseSetDialect = goose.SetDialect
var gooseUp = func(ctx context.Context, db *sql.DB, dir string) error {
	return goose.UpContext(ctx, db, dir)
}

func RunMySQLMigrations(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return fmt.Errorf("数据库连接不能为空")
	}

	gooseSetBaseFS(appmigrations.FS)
	if err := gooseSetDialect("mysql"); err != nil {
		return fmt.Errorf("设置数据库迁移方言失败: %w", err)
	}
	if err := gooseUp(ctx, database, "."); err != nil {
		return fmt.Errorf("执行数据库迁移失败: %w", err)
	}

	log.Printf("数据库迁移完成")
	return nil
}
