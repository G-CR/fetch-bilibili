package mysqlrepo

import (
	"context"
	"regexp"
	"testing"

	"fetch-bilibili/internal/jobs"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestJobUpdateStatusQueuedClearsTimestamps(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE jobs")).
		WithArgs(
			jobs.StatusQueued,
			"启动恢复后重新入队",
			jobs.StatusQueued,
			jobs.StatusQueued,
			jobs.StatusQueued,
			1,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Jobs().UpdateStatus(context.Background(), 1, jobs.StatusQueued, "启动恢复后重新入队"); err != nil {
		t.Fatalf("update status error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
