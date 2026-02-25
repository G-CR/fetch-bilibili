package app

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	httpapi "fetch-bilibili/internal/api/http"
	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/db"
	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
	mysqlrepo "fetch-bilibili/internal/repo/mysql"
	"fetch-bilibili/internal/scheduler"
	"fetch-bilibili/internal/worker"
)

type schedulerRunner interface {
	Start(context.Context)
}

type workerRunner interface {
	Start(context.Context)
	Wait()
}

type authWatcherRunner interface {
	Start(context.Context)
}

type creatorSyncRunner interface {
	Start(context.Context)
}

var newMySQL = db.NewMySQL
var newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
	return scheduler.New(cfg, jobs, nil)
}
var newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
	return worker.New(repo, handler, workers, pollEvery, nil)
}
var newRouter = httpapi.NewRouter
var newAuthWatcher = func(client bilibili.AuthClient, reloadInterval, checkInterval time.Duration) authWatcherRunner {
	return bilibili.NewAuthWatcher(client, reloadInterval, checkInterval, nil)
}
var newCreatorSyncer = func(service *creator.Service, filePath string, interval time.Duration) creatorSyncRunner {
	return creator.NewFileSyncer(service, filePath, interval, nil)
}

type App struct {
	cfg         config.Config
	db          *sql.DB
	repos       repo.Repositories
	scheduler   schedulerRunner
	workers     workerRunner
	authWatcher authWatcherRunner
	creatorSync creatorSyncRunner
	server      *http.Server
}

func New(cfg config.Config) (*App, error) {
	database, err := newMySQL(cfg.MySQL)
	if err != nil {
		return nil, err
	}

	repoImpl := mysqlrepo.New(database)
	repos := repo.Repositories{
		Creators:   repoImpl.Creators(),
		Videos:     repoImpl.Videos(),
		VideoFiles: repoImpl.VideoFiles(),
		Jobs:       repoImpl.Jobs(),
	}

	jobService := jobs.NewService(repos.Jobs)
	sched, err := newScheduler(cfg.Scheduler, jobService)
	if err != nil {
		_ = database.Close()
		return nil, err
	}

	client := bilibili.New(cfg.Bilibili, nil)
	creatorService := creator.NewService(repos.Creators, client, nil)
	handler := worker.NewDefaultHandler(
		repos.Creators,
		repos.Videos,
		repos.VideoFiles,
		repos.Jobs,
		client,
		cfg.Scheduler.CheckStableDays,
		cfg.Storage.RootDir,
		cfg.Limits.GlobalQPS,
		cfg.Limits.PerCreatorQPS,
		nil,
	)
	pool := newWorker(repos.Jobs, handler, cfg.Limits.DownloadConcurrency, 2*time.Second)

	var authWatcher authWatcherRunner
	if cfg.Bilibili.Cookie != "" || cfg.Bilibili.SESSDATA != "" || cfg.Bilibili.CookieFile != "" || cfg.Bilibili.SESSDATAFile != "" {
		authWatcher = newAuthWatcher(client, cfg.Bilibili.AuthReloadInterval, cfg.Bilibili.AuthCheckInterval)
	}

	var creatorSync creatorSyncRunner
	if cfg.Creators.File != "" {
		creatorSync = newCreatorSyncer(creatorService, cfg.Creators.File, cfg.Creators.ReloadInterval)
	}

	router := newRouter(creatorService)
	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return &App{
		cfg:         cfg,
		db:          database,
		repos:       repos,
		scheduler:   sched,
		workers:     pool,
		authWatcher: authWatcher,
		creatorSync: creatorSync,
		server:      server,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	if a.scheduler != nil {
		go a.scheduler.Start(ctx)
	}
	if a.workers != nil {
		go a.workers.Start(ctx)
	}
	if a.authWatcher != nil {
		go a.authWatcher.Start(ctx)
	}
	if a.creatorSync != nil {
		go a.creatorSync.Start(ctx)
	}
	go func() {
		errCh <- a.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.server.Shutdown(shutdownCtx)
		_ = a.db.Close()
		if a.workers != nil {
			a.workers.Wait()
		}
		return ctx.Err()
	case err := <-errCh:
		_ = a.db.Close()
		if a.workers != nil {
			a.workers.Wait()
		}
		return err
	}
}
