package httpapi

import (
	"context"
	"net/http"

	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/dashboard"
	"fetch-bilibili/internal/repo"
)

type CreatorService interface {
	Upsert(ctx context.Context, entry creator.Entry) (repo.Creator, error)
	ListActive(ctx context.Context, limit int) ([]repo.Creator, error)
	Patch(ctx context.Context, id int64, patch creator.Patch) (repo.Creator, error)
	Delete(ctx context.Context, id int64) error
}

type JobService interface {
	EnqueueFetch(ctx context.Context) error
	EnqueueCheck(ctx context.Context) error
	EnqueueCleanup(ctx context.Context) error
	EnqueueDownload(ctx context.Context, videoID int64) error
	EnqueueCheckVideo(ctx context.Context, videoID int64) error
}

type DashboardService interface {
	ListJobs(ctx context.Context, filter repo.JobListFilter) ([]repo.Job, error)
	ListVideos(ctx context.Context, filter repo.VideoListFilter) ([]repo.Video, error)
	GetVideo(ctx context.Context, id int64) (repo.Video, error)
	GetSystemStatus(ctx context.Context) (dashboard.SystemStatus, error)
	GetStorageStats(ctx context.Context) (dashboard.StorageStats, error)
}

type ConfigDocument struct {
	Path    string
	Content string
}

type ConfigSaveResult struct {
	Changed          bool
	RestartScheduled bool
	Path             string
}

type ConfigService interface {
	Load(ctx context.Context) (ConfigDocument, error)
	Save(ctx context.Context, content string) (ConfigSaveResult, error)
}

func NewRouter(creatorSvc CreatorService, jobSvc JobService, dashboardSvc DashboardService, configSvc ConfigService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	mux.Handle("/creators", newCreatorHandler(creatorSvc))
	mux.Handle("/creators/", newCreatorItemHandler(creatorSvc))
	mux.Handle("/jobs", newJobHandler(jobSvc, dashboardSvc))
	mux.Handle("/videos", newVideoHandler(dashboardSvc))
	mux.Handle("/videos/", newVideoItemHandler(jobSvc, dashboardSvc))
	mux.Handle("/system/status", newSystemStatusHandler(dashboardSvc))
	mux.Handle("/system/config", newSystemConfigHandler(configSvc))
	mux.Handle("/storage/stats", newStorageStatsHandler(dashboardSvc))
	mux.Handle("/storage/cleanup", newStorageCleanupHandler(jobSvc))

	return withCORS(mux)
}
