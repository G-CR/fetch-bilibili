package mysqlrepo

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"fetch-bilibili/internal/repo"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreatorCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO creators")).
		WithArgs("bilibili", "123", "name", int64(10), "active").
		WillReturnResult(sqlmock.NewResult(2, 1))

	id, err := repoImpl.Creators().Create(context.Background(), repo.Creator{UID: "123", Name: "name", FollowerCount: 10})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if id != 2 {
		t.Fatalf("expected id 2")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorUpsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO creators")).
		WithArgs("bilibili", "123", "", int64(0), "active").
		WillReturnResult(sqlmock.NewResult(3, 1))

	id, err := repoImpl.Creators().Upsert(context.Background(), repo.Creator{UID: "123"})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if id != 3 {
		t.Fatalf("expected id 3")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE creators")).
		WithArgs("name", int64(12), "paused", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Creators().Update(context.Background(), repo.Creator{ID: 1, Name: "name", FollowerCount: 12, Status: "paused"}); err != nil {
		t.Fatalf("update error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorUpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE creators")).
		WithArgs("paused", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Creators().UpdateStatus(context.Background(), 2, "paused"); err != nil {
		t.Fatalf("update status error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorUpdateStatusDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE creators")).
		WithArgs("active", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Creators().UpdateStatus(context.Background(), 3, ""); err != nil {
		t.Fatalf("update status error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorFindByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Hour)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{"id", "platform", "uid", "name", "follower_count", "status", "created_at", "updated_at"}).
		AddRow(1, "bilibili", "123", "name", 5, "active", created, updated)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, uid")).
		WithArgs(int64(1)).
		WillReturnRows(rows)

	c, err := repoImpl.Creators().FindByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("find error: %v", err)
	}
	if c.UID != "123" || c.Name != "name" {
		t.Fatalf("unexpected creator data")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorListActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Hour)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{"id", "platform", "uid", "name", "follower_count", "status", "created_at", "updated_at"}).
		AddRow(1, "bilibili", "123", "name", 5, "active", created, updated).
		AddRow(2, "bilibili", "456", nil, 0, "active", created, updated)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, uid")).
		WithArgs(2).
		WillReturnRows(rows)

	list, err := repoImpl.Creators().ListActive(context.Background(), 2)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 creators")
	}
	if list[1].Name != "" {
		t.Fatalf("expected empty name for null")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorListActiveAfter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Hour)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{"id", "platform", "uid", "name", "follower_count", "status", "created_at", "updated_at"}).
		AddRow(3, "bilibili", "789", nil, 0, "active", created, updated)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, uid")).
		WithArgs(int64(2), 2).
		WillReturnRows(rows)

	list, err := repoImpl.Creators().ListActiveAfter(context.Background(), 2, 2)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list) != 1 || list[0].ID != 3 {
		t.Fatalf("unexpected list result")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorListActiveQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, uid")).
		WithArgs(1).
		WillReturnError(sql.ErrConnDone)

	if _, err := repoImpl.Creators().ListActive(context.Background(), 1); err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorCreateError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO creators")).
		WithArgs("bilibili", "123", "name", int64(10), "active").
		WillReturnError(sql.ErrConnDone)

	if _, err := repoImpl.Creators().Create(context.Background(), repo.Creator{UID: "123", Name: "name", FollowerCount: 10}); err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCreatorListActiveDefaultLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	rows := sqlmock.NewRows([]string{"id", "platform", "uid", "name", "follower_count", "status", "created_at", "updated_at"})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, uid")).
		WithArgs(50).
		WillReturnRows(rows)

	if _, err := repoImpl.Creators().ListActive(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
