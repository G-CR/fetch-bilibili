package db

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRunMySQLMigrationsSuccess(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origGooseSetBaseFS := gooseSetBaseFS
	origGooseSetDialect := gooseSetDialect
	origGooseUp := gooseUp
	defer func() {
		gooseSetBaseFS = origGooseSetBaseFS
		gooseSetDialect = origGooseSetDialect
		gooseUp = origGooseUp
	}()

	var calls []string
	gooseSetBaseFS = func(base fs.FS) {
		calls = append(calls, "set-fs")
	}
	gooseSetDialect = func(dialect string) error {
		calls = append(calls, "set-dialect:"+dialect)
		return nil
	}
	gooseUp = func(ctx context.Context, db *sql.DB, dir string) error {
		calls = append(calls, "up:"+dir)
		return nil
	}

	if err := RunMySQLMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMySQLMigrations error: %v", err)
	}

	want := []string{"set-fs", "set-dialect:mysql", "up:."}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls mismatch, want %v got %v", want, calls)
	}
}

func TestRunMySQLMigrationsDialectError(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origGooseSetBaseFS := gooseSetBaseFS
	origGooseSetDialect := gooseSetDialect
	origGooseUp := gooseUp
	defer func() {
		gooseSetBaseFS = origGooseSetBaseFS
		gooseSetDialect = origGooseSetDialect
		gooseUp = origGooseUp
	}()

	gooseSetBaseFS = func(base fs.FS) {}
	gooseSetDialect = func(dialect string) error {
		return errors.New("boom")
	}
	gooseUp = func(ctx context.Context, db *sql.DB, dir string) error {
		t.Fatalf("gooseUp should not be called")
		return nil
	}

	err = RunMySQLMigrations(context.Background(), db)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "设置数据库迁移方言失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMySQLMigrationsUpError(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origGooseSetBaseFS := gooseSetBaseFS
	origGooseSetDialect := gooseSetDialect
	origGooseUp := gooseUp
	defer func() {
		gooseSetBaseFS = origGooseSetBaseFS
		gooseSetDialect = origGooseSetDialect
		gooseUp = origGooseUp
	}()

	gooseSetBaseFS = func(base fs.FS) {}
	gooseSetDialect = func(dialect string) error {
		return nil
	}
	gooseUp = func(ctx context.Context, db *sql.DB, dir string) error {
		return errors.New("boom")
	}

	err = RunMySQLMigrations(context.Background(), db)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "执行数据库迁移失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMySQLMigrationsNilDB(t *testing.T) {
	err := RunMySQLMigrations(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "数据库连接不能为空") {
		t.Fatalf("unexpected error: %v", err)
	}
}
