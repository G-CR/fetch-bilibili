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

func TestVideoUpsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO videos")).
		WithArgs("bilibili", "v1", int64(1), "title", "desc", sqlmock.AnyArg(), 10, "cover", int64(100), int64(5), "NEW").
		WillReturnResult(sqlmock.NewResult(3, 1))

	id, created, err := repoImpl.Videos().Upsert(context.Background(), repo.Video{
		Platform:      "bilibili",
		VideoID:       "v1",
		CreatorID:     1,
		Title:         "title",
		Description:   "desc",
		PublishTime:   time.Now(),
		Duration:      10,
		CoverURL:      "cover",
		ViewCount:     100,
		FavoriteCount: 5,
		State:         "NEW",
	})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if id != 3 {
		t.Fatalf("expected id 3")
	}
	if !created {
		t.Fatalf("expected created")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoUpdateState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE videos")).
		WithArgs("STABLE", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Videos().UpdateState(context.Background(), 1, "STABLE"); err != nil {
		t.Fatalf("update state error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoUpdateStateError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE videos")).
		WithArgs("STABLE", int64(2)).
		WillReturnError(sql.ErrConnDone)

	if err := repoImpl.Videos().UpdateState(context.Background(), 2, "STABLE"); err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoFindByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Hour)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "platform", "video_id", "creator_id", "title", "description", "publish_time", "duration", "cover_url",
		"view_count", "favorite_count", "state", "out_of_print_at", "stable_at", "last_check_at", "created_at", "updated_at",
	}).AddRow(1, "bilibili", "v1", 2, "t1", nil, created, 10, "cover", 3, 4, "NEW", nil, nil, nil, created, updated)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, video_id")).
		WithArgs(int64(1)).
		WillReturnRows(rows)

	v, err := repoImpl.Videos().FindByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("find error: %v", err)
	}
	if v.VideoID != "v1" || v.CreatorID != 2 {
		t.Fatalf("unexpected video data")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoFindByIDError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, video_id")).
		WithArgs(int64(2)).
		WillReturnError(sql.ErrConnDone)

	if _, err := repoImpl.Videos().FindByID(context.Background(), 2); err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoListForCheck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Hour)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "platform", "video_id", "creator_id", "title", "description", "publish_time", "duration", "cover_url",
		"view_count", "favorite_count", "state", "out_of_print_at", "stable_at", "last_check_at", "created_at", "updated_at",
	}).AddRow(1, "bilibili", "v1", 1, "t1", "desc", created, 10, "cover", 1, 2, "DOWNLOADED", nil, nil, nil, created, updated).
		AddRow(2, "bilibili", "v2", 1, "t2", nil, nil, 20, "cover2", 3, 4, "STABLE", created, created, created, created, updated)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, video_id")).
		WithArgs(2).
		WillReturnRows(rows)

	list, err := repoImpl.Videos().ListForCheck(context.Background(), 2)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 videos")
	}
	if list[0].Description == "" {
		t.Fatalf("expected description")
	}
	if !list[1].PublishTime.IsZero() {
		t.Fatalf("expected zero publish_time for null value")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoUpdateCheckStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	now := time.Now()
	out := now.Add(-time.Hour)
	stable := now.Add(-2 * time.Hour)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE videos")).
		WithArgs("OUT_OF_PRINT", sqlmock.AnyArg(), sqlmock.AnyArg(), now, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Videos().UpdateCheckStatus(context.Background(), 1, "OUT_OF_PRINT", &out, &stable, now); err != nil {
		t.Fatalf("update check status error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoListForCheckQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, video_id")).
		WithArgs(1).
		WillReturnError(sql.ErrConnDone)

	if _, err := repoImpl.Videos().ListForCheck(context.Background(), 1); err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoListCleanupCandidates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	rows := sqlmock.NewRows([]string{
		"video_id", "source_video_id", "platform", "title", "state", "creator_id", "creator_name",
		"follower_count", "view_count", "favorite_count", "file_id", "file_path", "file_size_bytes",
	}).AddRow(
		int64(1), "BV1xx411c7mD", "bilibili", "稀有视频", "DOWNLOADED", int64(2), "测试博主",
		int64(1000), int64(2000), int64(30), int64(9), "/data/bilibili/BV1xx411c7mD.mp4", int64(123),
	)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT v.id AS video_id")).
		WithArgs(10).
		WillReturnRows(rows)

	list, err := repoImpl.Videos().ListCleanupCandidates(context.Background(), repo.CleanupCandidateFilter{
		Limit:             10,
		IncludeOutOfPrint: false,
	})
	if err != nil {
		t.Fatalf("ListCleanupCandidates error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 candidate")
	}
	if list[0].SourceVideoID != "BV1xx411c7mD" || list[0].FileID != 9 {
		t.Fatalf("unexpected candidate: %+v", list[0])
	}
}

func TestVideoListCleanupCandidatesIncludeOutOfPrint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT v.id AS video_id")).
		WithArgs(5).
		WillReturnRows(sqlmock.NewRows([]string{
			"video_id", "source_video_id", "platform", "title", "state", "creator_id", "creator_name",
			"follower_count", "view_count", "favorite_count", "file_id", "file_path", "file_size_bytes",
		}))

	if _, err := repoImpl.Videos().ListCleanupCandidates(context.Background(), repo.CleanupCandidateFilter{
		Limit:             5,
		IncludeOutOfPrint: true,
	}); err != nil {
		t.Fatalf("ListCleanupCandidates error: %v", err)
	}
}

func TestVideoUpsertDefaults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO videos")).
		WithArgs("bilibili", "v2", int64(1), "title", "", nil, 0, "", int64(0), int64(0), "NEW").
		WillReturnResult(sqlmock.NewResult(4, 1))

	id, created, err := repoImpl.Videos().Upsert(context.Background(), repo.Video{
		VideoID:   "v2",
		CreatorID: 1,
		Title:     "title",
	})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if id != 4 {
		t.Fatalf("expected id 4")
	}
	if !created {
		t.Fatalf("expected created")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoUpsertExisting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO videos")).
		WithArgs("bilibili", "v3", int64(1), "title", "", nil, 0, "", int64(0), int64(0), "NEW").
		WillReturnResult(sqlmock.NewResult(5, 2))

	_, created, err := repoImpl.Videos().Upsert(context.Background(), repo.Video{
		VideoID:   "v3",
		CreatorID: 1,
		Title:     "title",
	})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if created {
		t.Fatalf("expected created=false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoUpdateCheckStatusNilTimes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	now := time.Now()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE videos")).
		WithArgs("DOWNLOADED", nil, nil, now, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Videos().UpdateCheckStatus(context.Background(), 1, "DOWNLOADED", nil, nil, now); err != nil {
		t.Fatalf("update check status error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoUpdateCheckStatusWithTimes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	now := time.Now()
	out := now.Add(-time.Hour)
	stable := now.Add(-2 * time.Hour)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE videos")).
		WithArgs("OUT_OF_PRINT", out, stable, now, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repoImpl.Videos().UpdateCheckStatus(context.Background(), 2, "OUT_OF_PRINT", &out, &stable, now); err != nil {
		t.Fatalf("update check status error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoListRecent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	created := time.Now().Add(-time.Hour)
	updated := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "platform", "video_id", "creator_id", "title", "description", "publish_time", "duration", "cover_url",
		"view_count", "favorite_count", "state", "out_of_print_at", "stable_at", "last_check_at", "created_at", "updated_at",
	}).AddRow(9, "bilibili", "BV1", 1, "t1", "desc", created, 10, "cover", 1, 2, "OUT_OF_PRINT", created, nil, created, created, updated)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, video_id, creator_id, title, description, publish_time, duration, cover_url, view_count, favorite_count, state, out_of_print_at, stable_at, last_check_at, created_at, updated_at FROM videos")).
		WithArgs(int64(1), "OUT_OF_PRINT", 5).
		WillReturnRows(rows)

	list, err := repoImpl.Videos().ListRecent(context.Background(), repo.VideoListFilter{
		Limit:     5,
		CreatorID: 1,
		State:     "OUT_OF_PRINT",
	})
	if err != nil {
		t.Fatalf("list recent error: %v", err)
	}
	if len(list) != 1 || list[0].VideoID != "BV1" {
		t.Fatalf("unexpected list result")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVideoCountByState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	rows := sqlmock.NewRows([]string{"count"}).AddRow(7)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM videos WHERE state = ?")).
		WithArgs("OUT_OF_PRINT").
		WillReturnRows(rows)

	count, err := repoImpl.Videos().CountByState(context.Background(), "OUT_OF_PRINT")
	if err != nil {
		t.Fatalf("count by state error: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected count 7, got %d", count)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
