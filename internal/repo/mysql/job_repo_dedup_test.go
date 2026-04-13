package mysqlrepo

import (
	"context"
	"regexp"
	"testing"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/repo"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestJobEnqueueSkipsDuplicateType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM jobs WHERE type = ? AND status IN (?,?)")).
		WithArgs(jobs.TypeFetch, jobs.StatusQueued, jobs.StatusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	if _, err := repoImpl.Jobs().Enqueue(context.Background(), repo.Job{Type: jobs.TypeFetch, Status: jobs.StatusQueued}); err == nil {
		t.Fatalf("expected duplicate job error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestJobEnqueueSkipsDuplicateDownloadByVideoID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM jobs WHERE type = ? AND status IN (?,?) AND CAST(JSON_UNQUOTE(JSON_EXTRACT(payload_json, '$.video_id')) AS UNSIGNED) = ?")).
		WithArgs(jobs.TypeDownload, jobs.StatusQueued, jobs.StatusRunning, int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	if _, err := repoImpl.Jobs().Enqueue(context.Background(), repo.Job{
		Type:   jobs.TypeDownload,
		Status: jobs.StatusQueued,
		Payload: map[string]any{
			"video_id": int64(99),
		},
	}); err == nil {
		t.Fatalf("expected duplicate download error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
