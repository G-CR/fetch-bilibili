package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"fetch-bilibili/internal/config"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNewMySQLSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()
	mock.ExpectPing()

	orig := sqlOpen
	defer func() { sqlOpen = orig }()
	sqlOpen = func(driverName, dsn string) (*sql.DB, error) {
		return db, nil
	}

	cfg := config.MySQLConfig{DSN: "dsn", MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: time.Second}
	conn, err := NewMySQL(cfg)
	if err != nil {
		t.Fatalf("NewMySQL error: %v", err)
	}
	_ = conn.Close()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestNewMySQLOpenError(t *testing.T) {
	orig := sqlOpen
	defer func() { sqlOpen = orig }()
	sqlOpen = func(driverName, dsn string) (*sql.DB, error) {
		return nil, errors.New("open error")
	}

	cfg := config.MySQLConfig{DSN: "dsn"}
	if _, err := NewMySQL(cfg); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewMySQLPingError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()
	mock.ExpectPing().WillReturnError(context.DeadlineExceeded)

	orig := sqlOpen
	defer func() { sqlOpen = orig }()
	sqlOpen = func(driverName, dsn string) (*sql.DB, error) {
		return db, nil
	}

	cfg := config.MySQLConfig{DSN: "dsn"}
	if _, err := NewMySQL(cfg); err == nil {
		t.Fatalf("expected error")
	}
}
