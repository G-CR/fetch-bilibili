package db

import (
	"context"
	"database/sql"
	"time"

	"fetch-bilibili/internal/config"
	_ "github.com/go-sql-driver/mysql"
)

var sqlOpen = sql.Open

func NewMySQL(cfg config.MySQLConfig) (*sql.DB, error) {
	db, err := sqlOpen("mysql", cfg.DSN)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}
