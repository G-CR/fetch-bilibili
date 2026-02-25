package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	httpapi "fetch-bilibili/internal/api/http"
	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/creator"
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

func TestNewSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()
	mock.ExpectPing()

	origMySQL := newMySQL
	origScheduler := newScheduler
	origWorker := newWorker
	origRouter := newRouter
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
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
	newRouter = func(httpapi.CreatorService) http.Handler {
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
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
	}()

	newMySQL = func(cfg config.MySQLConfig) (*sql.DB, error) {
		return db, nil
	}
	newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
		return nil, errors.New("scheduler error")
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
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
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
	newRouter = func(httpapi.CreatorService) http.Handler {
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
	cancel()
	if err := app.Run(ctx); !errors.Is(err, context.Canceled) {
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
	defer func() {
		newMySQL = origMySQL
		newScheduler = origScheduler
		newWorker = origWorker
		newRouter = origRouter
		newCreatorSyncer = origCreatorSyncer
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
	newRouter = func(httpapi.CreatorService) http.Handler {
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
