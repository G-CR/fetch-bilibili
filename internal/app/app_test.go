package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	httpapi "fetch-bilibili/internal/api/http"
	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/dashboard"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
	"fetch-bilibili/internal/scheduler"
	"fetch-bilibili/internal/worker"

	"github.com/DATA-DOG/go-sqlmock"
)

type stubScheduler struct {
	started int32
}

func (s *stubScheduler) Start(ctx context.Context) {
	atomic.StoreInt32(&s.started, 1)
}

type stubWorker struct {
	started int32
	waited  int32
}

func (w *stubWorker) Start(ctx context.Context) {
	atomic.StoreInt32(&w.started, 1)
}

func (w *stubWorker) Wait() {
	atomic.StoreInt32(&w.waited, 1)
}

type stubCreatorSyncer struct {
	started int32
}

func (s *stubCreatorSyncer) Start(ctx context.Context) {
	atomic.StoreInt32(&s.started, 1)
}

type stubAuthWatcher struct {
	started int32
}

func (s *stubAuthWatcher) Start(ctx context.Context) {
	atomic.StoreInt32(&s.started, 1)
}

type stubHTTPDashboardService struct{}

func (s *stubHTTPDashboardService) ListJobs(context.Context, repo.JobListFilter) ([]repo.Job, error) {
	return nil, nil
}

func (s *stubHTTPDashboardService) ListVideos(context.Context, repo.VideoListFilter) ([]repo.Video, error) {
	return nil, nil
}

func (s *stubHTTPDashboardService) GetVideo(context.Context, int64) (repo.Video, error) {
	return repo.Video{}, nil
}

func (s *stubHTTPDashboardService) GetSystemStatus(context.Context) (dashboard.SystemStatus, error) {
	return dashboard.SystemStatus{}, nil
}

func (s *stubHTTPDashboardService) GetStorageStats(context.Context) (dashboard.StorageStats, error) {
	return dashboard.StorageStats{}, nil
}

func TestNewSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origWorker := newWorker
	origRouter := newRouter
	origDashboardService := newDashboardService
	origRunMySQLMigrations := runMySQLMigrations
	origRunStartupRecovery := runStartupRecovery
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
		newDashboardService = origDashboardService
		runMySQLMigrations = origRunMySQLMigrations
		runStartupRecovery = origRunStartupRecovery
	}()

	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return &stubScheduler{}, nil
	}
	newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
		return &stubWorker{}
	}
	dashboardSvc := &stubHTTPDashboardService{}
	var dashboardCreated bool
	newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
		dashboardCreated = true
		return dashboardSvc
	}
	runMySQLMigrations = func(ctx context.Context, db *sql.DB) error {
		return nil
	}
	newRouter = func(_ httpapi.CreatorService, _ httpapi.JobService, gotDashboard httpapi.DashboardService, _ httpapi.ConfigService) http.Handler {
		if gotDashboard != dashboardSvc {
			t.Fatalf("expected dashboard service injected")
		}
		return http.NewServeMux()
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "user:pass@tcp(localhost:3306)/db"
	cfg.Server.Addr = "127.0.0.1:0"

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if app == nil {
		t.Fatalf("expected app")
	}
	if !dashboardCreated {
		t.Fatalf("expected dashboard service created")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestNewSchedulerError(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origDashboardService := newDashboardService
	origRunMySQLMigrations := runMySQLMigrations
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newDashboardService = origDashboardService
		runMySQLMigrations = origRunMySQLMigrations
	}()

	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return nil, errors.New("scheduler error")
	}
	newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
		return &stubHTTPDashboardService{}
	}
	runMySQLMigrations = func(ctx context.Context, db *sql.DB) error {
		return nil
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "dsn"
	cfg.Server.Addr = "127.0.0.1:0"

	if _, err := New(cfg); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewMySQLError(t *testing.T) {
	origMySQL := newMySQL
	defer func() { newMySQL = origMySQL }()

	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return nil, errors.New("mysql error")
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "dsn"

	if _, err := New(cfg); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunCanceled(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origWorker := newWorker
	origRouter := newRouter
	origDashboardService := newDashboardService
	origRunMySQLMigrations := runMySQLMigrations
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
		newDashboardService = origDashboardService
		runMySQLMigrations = origRunMySQLMigrations
	}()

	schedulerStub := &stubScheduler{}
	workerStub := &stubWorker{}

	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return schedulerStub, nil
	}
	newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
		return workerStub
	}
	newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
		return &stubHTTPDashboardService{}
	}
	runMySQLMigrations = func(ctx context.Context, db *sql.DB) error {
		return nil
	}
	runStartupRecovery = func(context.Context, *App) error {
		return nil
	}
	newRouter = func(httpapi.CreatorService, httpapi.JobService, httpapi.DashboardService, httpapi.ConfigService) http.Handler {
		return http.NewServeMux()
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "dsn"
	cfg.Server.Addr = "127.0.0.1:0"

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx)
	}()

	deadline := time.Now().Add(50 * time.Millisecond)
	for (atomic.LoadInt32(&schedulerStub.started) == 0 || atomic.LoadInt32(&workerStub.started) == 0) && time.Now().Before(deadline) {
		time.Sleep(1 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if atomic.LoadInt32(&schedulerStub.started) != 1 {
		t.Fatalf("scheduler not started")
	}
	if atomic.LoadInt32(&workerStub.started) != 1 {
		t.Fatalf("worker not started")
	}
	if atomic.LoadInt32(&workerStub.waited) != 1 {
		t.Fatalf("worker wait not called")
	}
}

func TestRunServerError(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	app := &App{
		db:     db,
		server: &http.Server{Addr: "bad:addr"},
	}

	if err := app.Run(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunNoSchedulerOrWorker(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	app := &App{
		db:     db,
		server: &http.Server{Addr: "127.0.0.1:0"},
	}

	origRunStartupRecovery := runStartupRecovery
	defer func() {
		runStartupRecovery = origRunStartupRecovery
	}()
	runStartupRecovery = func(context.Context, *App) error {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := app.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRunStartsCreatorSyncer(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origWorker := newWorker
	origRouter := newRouter
	origCreatorSyncer := newCreatorSyncer
	origDashboardService := newDashboardService
	origRunMySQLMigrations := runMySQLMigrations
	origRunStartupRecovery := runStartupRecovery
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
		newCreatorSyncer = origCreatorSyncer
		newDashboardService = origDashboardService
		runMySQLMigrations = origRunMySQLMigrations
		runStartupRecovery = origRunStartupRecovery
	}()

	syncerStub := &stubCreatorSyncer{}
	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return &stubScheduler{}, nil
	}
	newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
		return &stubWorker{}
	}
	newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
		return &stubHTTPDashboardService{}
	}
	runMySQLMigrations = func(ctx context.Context, db *sql.DB) error {
		return nil
	}
	runStartupRecovery = func(context.Context, *App) error {
		return nil
	}
	newRouter = func(httpapi.CreatorService, httpapi.JobService, httpapi.DashboardService, httpapi.ConfigService) http.Handler {
		return http.NewServeMux()
	}
	newCreatorSyncer = func(service *creator.Service, filePath string, interval time.Duration) creatorSyncRunner {
		return syncerStub
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "dsn"
	cfg.Server.Addr = "127.0.0.1:0"
	cfg.Creators.File = "/tmp/creators.yaml"

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = app.Run(ctx)
		close(done)
	}()

	deadline := time.Now().Add(50 * time.Millisecond)
	for atomic.LoadInt32(&syncerStub.started) == 0 && time.Now().Before(deadline) {
		time.Sleep(1 * time.Millisecond)
	}
	cancel()
	<-done

	if atomic.LoadInt32(&syncerStub.started) != 1 {
		t.Fatalf("creator syncer not started")
	}
}

func TestNewRunsMySQLMigrationsWhenEnabled(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origWorker := newWorker
	origRouter := newRouter
	origDashboardService := newDashboardService
	origRunMySQLMigrations := runMySQLMigrations
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
		newDashboardService = origDashboardService
		runMySQLMigrations = origRunMySQLMigrations
	}()

	var called int32
	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return &stubScheduler{}, nil
	}
	newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
		return &stubWorker{}
	}
	newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
		return &stubHTTPDashboardService{}
	}
	newRouter = func(httpapi.CreatorService, httpapi.JobService, httpapi.DashboardService, httpapi.ConfigService) http.Handler {
		return http.NewServeMux()
	}
	runMySQLMigrations = func(ctx context.Context, db *sql.DB) error {
		atomic.AddInt32(&called, 1)
		return nil
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "dsn"
	cfg.Server.Addr = "127.0.0.1:0"
	cfg.MySQL.AutoMigrate = true

	if _, err := New(cfg); err != nil {
		t.Fatalf("New error: %v", err)
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("expected migration to run once")
	}
}

func TestNewSkipsMySQLMigrationsWhenDisabled(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origWorker := newWorker
	origRouter := newRouter
	origDashboardService := newDashboardService
	origRunMySQLMigrations := runMySQLMigrations
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
		newDashboardService = origDashboardService
		runMySQLMigrations = origRunMySQLMigrations
	}()

	var called int32
	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return &stubScheduler{}, nil
	}
	newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
		return &stubWorker{}
	}
	newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
		return &stubHTTPDashboardService{}
	}
	newRouter = func(httpapi.CreatorService, httpapi.JobService, httpapi.DashboardService, httpapi.ConfigService) http.Handler {
		return http.NewServeMux()
	}
	runMySQLMigrations = func(ctx context.Context, db *sql.DB) error {
		atomic.AddInt32(&called, 1)
		return nil
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "dsn"
	cfg.Server.Addr = "127.0.0.1:0"
	cfg.MySQL.AutoMigrate = false

	if _, err := New(cfg); err != nil {
		t.Fatalf("New error: %v", err)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatalf("expected migration to be skipped")
	}
}

func TestNewCreatesAuthWatcherWhenCookieConfigured(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origWorker := newWorker
	origRouter := newRouter
	origDashboardService := newDashboardService
	origRunMySQLMigrations := runMySQLMigrations
	origAuthWatcher := newAuthWatcher
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
		newDashboardService = origDashboardService
		runMySQLMigrations = origRunMySQLMigrations
		newAuthWatcher = origAuthWatcher
	}()

	watcher := &stubAuthWatcher{}
	var authWatcherCreated bool

	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return &stubScheduler{}, nil
	}
	newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
		return &stubWorker{}
	}
	newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
		return &stubHTTPDashboardService{}
	}
	runMySQLMigrations = func(ctx context.Context, db *sql.DB) error {
		return nil
	}
	newRouter = func(httpapi.CreatorService, httpapi.JobService, httpapi.DashboardService, httpapi.ConfigService) http.Handler {
		return http.NewServeMux()
	}
	newAuthWatcher = func(client bilibili.AuthClient, reloadInterval, checkInterval time.Duration) authWatcherRunner {
		authWatcherCreated = true
		return watcher
	}

	cfg := config.Default()
	cfg.Storage.RootDir = "/tmp"
	cfg.MySQL.DSN = "dsn"
	cfg.Server.Addr = "127.0.0.1:0"
	cfg.Bilibili.Cookie = "SESSDATA=ok"

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if !authWatcherCreated {
		t.Fatalf("expected auth watcher created")
	}
	if app.authWatcher == nil {
		t.Fatalf("expected auth watcher on app")
	}
}

func TestJobPayloadInt64(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		key     string
		want    int64
		ok      bool
	}{
		{name: "nil payload", payload: nil, key: "video_id", want: 0, ok: false},
		{name: "missing key", payload: map[string]any{"other": 1}, key: "video_id", want: 0, ok: false},
		{name: "int64", payload: map[string]any{"video_id": int64(11)}, key: "video_id", want: 11, ok: true},
		{name: "int", payload: map[string]any{"video_id": int(12)}, key: "video_id", want: 12, ok: true},
		{name: "float64", payload: map[string]any{"video_id": float64(13)}, key: "video_id", want: 13, ok: true},
		{name: "json number", payload: map[string]any{"video_id": json.Number("14")}, key: "video_id", want: 14, ok: true},
		{name: "string", payload: map[string]any{"video_id": "15"}, key: "video_id", want: 15, ok: true},
		{name: "bad json number", payload: map[string]any{"video_id": json.Number("bad")}, key: "video_id", want: 0, ok: false},
		{name: "bad string", payload: map[string]any{"video_id": "bad"}, key: "video_id", want: 0, ok: false},
		{name: "unsupported type", payload: map[string]any{"video_id": true}, key: "video_id", want: 0, ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := jobPayloadInt64(tc.payload, tc.key)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("expected (%d, %v), got (%d, %v)", tc.want, tc.ok, got, ok)
			}
		})
	}
}

func TestStorageVideoPathDefaultsPlatform(t *testing.T) {
	got := storageVideoPath("/data/archive", "", "BV1")
	want := "/data/archive/store/bilibili/BV1.mp4"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
