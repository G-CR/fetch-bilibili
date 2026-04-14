package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	httpapi "fetch-bilibili/internal/api/http"
	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/dashboard"
	"fetch-bilibili/internal/db"
	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/library"
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
var runMySQLMigrations = db.RunMySQLMigrations
var newScheduler = func(cfg config.SchedulerConfig, jobs scheduler.JobService) (schedulerRunner, error) {
	return scheduler.New(cfg, jobs, nil)
}
var newWorker = func(repo repo.JobRepository, handler worker.Handler, workers int, pollEvery time.Duration) workerRunner {
	return worker.New(repo, handler, workers, pollEvery, nil)
}
var newRouter = httpapi.NewRouter
var newDashboardService = func(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth bilibili.AuthClient, cfg config.Config) httpapi.DashboardService {
	return dashboard.New(db, creators, videos, jobs, auth, cfg)
}
var newAuthWatcher = func(client bilibili.AuthClient, reloadInterval, checkInterval time.Duration) authWatcherRunner {
	return bilibili.NewAuthWatcher(client, reloadInterval, checkInterval, nil)
}
var newCreatorSyncer = func(service *creator.Service, filePath string, interval time.Duration) creatorSyncRunner {
	return creator.NewFileSyncer(service, filePath, interval, nil)
}
var runStartupRecovery = func(ctx context.Context, app *App) error {
	return app.recoverRuntimeState(ctx)
}

var ErrRestartRequested = errors.New("restart requested")

type App struct {
	cfg         config.Config
	db          *sql.DB
	repos       repo.Repositories
	scheduler   schedulerRunner
	workers     workerRunner
	authWatcher authWatcherRunner
	creatorSync creatorSyncRunner
	server      *http.Server
	restartCh   chan struct{}
}

type configEditorAdapter struct {
	editor *config.Editor
}

func (a configEditorAdapter) Load(ctx context.Context) (httpapi.ConfigDocument, error) {
	doc, err := a.editor.Load(ctx)
	if err != nil {
		return httpapi.ConfigDocument{}, err
	}
	return httpapi.ConfigDocument{
		Path:    doc.Path,
		Content: doc.Content,
	}, nil
}

func (a configEditorAdapter) Save(ctx context.Context, content string) (httpapi.ConfigSaveResult, error) {
	result, err := a.editor.Save(ctx, content)
	if err != nil {
		return httpapi.ConfigSaveResult{}, err
	}
	return httpapi.ConfigSaveResult{
		Changed:          result.Changed,
		RestartScheduled: result.RestartScheduled,
		Path:             result.Path,
	}, nil
}

func New(cfg config.Config) (*App, error) {
	database, err := newMySQL(cfg.MySQL)
	if err != nil {
		return nil, err
	}
	if cfg.MySQL.AutoMigrate {
		if err := runMySQLMigrations(context.Background(), database); err != nil {
			_ = database.Close()
			return nil, err
		}
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
	handler.SetStoragePolicy(cfg.Storage.MaxBytes, cfg.Storage.SafeBytes, cfg.Storage.KeepOutOfPrint)
	handler.SetCleanupRetention(cfg.Storage.CleanupRetentionHours)
	pool := newWorker(repos.Jobs, handler, cfg.Limits.DownloadConcurrency, 2*time.Second)

	var authWatcher authWatcherRunner
	if cfg.Bilibili.Cookie != "" || cfg.Bilibili.SESSDATA != "" {
		authWatcher = newAuthWatcher(client, cfg.Bilibili.AuthReloadInterval, cfg.Bilibili.AuthCheckInterval)
	}

	var creatorSync creatorSyncRunner
	if cfg.Creators.File != "" {
		creatorSync = newCreatorSyncer(creatorService, cfg.Creators.File, cfg.Creators.ReloadInterval)
	}

	dashboardService := newDashboardService(database, repos.Creators, repos.Videos, repos.Jobs, client, cfg)
	app := &App{
		cfg:         cfg,
		db:          database,
		repos:       repos,
		scheduler:   sched,
		workers:     pool,
		authWatcher: authWatcher,
		creatorSync: creatorSync,
		restartCh:   make(chan struct{}, 1),
	}
	configEditor := config.NewEditor(resolveConfigPath(), app.requestRestart)
	router := newRouter(creatorService, jobService, dashboardService, configEditorAdapter{editor: configEditor})
	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
	app.server = server
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	if err := runStartupRecovery(ctx, a); err != nil {
		log.Printf("启动恢复失败: %v", err)
	}

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
		a.shutdown()
		return ctx.Err()
	case <-a.restartCh:
		a.shutdown()
		return ErrRestartRequested
	case err := <-errCh:
		a.closeResources()
		return err
	}
}

func resolveConfigPath() string {
	if path := os.Getenv("FETCH_CONFIG"); path != "" {
		return path
	}
	return "configs/config.yaml"
}

func (a *App) requestRestart() {
	select {
	case a.restartCh <- struct{}{}:
	default:
	}
}

func (a *App) shutdown() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.server.Shutdown(shutdownCtx)
	a.closeResources()
}

func (a *App) closeResources() {
	_ = a.db.Close()
	if a.workers != nil {
		a.workers.Wait()
	}
}

func (a *App) recoverRuntimeState(ctx context.Context) error {
	if err := a.requeueRunningJobs(ctx); err != nil {
		return err
	}
	if err := a.recoverMissingDownloadedVideos(ctx); err != nil {
		return err
	}
	if err := a.recoverDownloadingVideos(ctx); err != nil {
		return err
	}
	return nil
}

func (a *App) requeueRunningJobs(ctx context.Context) error {
	if a.repos.Jobs == nil {
		return nil
	}

	jobsList, err := a.repos.Jobs.ListRecent(ctx, repo.JobListFilter{
		Limit:  500,
		Status: jobs.StatusRunning,
	})
	if err != nil {
		return err
	}

	var recovered int
	for _, job := range jobsList {
		if err := a.repos.Jobs.UpdateStatus(ctx, job.ID, jobs.StatusQueued, "启动恢复后重新入队"); err != nil {
			return err
		}
		recovered++
	}
	if recovered > 0 {
		log.Printf("启动恢复：已重新入队 %d 个运行中任务", recovered)
	}
	return nil
}

func (a *App) recoverDownloadingVideos(ctx context.Context) error {
	if a.repos.Videos == nil {
		return nil
	}

	videosList, err := a.repos.Videos.ListRecent(ctx, repo.VideoListFilter{
		Limit: 500,
		State: "DOWNLOADING",
	})
	if err != nil {
		return err
	}
	if len(videosList) == 0 {
		return nil
	}

	activeDownloads := make(map[int64]struct{})
	if a.repos.Jobs != nil {
		jobsList, err := a.repos.Jobs.ListRecent(ctx, repo.JobListFilter{
			Limit: 1000,
			Type:  jobs.TypeDownload,
		})
		if err != nil {
			return err
		}
		for _, job := range jobsList {
			if job.Status != jobs.StatusQueued && job.Status != jobs.StatusRunning {
				continue
			}
			if videoID, ok := jobPayloadInt64(job.Payload, "video_id"); ok && videoID > 0 {
				activeDownloads[videoID] = struct{}{}
			}
		}
	}

	var recovered int
	for _, video := range videosList {
		if _, ok := activeDownloads[video.ID]; ok {
			continue
		}

		nextState := "NEW"
		path := storageVideoPath(a.cfg.Storage.RootDir, video.Platform, video.VideoID)
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			nextState = "DOWNLOADED"
		}
		if err := a.repos.Videos.UpdateState(ctx, video.ID, nextState); err != nil {
			return err
		}
		recovered++
	}
	if recovered > 0 {
		log.Printf("启动恢复：已修复 %d 个 DOWNLOADING 视频状态", recovered)
	}
	return nil
}

func (a *App) recoverMissingDownloadedVideos(ctx context.Context) error {
	if a.repos.Videos == nil {
		return nil
	}

	videosList, err := a.repos.Videos.ListRecent(ctx, repo.VideoListFilter{
		Limit: 500,
		State: "DOWNLOADED",
	})
	if err != nil {
		return err
	}
	if len(videosList) == 0 {
		return nil
	}

	var repaired int
	for _, video := range videosList {
		path := storageVideoPath(a.cfg.Storage.RootDir, video.Platform, video.VideoID)
		info, statErr := os.Stat(path)
		switch {
		case statErr == nil && info.Size() > 0:
			continue
		case statErr == nil:
			// 空文件视为无效文件，继续修复
		case os.IsNotExist(statErr):
		default:
			return statErr
		}

		if a.repos.VideoFiles != nil {
			if _, err := a.repos.VideoFiles.DeleteByVideoID(ctx, video.ID); err != nil {
				return err
			}
		}
		if err := a.repos.Videos.UpdateState(ctx, video.ID, "NEW"); err != nil {
			return err
		}
		repaired++
	}
	if repaired > 0 {
		log.Printf("启动恢复：已修复 %d 个 DOWNLOADED 缺失文件视频状态", repaired)
	}
	return nil
}

func storageVideoPath(root, platform, videoID string) string {
	return library.StoreVideoPath(root, platform, videoID)
}

func jobPayloadInt64(payload map[string]any, key string) (int64, bool) {
	if payload == nil {
		return 0, false
	}
	raw, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case float64:
		return int64(value), true
	case json.Number:
		n, err := value.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(value, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
