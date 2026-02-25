package mysqlrepo

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"fetch-bilibili/internal/repo"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestVideoFileCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO video_files")).
		WithArgs(int64(1), "/data/bilibili/v1.mp4", int64(10), "", "video").
		WillReturnResult(sqlmock.NewResult(2, 1))

	id, err := repoImpl.VideoFiles().Create(context.Background(), repo.VideoFile{
		VideoID:   1,
		Path:      "/data/bilibili/v1.mp4",
		SizeBytes: 10,
		Type:      "video",
	})
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

func TestVideoFileCreateDefaultType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO video_files")).
		WithArgs(int64(2), "/data/bilibili/v2.mp4", int64(20), "", "video").
		WillReturnResult(sqlmock.NewResult(3, 1))

	if _, err := repoImpl.VideoFiles().Create(context.Background(), repo.VideoFile{
		VideoID:   2,
		Path:      "/data/bilibili/v2.mp4",
		SizeBytes: 20,
	}); err != nil {
		t.Fatalf("create error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoFileCreateError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO video_files")).
		WithArgs(int64(3), "/data/bilibili/v3.mp4", int64(30), "", "video").
		WillReturnError(sql.ErrConnDone)

	if _, err := repoImpl.VideoFiles().Create(context.Background(), repo.VideoFile{
		VideoID:   3,
		Path:      "/data/bilibili/v3.mp4",
		SizeBytes: 30,
	}); err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
