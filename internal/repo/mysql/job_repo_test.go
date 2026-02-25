package mysqlrepo

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/repo"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestJobEnqueue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO jobs")).
		WithArgs(jobs.TypeFetch, jobs.StatusQueued, sqlmock.AnyArg(), nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	id, err := repoImpl.Jobs().Enqueue(context.Background(), repo.Job{Type: jobs.TypeFetch, Payload: map[string]any{"a": 1}})
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}
	if id != 1 {
		t.Fatalf("expected id 1")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobFetchQueued(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Minute)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "type", "status", "payload_json", "error_message", "not_before", "started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(1, jobs.TypeFetch, jobs.StatusQueued, []byte(`{"k":"v"}`), nil, nil, nil, nil, created, updated)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, type, status")).
		WithArgs(jobs.StatusQueued, 1).
		WillReturnRows(rows)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE jobs SET status = 'running'")).
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	list, err := repoImpl.Jobs().FetchQueued(context.Background(), 1)
	if err != nil {
		t.Fatalf("fetch queued error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 job")
	}
	if list[0].Status != jobs.StatusRunning {
		t.Fatalf("expected running status")
	}
	if list[0].Payload["k"] != "v" {
		t.Fatalf("expected payload parsed")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobUpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE jobs")).
		WithArgs(jobs.StatusSuccess, nil, jobs.StatusSuccess, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Jobs().UpdateStatus(context.Background(), 1, jobs.StatusSuccess, ""); err != nil {
		t.Fatalf("update status error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobFetchQueuedEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, type, status")).
		WithArgs(jobs.StatusQueued, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "type", "status", "payload_json", "error_message", "not_before", "started_at", "finished_at", "created_at", "updated_at"}))
	mock.ExpectCommit()

	list, err := repoImpl.Jobs().FetchQueued(context.Background(), 1)
	if err != nil {
		t.Fatalf("fetch queued error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobFetchQueuedQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, type, status")).
		WithArgs(jobs.StatusQueued, 1).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	if _, err := repoImpl.Jobs().FetchQueued(context.Background(), 1); err == nil {
		t.Fatalf("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobEnqueueMarshalError(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	job := repo.Job{
		Type:    jobs.TypeFetch,
		Payload: map[string]any{"bad": func() {}},
	}

	if _, err := repoImpl.Jobs().Enqueue(context.Background(), job); err == nil {
		t.Fatalf("expected marshal error")
	}
}

func TestJobFetchQueuedBeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectBegin().WillReturnError(sql.ErrConnDone)

	if _, err := repoImpl.Jobs().FetchQueued(context.Background(), 1); err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobFetchQueuedBadPayload(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Minute)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "type", "status", "payload_json", "error_message", "not_before", "started_at", "finished_at", "created_at", "updated_at",
	}).AddRow(1, jobs.TypeFetch, jobs.StatusQueued, []byte("{bad"), nil, nil, nil, nil, created, updated)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, type, status")).
		WithArgs(jobs.StatusQueued, 1).
		WillReturnRows(rows)
	mock.ExpectRollback()

	if _, err := repoImpl.Jobs().FetchQueued(context.Background(), 1); err == nil {
		t.Fatalf("expected unmarshal error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobUpdateStatusWithErrorMsg(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE jobs")).
		WithArgs(jobs.StatusFailed, "boom", jobs.StatusFailed, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Jobs().UpdateStatus(context.Background(), 1, jobs.StatusFailed, "boom"); err != nil {
		t.Fatalf("update status error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
