package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/repo"
)

type recoveryVideoRepo struct {
	videos  []repo.Video
	updates []videoStateUpdate
}

type videoStateUpdate struct {
	id    int64
	state string
}

func (r *recoveryVideoRepo) Upsert(context.Context, repo.Video) (int64, bool, error) {
	panic("unexpected call to Upsert")
}

func (r *recoveryVideoRepo) UpdateState(_ context.Context, id int64, state string) error {
	r.updates = append(r.updates, videoStateUpdate{id: id, state: state})
	for i := range r.videos {
		if r.videos[i].ID == id {
			r.videos[i].State = state
		}
	}
	return nil
}

func (r *recoveryVideoRepo) FindByID(context.Context, int64) (repo.Video, error) {
	panic("unexpected call to FindByID")
}

func (r *recoveryVideoRepo) ListForCheck(context.Context, int) ([]repo.Video, error) {
	panic("unexpected call to ListForCheck")
}

func (r *recoveryVideoRepo) ListRecent(_ context.Context, filter repo.VideoListFilter) ([]repo.Video, error) {
	if filter.State == "" {
		return append([]repo.Video(nil), r.videos...), nil
	}
	var out []repo.Video
	for _, video := range r.videos {
		if video.State == filter.State {
			out = append(out, video)
		}
	}
	return out, nil
}

func (r *recoveryVideoRepo) ListLibraryByCreator(context.Context, int64) ([]repo.LibraryVideo, error) {
	panic("unexpected call to ListLibraryByCreator")
}

func (r *recoveryVideoRepo) ListCleanupCandidates(context.Context, repo.CleanupCandidateFilter) ([]repo.CleanupCandidate, error) {
	panic("unexpected call to ListCleanupCandidates")
}

func (r *recoveryVideoRepo) CountByState(context.Context, string) (int64, error) {
	panic("unexpected call to CountByState")
}

func (r *recoveryVideoRepo) UpdateCheckStatus(context.Context, int64, string, *time.Time, *time.Time, time.Time) error {
	panic("unexpected call to UpdateCheckStatus")
}

type recoveryJobRepo struct {
	jobs    []repo.Job
	updates []jobStatusUpdate
}

type recoveryVideoFileRepo struct {
	deletedVideoIDs []int64
}

type jobStatusUpdate struct {
	id     int64
	status string
	errMsg string
}

func (r *recoveryJobRepo) Enqueue(context.Context, repo.Job) (int64, error) {
	panic("unexpected call to Enqueue")
}

func (r *recoveryJobRepo) FetchQueued(context.Context, int) ([]repo.Job, error) {
	panic("unexpected call to FetchQueued")
}

func (r *recoveryJobRepo) ListRecent(_ context.Context, filter repo.JobListFilter) ([]repo.Job, error) {
	var out []repo.Job
	for _, job := range r.jobs {
		if filter.Status != "" && job.Status != filter.Status {
			continue
		}
		if filter.Type != "" && job.Type != filter.Type {
			continue
		}
		out = append(out, job)
	}
	return out, nil
}

func (r *recoveryJobRepo) CountByStatuses(context.Context, []string) (int64, error) {
	panic("unexpected call to CountByStatuses")
}

func (r *recoveryJobRepo) UpdateStatus(_ context.Context, id int64, status string, errMsg string) error {
	r.updates = append(r.updates, jobStatusUpdate{
		id:     id,
		status: status,
		errMsg: errMsg,
	})
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			r.jobs[i].Status = status
			r.jobs[i].ErrorMsg = errMsg
		}
	}
	return nil
}

func (r *recoveryVideoFileRepo) Create(context.Context, repo.VideoFile) (int64, error) {
	panic("unexpected call to Create")
}

func (r *recoveryVideoFileRepo) DeleteByID(context.Context, int64) (int64, error) {
	panic("unexpected call to DeleteByID")
}

func (r *recoveryVideoFileRepo) DeleteByVideoID(_ context.Context, videoID int64) (int64, error) {
	r.deletedVideoIDs = append(r.deletedVideoIDs, videoID)
	return 1, nil
}

func (r *recoveryVideoFileRepo) CountByVideoID(context.Context, int64) (int64, error) {
	panic("unexpected call to CountByVideoID")
}

func TestRecoverRuntimeStateRequeuesRunningJobs(t *testing.T) {
	jobsRepo := &recoveryJobRepo{
		jobs: []repo.Job{
			{ID: 1, Type: jobs.TypeFetch, Status: jobs.StatusRunning},
			{ID: 2, Type: jobs.TypeCheck, Status: jobs.StatusSuccess},
		},
	}
	app := &App{
		repos: repo.Repositories{Jobs: jobsRepo},
	}

	if err := app.recoverRuntimeState(context.Background()); err != nil {
		t.Fatalf("recover runtime state: %v", err)
	}

	if len(jobsRepo.updates) != 1 {
		t.Fatalf("expected 1 job requeued, got %d", len(jobsRepo.updates))
	}
	got := jobsRepo.updates[0]
	if got.id != 1 || got.status != jobs.StatusQueued {
		t.Fatalf("unexpected requeue update: %+v", got)
	}
	if got.errMsg != "启动恢复后重新入队" {
		t.Fatalf("unexpected recovery message: %s", got.errMsg)
	}
}

func TestRecoverRuntimeStateNoRepos(t *testing.T) {
	app := &App{cfg: config.Default()}
	if err := app.recoverRuntimeState(context.Background()); err != nil {
		t.Fatalf("recover runtime state: %v", err)
	}
}

func TestRecoverRuntimeStateResetsDownloadingVideoToNew(t *testing.T) {
	videosRepo := &recoveryVideoRepo{
		videos: []repo.Video{
			{ID: 1, Platform: "bilibili", VideoID: "BV-new", State: "DOWNLOADING"},
		},
	}
	jobsRepo := &recoveryJobRepo{}
	app := &App{
		cfg:   config.Default(),
		repos: repo.Repositories{Videos: videosRepo, Jobs: jobsRepo},
	}
	app.cfg.Storage.RootDir = t.TempDir()

	if err := app.recoverRuntimeState(context.Background()); err != nil {
		t.Fatalf("recover runtime state: %v", err)
	}

	if len(videosRepo.updates) != 1 {
		t.Fatalf("expected 1 video state update, got %d", len(videosRepo.updates))
	}
	if videosRepo.updates[0].state != "NEW" {
		t.Fatalf("expected NEW state, got %s", videosRepo.updates[0].state)
	}
}

func TestRecoverRuntimeStateMarksDownloadedWhenFileExists(t *testing.T) {
	rootDir := t.TempDir()
	dst := storageVideoPath(rootDir, "bilibili", "BV-downloaded")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dst, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	videosRepo := &recoveryVideoRepo{
		videos: []repo.Video{
			{ID: 1, Platform: "bilibili", VideoID: "BV-downloaded", State: "DOWNLOADING"},
		},
	}
	app := &App{
		cfg:   config.Default(),
		repos: repo.Repositories{Videos: videosRepo, Jobs: &recoveryJobRepo{}},
	}
	app.cfg.Storage.RootDir = rootDir

	if err := app.recoverRuntimeState(context.Background()); err != nil {
		t.Fatalf("recover runtime state: %v", err)
	}

	if len(videosRepo.updates) != 1 {
		t.Fatalf("expected 1 video state update, got %d", len(videosRepo.updates))
	}
	if videosRepo.updates[0].state != "DOWNLOADED" {
		t.Fatalf("expected DOWNLOADED state, got %s", videosRepo.updates[0].state)
	}
}

func TestRecoverRuntimeStateKeepsDownloadingWhenActiveJobPayloadIsString(t *testing.T) {
	videosRepo := &recoveryVideoRepo{
		videos: []repo.Video{
			{ID: 1, Platform: "bilibili", VideoID: "BV-active", State: "DOWNLOADING"},
		},
	}
	jobsRepo := &recoveryJobRepo{
		jobs: []repo.Job{
			{
				ID:      10,
				Type:    jobs.TypeDownload,
				Status:  jobs.StatusRunning,
				Payload: map[string]any{"video_id": "1"},
			},
		},
	}
	app := &App{
		cfg:   config.Default(),
		repos: repo.Repositories{Videos: videosRepo, Jobs: jobsRepo},
	}
	app.cfg.Storage.RootDir = t.TempDir()

	if err := app.recoverRuntimeState(context.Background()); err != nil {
		t.Fatalf("recover runtime state: %v", err)
	}

	if len(videosRepo.updates) != 0 {
		t.Fatalf("expected active download to skip recovery, got %+v", videosRepo.updates)
	}
}

func TestRecoverRuntimeStateResetsDownloadedVideoToNewWhenFileMissing(t *testing.T) {
	videosRepo := &recoveryVideoRepo{
		videos: []repo.Video{
			{ID: 7, Platform: "bilibili", VideoID: "BV-missing", State: "DOWNLOADED"},
		},
	}
	videoFiles := &recoveryVideoFileRepo{}
	app := &App{
		cfg: config.Config{
			Storage: config.StorageConfig{RootDir: t.TempDir()},
		},
		repos: repo.Repositories{
			Videos:     videosRepo,
			VideoFiles: videoFiles,
			Jobs:       &recoveryJobRepo{},
		},
	}

	if err := app.recoverRuntimeState(context.Background()); err != nil {
		t.Fatalf("recover runtime state: %v", err)
	}

	if len(videosRepo.updates) != 1 {
		t.Fatalf("expected 1 video state update, got %d", len(videosRepo.updates))
	}
	if videosRepo.updates[0].id != 7 || videosRepo.updates[0].state != "NEW" {
		t.Fatalf("unexpected update: %+v", videosRepo.updates[0])
	}
	if len(videoFiles.deletedVideoIDs) != 1 || videoFiles.deletedVideoIDs[0] != 7 {
		t.Fatalf("expected stale file records removed, got %+v", videoFiles.deletedVideoIDs)
	}
}

func TestRecoverRuntimeStateResetsDownloadedVideoToNewWhenFileEmpty(t *testing.T) {
	rootDir := t.TempDir()
	dst := storageVideoPath(rootDir, "bilibili", "BV-empty")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dst, nil, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	videosRepo := &recoveryVideoRepo{
		videos: []repo.Video{
			{ID: 9, Platform: "bilibili", VideoID: "BV-empty", State: "DOWNLOADED"},
		},
	}
	videoFiles := &recoveryVideoFileRepo{}
	app := &App{
		cfg: config.Config{
			Storage: config.StorageConfig{RootDir: rootDir},
		},
		repos: repo.Repositories{
			Videos:     videosRepo,
			VideoFiles: videoFiles,
		},
	}

	if err := app.recoverRuntimeState(context.Background()); err != nil {
		t.Fatalf("recover runtime state: %v", err)
	}

	if len(videosRepo.updates) != 1 {
		t.Fatalf("expected 1 video state update, got %d", len(videosRepo.updates))
	}
	if videosRepo.updates[0].id != 9 || videosRepo.updates[0].state != "NEW" {
		t.Fatalf("unexpected update: %+v", videosRepo.updates[0])
	}
	if len(videoFiles.deletedVideoIDs) != 1 || videoFiles.deletedVideoIDs[0] != 9 {
		t.Fatalf("expected stale file records removed, got %+v", videoFiles.deletedVideoIDs)
	}
}
