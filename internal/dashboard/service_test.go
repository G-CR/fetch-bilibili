package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"

	"github.com/DATA-DOG/go-sqlmock"
)

type stubCreatorRepo struct {
	count int64
	err   error
}

func (s stubCreatorRepo) Upsert(context.Context, repo.Creator) (int64, error) { return 0, nil }
func (s stubCreatorRepo) Create(context.Context, repo.Creator) (int64, error) { return 0, nil }
func (s stubCreatorRepo) Update(context.Context, repo.Creator) error          { return nil }
func (s stubCreatorRepo) UpdateStatus(context.Context, int64, string) error   { return nil }
func (s stubCreatorRepo) DeleteByID(context.Context, int64) (int64, error)    { return 0, nil }
func (s stubCreatorRepo) FindByID(context.Context, int64) (repo.Creator, error) {
	return repo.Creator{}, nil
}
func (s stubCreatorRepo) FindByPlatformUID(context.Context, string, string) (repo.Creator, error) {
	return repo.Creator{}, nil
}
func (s stubCreatorRepo) ListActive(context.Context, int) ([]repo.Creator, error) { return nil, nil }
func (s stubCreatorRepo) ListActiveAfter(context.Context, int64, int) ([]repo.Creator, error) {
	return nil, nil
}
func (s stubCreatorRepo) CountActive(context.Context) (int64, error) {
	return s.count, s.err
}

type stubVideoRepo struct {
	list       []repo.Video
	listErr    error
	find       map[int64]repo.Video
	findErr    error
	count      int64
	countErr   error
	countState string
}

func (s *stubVideoRepo) Upsert(context.Context, repo.Video) (int64, bool, error) {
	return 0, false, nil
}
func (s *stubVideoRepo) UpdateState(context.Context, int64, string) error { return nil }
func (s *stubVideoRepo) FindByID(context.Context, int64) (repo.Video, error) {
	if s.findErr != nil {
		return repo.Video{}, s.findErr
	}
	for id, video := range s.find {
		if id > 0 {
			return video, nil
		}
	}
	return repo.Video{}, nil
}
func (s *stubVideoRepo) ListForCheck(context.Context, int) ([]repo.Video, error) { return nil, nil }
func (s *stubVideoRepo) UpdateCheckStatus(context.Context, int64, string, *time.Time, *time.Time, time.Time) error {
	return nil
}
func (s *stubVideoRepo) ListRecent(context.Context, repo.VideoListFilter) ([]repo.Video, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]repo.Video(nil), s.list...), nil
}
func (s *stubVideoRepo) ListCleanupCandidates(context.Context, repo.CleanupCandidateFilter) ([]repo.CleanupCandidate, error) {
	return nil, nil
}
func (s *stubVideoRepo) CountByState(_ context.Context, state string) (int64, error) {
	s.countState = state
	return s.count, s.countErr
}

type stubJobRepo struct {
	list     []repo.Job
	listErr  error
	count    int64
	countErr error
	statuses []string
}

func (s *stubJobRepo) Enqueue(context.Context, repo.Job) (int64, error)     { return 0, nil }
func (s *stubJobRepo) FetchQueued(context.Context, int) ([]repo.Job, error) { return nil, nil }
func (s *stubJobRepo) UpdateStatus(context.Context, int64, string, string) error {
	return nil
}
func (s *stubJobRepo) ListRecent(context.Context, repo.JobListFilter) ([]repo.Job, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]repo.Job(nil), s.list...), nil
}
func (s *stubJobRepo) CountByStatuses(_ context.Context, statuses []string) (int64, error) {
	s.statuses = append([]string(nil), statuses...)
	return s.count, s.countErr
}

type stubAuthChecker struct {
	info    bilibili.AuthInfo
	err     error
	runtime bilibili.RuntimeStatus
}

func (s stubAuthChecker) CheckAuth(context.Context) (bilibili.AuthInfo, error) {
	return s.info, s.err
}

func (s stubAuthChecker) RuntimeStatus() bilibili.RuntimeStatus {
	return s.runtime
}

type plainAuthChecker struct {
	info bilibili.AuthInfo
	err  error
}

func (s plainAuthChecker) CheckAuth(context.Context) (bilibili.AuthInfo, error) {
	return s.info, s.err
}

func TestServiceSystemStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectPing()

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp/bili"
	cfg.Bilibili.Cookie = "SESSDATA=ok"

	jobsRepo := &stubJobRepo{
		list:  []repo.Job{{ID: 3, Type: "fetch", Status: "success", CreatedAt: time.Now()}},
		count: 2,
	}
	videosRepo := &stubVideoRepo{count: 5}

	service := New(
		db,
		stubCreatorRepo{count: 4},
		videosRepo,
		jobsRepo,
		stubAuthChecker{
			info: bilibili.AuthInfo{IsLogin: true, Mid: 1, Uname: "tester"},
			runtime: bilibili.RuntimeStatus{
				CookieSource:     "cookie_file",
				LastCheckResult:  "valid",
				LastReloadResult: "success",
				LastCheckAt:      time.Now().Add(-time.Minute),
				LastReloadAt:     time.Now().Add(-2 * time.Minute),
			},
		},
		cfg,
	)

	status, err := service.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSystemStatus error: %v", err)
	}
	if status.Health != "online" {
		t.Fatalf("expected online health, got %s", status.Health)
	}
	if status.Overview.ActiveCreators != 4 || status.Overview.PendingJobs != 2 || status.Overview.RareVideos != 5 {
		t.Fatalf("unexpected overview: %+v", status.Overview)
	}
	if status.Cookie.Status != "valid" || !status.Cookie.IsLogin {
		t.Fatalf("unexpected cookie status: %+v", status.Cookie)
	}
	if status.Cookie.Source != "cookie_file" {
		t.Fatalf("unexpected cookie source: %+v", status.Cookie)
	}
	if status.Cookie.LastCheckResult != "valid" || status.Cookie.LastReloadResult != "success" {
		t.Fatalf("unexpected cookie runtime: %+v", status.Cookie)
	}
	if !status.AuthEnabled {
		t.Fatalf("expected auth enabled")
	}
	if len(jobsRepo.statuses) != 2 {
		t.Fatalf("expected statuses captured")
	}
	if videosRepo.countState != "OUT_OF_PRINT" {
		t.Fatalf("expected out_of_print count query")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestServiceSystemStatusRiskSnapshot(t *testing.T) {
	cfg := config.Default()
	cfg.Bilibili.Cookie = "SESSDATA=ok"

	service := New(
		nil,
		stubCreatorRepo{},
		&stubVideoRepo{},
		&stubJobRepo{},
		stubAuthChecker{
			info: bilibili.AuthInfo{IsLogin: true, Mid: 1, Uname: "tester"},
			runtime: bilibili.RuntimeStatus{
				CookieSource:    "config",
				LastCheckResult: "valid",
				LastCheckAt:     time.Now().Add(-time.Minute),
				RiskUntil:       time.Now().Add(30 * time.Second),
				RiskBackoff:     30 * time.Second,
				LastRiskAt:      time.Now().Add(-5 * time.Second),
				LastRiskReason:  "/x/web-interface/nav 返回风控码 -412",
			},
		},
		cfg,
	)

	status, err := service.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSystemStatus error: %v", err)
	}
	if status.Risk.Level != "高" || !status.Risk.Active {
		t.Fatalf("unexpected risk status: %+v", status.Risk)
	}
	if status.Risk.BackoffSeconds <= 0 {
		t.Fatalf("expected positive backoff seconds: %+v", status.Risk)
	}
	if status.Risk.LastReason == "" {
		t.Fatalf("expected last reason")
	}
}

func TestServiceStorageStats(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "store", "bilibili"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "store", "bilibili", "a.mp4"), []byte("12345"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "store", "bilibili", "b.mp4"), []byte("12"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg := config.Default()
	cfg.Storage.RootDir = root
	cfg.Storage.MaxBytes = 100
	cfg.Storage.SafeBytes = 80

	service := New(nil, stubCreatorRepo{}, &stubVideoRepo{count: 2}, &stubJobRepo{}, nil, cfg)

	stats, err := service.GetStorageStats(context.Background())
	if err != nil {
		t.Fatalf("GetStorageStats error: %v", err)
	}
	if stats.UsedBytes != 7 {
		t.Fatalf("expected used bytes 7, got %d", stats.UsedBytes)
	}
	if stats.FileCount != 2 {
		t.Fatalf("expected 2 files, got %d", stats.FileCount)
	}
	if stats.HottestBucket != "bilibili" {
		t.Fatalf("expected hottest bucket bilibili, got %s", stats.HottestBucket)
	}
	if stats.RareVideos != 2 {
		t.Fatalf("expected rare videos 2, got %d", stats.RareVideos)
	}
}

func TestServiceListJobsAndVideos(t *testing.T) {
	jobsRepo := &stubJobRepo{
		list: []repo.Job{{ID: 1, Type: "fetch", Status: "success"}},
	}
	videosRepo := &stubVideoRepo{
		list: []repo.Video{{ID: 1, VideoID: "BV1", State: "DOWNLOADED"}},
	}

	service := New(nil, stubCreatorRepo{}, videosRepo, jobsRepo, nil, config.Default())

	jobsList, err := service.ListJobs(context.Background(), repo.JobListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListJobs error: %v", err)
	}
	if len(jobsList) != 1 || jobsList[0].ID != 1 {
		t.Fatalf("unexpected jobs list")
	}

	videosList, err := service.ListVideos(context.Background(), repo.VideoListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListVideos error: %v", err)
	}
	if len(videosList) != 1 || videosList[0].VideoID != "BV1" {
		t.Fatalf("unexpected videos list")
	}
}

func TestServiceListJobsAndVideosWithoutRepos(t *testing.T) {
	service := New(nil, stubCreatorRepo{}, nil, nil, nil, config.Default())

	jobsList, err := service.ListJobs(context.Background(), repo.JobListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListJobs error: %v", err)
	}
	if jobsList != nil {
		t.Fatalf("expected nil jobs list, got %+v", jobsList)
	}

	videosList, err := service.ListVideos(context.Background(), repo.VideoListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListVideos error: %v", err)
	}
	if videosList != nil {
		t.Fatalf("expected nil videos list, got %+v", videosList)
	}
}

func TestServiceSystemStatusDBError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	mock.ExpectPing().WillReturnError(sql.ErrConnDone)

	cfg := config.Default()
	cfg.Bilibili.Cookie = "SESSDATA=bad"

	service := New(db, stubCreatorRepo{}, &stubVideoRepo{}, &stubJobRepo{}, stubAuthChecker{err: errors.New("boom")}, cfg)

	status, err := service.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSystemStatus error: %v", err)
	}
	if status.Health != "degraded" {
		t.Fatalf("expected degraded health")
	}
	if status.Cookie.Status != "error" {
		t.Fatalf("expected cookie error status")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestServiceGetVideo(t *testing.T) {
	videosRepo := &stubVideoRepo{
		find: map[int64]repo.Video{
			11: {ID: 11, VideoID: "BV11", Title: "demo"},
		},
	}
	service := New(nil, stubCreatorRepo{}, videosRepo, &stubJobRepo{}, nil, config.Default())

	video, err := service.GetVideo(context.Background(), 11)
	if err != nil {
		t.Fatalf("GetVideo error: %v", err)
	}
	if video.ID != 11 || video.VideoID != "BV11" {
		t.Fatalf("unexpected video: %+v", video)
	}
}

func TestServiceGetVideoWithoutRepo(t *testing.T) {
	service := New(nil, stubCreatorRepo{}, nil, &stubJobRepo{}, nil, config.Default())

	if _, err := service.GetVideo(context.Background(), 11); !errors.Is(err, repo.ErrNotImplemented) {
		t.Fatalf("expected not implemented, got %v", err)
	}
}

func TestServiceCheckCookieScenarios(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		status := New(nil, stubCreatorRepo{}, nil, nil, nil, config.Default()).checkCookie(context.Background(), bilibili.RuntimeStatus{})
		if status.Status != "not_configured" || status.Configured {
			t.Fatalf("unexpected status: %+v", status)
		}
	})

	t.Run("configured but no auth client", func(t *testing.T) {
		cfg := config.Default()
		cfg.Bilibili.Cookie = "SESSDATA=ok"

		status := New(nil, stubCreatorRepo{}, nil, nil, nil, cfg).checkCookie(context.Background(), bilibili.RuntimeStatus{})
		if status.Status != "unknown" || !status.Configured {
			t.Fatalf("unexpected status: %+v", status)
		}
	})

	t.Run("invalid cookie", func(t *testing.T) {
		cfg := config.Default()
		cfg.Bilibili.Cookie = "SESSDATA=bad"

		status := New(nil, stubCreatorRepo{}, nil, nil, plainAuthChecker{}, cfg).checkCookie(context.Background(), bilibili.RuntimeStatus{})
		if status.Status != "invalid" || status.IsLogin {
			t.Fatalf("unexpected status: %+v", status)
		}
	})

	t.Run("auth error", func(t *testing.T) {
		cfg := config.Default()
		cfg.Bilibili.Cookie = "SESSDATA=bad"

		status := New(nil, stubCreatorRepo{}, nil, nil, plainAuthChecker{err: errors.New("boom")}, cfg).checkCookie(context.Background(), bilibili.RuntimeStatus{})
		if status.Status != "error" || status.Error != "boom" {
			t.Fatalf("unexpected status: %+v", status)
		}
	})
}

func TestServiceRuntimeStatusWithoutProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Bilibili.Cookie = "SESSDATA=ok"

	service := New(nil, stubCreatorRepo{}, nil, nil, plainAuthChecker{}, cfg)
	status := service.runtimeStatus()
	if status != (bilibili.RuntimeStatus{}) {
		t.Fatalf("expected zero runtime status, got %+v", status)
	}
}

func TestServiceSystemStatusInvalidCookieRaisesMediumRisk(t *testing.T) {
	cfg := config.Default()
	cfg.Bilibili.Cookie = "SESSDATA=bad"

	service := New(nil, stubCreatorRepo{}, &stubVideoRepo{}, &stubJobRepo{}, plainAuthChecker{}, cfg)
	status, err := service.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSystemStatus error: %v", err)
	}
	if status.Cookie.Status != "invalid" {
		t.Fatalf("expected invalid cookie, got %+v", status.Cookie)
	}
	if status.Risk.Level != "中" || status.RiskLevel != "中" {
		t.Fatalf("expected medium risk, got %+v / %s", status.Risk, status.RiskLevel)
	}
}

func TestServiceStorageStatsVideoCountError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("123"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg := config.Default()
	cfg.Storage.RootDir = root

	service := New(nil, stubCreatorRepo{}, &stubVideoRepo{countErr: errors.New("boom")}, &stubJobRepo{}, nil, cfg)
	if _, err := service.GetStorageStats(context.Background()); err == nil {
		t.Fatalf("expected storage stats error")
	}
}

func TestScanStorageEdgeCases(t *testing.T) {
	t.Run("empty root", func(t *testing.T) {
		used, files, bucket, err := scanStorage("")
		if err != nil {
			t.Fatalf("scanStorage error: %v", err)
		}
		if used != 0 || files != 0 || bucket != "-" {
			t.Fatalf("unexpected result: used=%d files=%d bucket=%q", used, files, bucket)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "missing")
		used, files, bucket, err := scanStorage(root)
		if err != nil {
			t.Fatalf("scanStorage error: %v", err)
		}
		if used != 0 || files != 0 || bucket != "-" {
			t.Fatalf("unexpected result: used=%d files=%d bucket=%q", used, files, bucket)
		}
	})

	t.Run("root is file", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "single.file")
		if err := os.WriteFile(root, []byte("123"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		used, files, bucket, err := scanStorage(root)
		if err != nil {
			t.Fatalf("scanStorage error: %v", err)
		}
		if used != 0 || files != 0 || bucket != "-" {
			t.Fatalf("unexpected result: used=%d files=%d bucket=%q", used, files, bucket)
		}
	})
}

func TestPercentBoundaries(t *testing.T) {
	cases := []struct {
		name string
		used int64
		max  int64
		want int
	}{
		{name: "zero max", used: 10, max: 0, want: 0},
		{name: "negative result", used: -1, max: 10, want: 0},
		{name: "overflow capped", used: 30, max: 10, want: 100},
		{name: "normal", used: 3, max: 10, want: 30},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := percent(tc.used, tc.max); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}
