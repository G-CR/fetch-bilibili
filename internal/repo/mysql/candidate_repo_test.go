package mysqlrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"fetch-bilibili/internal/repo"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCandidateRepoUpsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)
	discoveredAt := time.Now().Add(-time.Hour).UTC()
	scoredAt := time.Now().UTC()
	rows := sqlmock.NewRows([]string{
		"id", "platform", "uid", "name", "avatar_url", "profile_url", "follower_count", "status", "score", "score_version",
		"last_discovered_at", "last_scored_at", "approved_at", "ignored_at", "blocked_at", "created_at", "updated_at",
	}).AddRow(
		7, "bilibili", "123", "候选博主", "https://img.test/avatar.jpg", "https://space.bilibili.com/123", 88, "reviewing", 72, "v1",
		discoveredAt, scoredAt, nil, nil, nil, scoredAt, scoredAt,
	)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO candidate_creators")).
		WithArgs(
			"bilibili",
			"123",
			"候选博主",
			"https://img.test/avatar.jpg",
			"https://space.bilibili.com/123",
			int64(88),
			"reviewing",
			72,
			"v1",
			discoveredAt,
			scoredAt,
			nil,
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, uid, name, avatar_url, profile_url, follower_count, status, score, score_version, last_discovered_at, last_scored_at, approved_at, ignored_at, blocked_at, created_at, updated_at FROM candidate_creators WHERE id = ?")).
		WithArgs(int64(7)).
		WillReturnRows(rows)

	got, err := repoImpl.Candidates().Upsert(context.Background(), repo.CandidateCreator{
		UID:              "123",
		Name:             "候选博主",
		AvatarURL:        "https://img.test/avatar.jpg",
		ProfileURL:       "https://space.bilibili.com/123",
		FollowerCount:    88,
		Status:           "reviewing",
		Score:            72,
		ScoreVersion:     "v1",
		LastDiscoveredAt: discoveredAt,
		LastScoredAt:     scoredAt,
	})
	if err != nil {
		t.Fatalf("upsert error: %v", err)
	}
	if got.ID != 7 || got.UID != "123" || got.Score != 72 {
		t.Fatalf("unexpected candidate: %+v", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCandidateRepoReplaceSourcesDeduplicatesByTypeAndValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)
	detailA, _ := json.Marshal(map[string]any{"video_title": "片段 A", "video_id": "BV1A"})
	detailB, _ := json.Marshal(map[string]any{"video_title": "片段 B", "video_id": "BV1B"})

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM candidate_creator_sources WHERE candidate_creator_id = ?")).
		WithArgs(int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO candidate_creator_sources")).
		WithArgs(int64(9), "keyword", "补档", "关键词：补档", 12, detailB).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO candidate_creator_sources")).
		WithArgs(int64(9), "related_creator", "10086", "来自博主 10086", 8, detailA).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err = repoImpl.Candidates().ReplaceSources(context.Background(), 9, []repo.CandidateCreatorSource{
		{CandidateCreatorID: 9, SourceType: "keyword", SourceValue: "补档", SourceLabel: "关键词：补档", Weight: 10, DetailJSON: detailA},
		{CandidateCreatorID: 9, SourceType: "keyword", SourceValue: "补档", SourceLabel: "关键词：补档", Weight: 12, DetailJSON: detailB},
		{CandidateCreatorID: 9, SourceType: "related_creator", SourceValue: "10086", SourceLabel: "来自博主 10086", Weight: 8, DetailJSON: detailA},
	})
	if err != nil {
		t.Fatalf("replace sources error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCandidateRepoReplaceScoreDetails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)
	detail, _ := json.Marshal(map[string]any{"keyword": "补档"})

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM candidate_creator_score_details WHERE candidate_creator_id = ?")).
		WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO candidate_creator_score_details")).
		WithArgs(int64(11), "keyword_risk", "命中高风险关键词", 20, detail).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO candidate_creator_score_details")).
		WithArgs(int64(11), "activity_30d", "最近 30 天更新活跃", 10, nil).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err = repoImpl.Candidates().ReplaceScoreDetails(context.Background(), 11, []repo.CandidateCreatorScoreDetail{
		{CandidateCreatorID: 11, FactorKey: "keyword_risk", FactorLabel: "命中高风险关键词", ScoreDelta: 20, DetailJSON: detail},
		{CandidateCreatorID: 11, FactorKey: "activity_30d", FactorLabel: "最近 30 天更新活跃", ScoreDelta: 10},
	})
	if err != nil {
		t.Fatalf("replace score details error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCandidateRepoList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)
	now := time.Now().UTC()
	keyword := "%剪辑%"
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	listRows := sqlmock.NewRows([]string{
		"id", "platform", "uid", "name", "avatar_url", "profile_url", "follower_count", "status", "score", "score_version",
		"last_discovered_at", "last_scored_at", "approved_at", "ignored_at", "blocked_at", "created_at", "updated_at",
	}).AddRow(
		3, "bilibili", "9988", "影视剪辑号", nil, nil, 1234, "reviewing", 81, "v1",
		now, now, nil, nil, nil, now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM candidate_creators c")).
		WithArgs("reviewing", 60, keyword, keyword, keyword).
		WillReturnRows(countRows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT c.id, c.platform, c.uid, c.name, c.avatar_url, c.profile_url, c.follower_count, c.status, c.score, c.score_version, c.last_discovered_at, c.last_scored_at, c.approved_at, c.ignored_at, c.blocked_at, c.created_at, c.updated_at FROM candidate_creators c")).
		WithArgs("reviewing", 60, keyword, keyword, keyword, 10, 10).
		WillReturnRows(listRows)

	items, total, err := repoImpl.Candidates().List(context.Background(), repo.CandidateListFilter{
		Status:   "reviewing",
		MinScore: 60,
		Keyword:  "剪辑",
		Page:     2,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].UID != "9988" {
		t.Fatalf("unexpected list result: total=%d items=%+v", total, items)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCandidateRepoFindByPlatformUIDMissingRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, platform, uid, name, avatar_url, profile_url, follower_count, status, score, score_version, last_discovered_at, last_scored_at, approved_at, ignored_at, blocked_at, created_at, updated_at FROM candidate_creators WHERE platform = ? AND uid = ?")).
		WithArgs("bilibili", "404").
		WillReturnError(sql.ErrNoRows)

	_, err = repoImpl.Candidates().FindByPlatformUID(context.Background(), "bilibili", "404")
	if err != repo.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCandidateRepoUpdateReviewStatusRejectsIllegalTransition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)
	now := time.Now().UTC()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE candidate_creators")).
		WithArgs("ignored", now, int64(5), "reviewing").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM candidate_creators WHERE id = ?")).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("approved"))

	err = repoImpl.Candidates().UpdateReviewStatus(context.Background(), 5, []string{"reviewing"}, "ignored", now)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "非法状态流转") {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCandidateRepoUpdateReviewStatusMissingRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repoImpl := New(db)
	now := time.Now().UTC()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE candidate_creators")).
		WithArgs("blocked", now, int64(8), "reviewing").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM candidate_creators WHERE id = ?")).
		WithArgs(int64(8)).
		WillReturnError(sql.ErrNoRows)

	err = repoImpl.Candidates().UpdateReviewStatus(context.Background(), 8, []string{"reviewing"}, "blocked", now)
	if err != repo.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
