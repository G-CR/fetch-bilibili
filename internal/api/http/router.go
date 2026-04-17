package httpapi

import (
	"context"
	"net/http"

	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/dashboard"
	"fetch-bilibili/internal/discovery"
	"fetch-bilibili/internal/live"
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

type CandidateService interface {
	ListCandidates(ctx context.Context, filter repo.CandidateListFilter) ([]discovery.CandidateView, int64, error)
	GetCandidate(ctx context.Context, id int64) (discovery.CandidateDetailView, error)
	TriggerDiscover(ctx context.Context) error
	Approve(ctx context.Context, id int64) (repo.Creator, error)
	Ignore(ctx context.Context, id int64) error
	Block(ctx context.Context, id int64) error
	Review(ctx context.Context, id int64) error
}

func NewRouter(creatorSvc CreatorService, jobSvc JobService, dashboardSvc DashboardService, configSvc ConfigService, candidateSvc CandidateService, broker *live.Broker) http.Handler {
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
	mux.Handle("/events/stream", newEventsStreamHandler(broker))
	mux.Handle("/videos", newVideoHandler(dashboardSvc))
	mux.Handle("/videos/", newVideoItemHandler(jobSvc, dashboardSvc))
	mux.Handle("/candidate-creators", newCandidateHandler(candidateSvc))
	mux.Handle("/candidate-creators/discover", newCandidateDiscoverHandler(candidateSvc))
	mux.Handle("/candidate-creators/", newCandidateItemHandler(candidateSvc))
	mux.Handle("/system/status", newSystemStatusHandler(dashboardSvc))
	mux.Handle("/system/config", newSystemConfigHandler(configSvc))
	mux.Handle("/storage/stats", newStorageStatsHandler(dashboardSvc))
	mux.Handle("/storage/cleanup", newStorageCleanupHandler(jobSvc))

	return withCORS(mux)
}
