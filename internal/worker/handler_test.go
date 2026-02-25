package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
)

type fakeCreators struct {
	list []repo.Creator
	err  error
}

func (f *fakeCreators) Create(ctx context.Context, c repo.Creator) (int64, error) {
	return 0, repo.ErrNotImplemented
}

func (f *fakeCreators) Upsert(ctx context.Context, c repo.Creator) (int64, error) {
	return 0, repo.ErrNotImplemented
}

func (f *fakeCreators) Update(ctx context.Context, c repo.Creator) error {
	return repo.ErrNotImplemented
}

func (f *fakeCreators) UpdateStatus(ctx context.Context, id int64, status string) error {
	return repo.ErrNotImplemented
}

func (f *fakeCreators) FindByID(ctx context.Context, id int64) (repo.Creator, error) {
	return repo.Creator{}, repo.ErrNotImplemented
}

func (f *fakeCreators) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	return f.list, f.err
}

func (f *fakeCreators) ListActiveAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	return f.list, f.err
}

type fakeVideos struct {
	upsertErrIDs map[string]error
	list         []repo.Video
	listErr      error
	updates      []updateCall
	updateErr    error
	find         map[int64]repo.Video
	findErr      error
	states       map[int64]string
}

type updateCall struct {
	id     int64
	state  string
	outAt  *time.Time
	stable *time.Time
	last   time.Time
}

type fakeJobs struct {
	enqueued []repo.Job
	err      error
}

func (f *fakeJobs) Enqueue(ctx context.Context, job repo.Job) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.enqueued = append(f.enqueued, job)
	return int64(len(f.enqueued)), nil
}

func (f *fakeJobs) FetchQueued(ctx context.Context, limit int) ([]repo.Job, error) {
	return nil, repo.ErrNotImplemented
}

func (f *fakeJobs) UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error {
	return repo.ErrNotImplemented
}

type fakeVideoFiles struct {
	created []repo.VideoFile
	err     error
}

func (f *fakeVideoFiles) Create(ctx context.Context, file repo.VideoFile) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.created = append(f.created, file)
	return int64(len(f.created)), nil
}

func (f *fakeVideos) Upsert(ctx context.Context, v repo.Video) (int64, bool, error) {
	if f.upsertErrIDs != nil {
		if err, ok := f.upsertErrIDs[v.VideoID]; ok {
			return 0, false, err
		}
	}
	return 1, true, nil
}

func (f *fakeVideos) UpdateState(ctx context.Context, id int64, state string) error {
	if f.states == nil {
		f.states = make(map[int64]string)
	}
	f.states[id] = state
	return nil
}

func (f *fakeVideos) FindByID(ctx context.Context, id int64) (repo.Video, error) {
	if f.findErr != nil {
		return repo.Video{}, f.findErr
	}
	if f.find != nil {
		if v, ok := f.find[id]; ok {
			return v, nil
		}
	}
	return repo.Video{}, repo.ErrNotImplemented
}

func (f *fakeVideos) ListForCheck(ctx context.Context, limit int) ([]repo.Video, error) {
	return f.list, f.listErr
}

func (f *fakeVideos) UpdateCheckStatus(ctx context.Context, id int64, state string, outOfPrintAt *time.Time, stableAt *time.Time, lastCheckAt time.Time) error {
	f.updates = append(f.updates, updateCall{id: id, state: state, outAt: outOfPrintAt, stable: stableAt, last: lastCheckAt})
	return f.updateErr
}

type stubClient struct {
	list      []bilibili.VideoMeta
	listErr   error
	available map[string]bool
	checkErr  map[string]error
	download  map[string]int64
	downErr   map[string]error
}

func (s *stubClient) ListVideos(ctx context.Context, uid string) ([]bilibili.VideoMeta, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.list, nil
}

func (s *stubClient) CheckAvailable(ctx context.Context, videoID string) (bool, error) {
	if s.checkErr != nil {
		if err, ok := s.checkErr[videoID]; ok {
			return false, err
		}
	}
	if s.available != nil {
		if ok, exists := s.available[videoID]; exists {
			return ok, nil
		}
	}
	return true, nil
}

func (s *stubClient) Download(ctx context.Context, videoID, dst string) (int64, error) {
	if s.downErr != nil {
		if err, ok := s.downErr[videoID]; ok {
			return 0, err
		}
	}
	if s.download != nil {
		if size, ok := s.download[videoID]; ok {
			return size, nil
		}
	}
	return 0, nil
}

func TestHandleFetchNotInitialized(t *testing.T) {
	h := NewDefaultHandler(nil, nil, nil, nil, nil, 30, "/tmp", 0, 0, nil)
	if err := h.Handle(context.Background(), repo.Job{Type: "fetch"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleFetchNoCreators(t *testing.T) {
	creators := &fakeCreators{list: []repo.Creator{}}
	h := NewDefaultHandler(creators, &fakeVideos{}, nil, nil, &stubClient{}, 30, "/tmp", 0, 0, nil)
	if err := h.Handle(context.Background(), repo.Job{Type: "fetch"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleFetchEnqueueDownload(t *testing.T) {
	creators := &fakeCreators{list: []repo.Creator{{ID: 1, UID: "123"}}}
	videos := &fakeVideos{}
	jobsRepo := &fakeJobs{}
	client := &stubClient{list: []bilibili.VideoMeta{{VideoID: "v1"}, {VideoID: "v2"}}}
	h := NewDefaultHandler(creators, videos, nil, jobsRepo, client, 30, "/tmp", 0, 0, nil)

	if err := h.Handle(context.Background(), repo.Job{Type: "fetch"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobsRepo.enqueued) != 2 {
		t.Fatalf("expected 2 download jobs, got %d", len(jobsRepo.enqueued))
	}
	if jobsRepo.enqueued[0].Type != jobs.TypeDownload {
		t.Fatalf("expected download job")
	}
}

func TestHandleFetchWithUpsertError(t *testing.T) {
	creators := &fakeCreators{list: []repo.Creator{{ID: 1, UID: "123"}}}
	videos := &fakeVideos{upsertErrIDs: map[string]error{"bad": errors.New("upsert")}}
	client := &stubClient{list: []bilibili.VideoMeta{{VideoID: "bad"}, {VideoID: "ok"}}}
	h := NewDefaultHandler(creators, videos, nil, &fakeJobs{}, client, 30, "/tmp", 0, 0, nil)

	err := h.Handle(context.Background(), repo.Job{Type: "fetch"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleFetchListError(t *testing.T) {
	creators := &fakeCreators{err: errors.New("list error")}
	h := NewDefaultHandler(creators, &fakeVideos{}, nil, nil, &stubClient{}, 30, "/tmp", 0, 0, nil)

	if err := h.Handle(context.Background(), repo.Job{Type: "fetch"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleCheckNotInitialized(t *testing.T) {
	h := NewDefaultHandler(nil, nil, nil, nil, nil, 30, "/tmp", 0, 0, nil)
	if err := h.Handle(context.Background(), repo.Job{Type: "check"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleCheckNoVideos(t *testing.T) {
	videos := &fakeVideos{list: []repo.Video{}}
	h := NewDefaultHandler(&fakeCreators{}, videos, nil, nil, &stubClient{}, 30, "/tmp", 0, 0, nil)
	if err := h.Handle(context.Background(), repo.Job{Type: "check"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleCheckUpdatesStatus(t *testing.T) {
	now := time.Now()
	videos := &fakeVideos{list: []repo.Video{
		{ID: 1, VideoID: "v1", PublishTime: now.Add(-40 * 24 * time.Hour), State: "DOWNLOADED"},
		{ID: 2, VideoID: "v2", PublishTime: now.Add(-10 * 24 * time.Hour), State: "DOWNLOADED"},
	}}
	client := &stubClient{available: map[string]bool{"v1": true, "v2": false}}
	h := NewDefaultHandler(&fakeCreators{}, videos, nil, nil, client, 30, "/tmp", 0, 0, nil)

	if err := h.Handle(context.Background(), repo.Job{Type: "check"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos.updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(videos.updates))
	}
}

func TestHandleCheckPublishTimeZero(t *testing.T) {
	videos := &fakeVideos{list: []repo.Video{{ID: 1, VideoID: "v1", State: "DOWNLOADED"}}}
	client := &stubClient{available: map[string]bool{"v1": true}}
	h := NewDefaultHandler(&fakeCreators{}, videos, nil, nil, client, 30, "/tmp", 0, 0, nil)

	if err := h.Handle(context.Background(), repo.Job{Type: "check"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos.updates) != 1 {
		t.Fatalf("expected 1 update")
	}
}

func TestHandleCheckClientError(t *testing.T) {
	videos := &fakeVideos{list: []repo.Video{{ID: 1, VideoID: "v1", State: "DOWNLOADED"}}}
	client := &stubClient{checkErr: map[string]error{"v1": errors.New("check error")}}
	h := NewDefaultHandler(&fakeCreators{}, videos, nil, nil, client, 30, "/tmp", 0, 0, nil)

	if err := h.Handle(context.Background(), repo.Job{Type: "check"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleCheckUpdateError(t *testing.T) {
	videos := &fakeVideos{list: []repo.Video{{ID: 1, VideoID: "v1", State: "DOWNLOADED"}}, updateErr: errors.New("update error")}
	client := &stubClient{available: map[string]bool{"v1": true}}
	h := NewDefaultHandler(&fakeCreators{}, videos, nil, nil, client, 30, "/tmp", 0, 0, nil)

	if err := h.Handle(context.Background(), repo.Job{Type: "check"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleUnknownType(t *testing.T) {
	h := NewDefaultHandler(&fakeCreators{}, &fakeVideos{}, nil, nil, &stubClient{}, 30, "/tmp", 0, 0, nil)
	if err := h.Handle(context.Background(), repo.Job{Type: "unknown"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleDownloadSuccess(t *testing.T) {
	dir := t.TempDir()
	videos := &fakeVideos{
		find: map[int64]repo.Video{
			1: {ID: 1, VideoID: "v1", Platform: "bilibili", State: "NEW"},
		},
	}
	files := &fakeVideoFiles{}
	client := &stubClient{download: map[string]int64{"v1": 10}}
	h := NewDefaultHandler(&fakeCreators{}, videos, files, nil, client, 30, dir, 0, 0, nil)

	job := repo.Job{Type: jobs.TypeDownload, Payload: map[string]any{"video_id": int64(1)}}
	if err := h.Handle(context.Background(), job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files.created) != 1 {
		t.Fatalf("expected file record")
	}
	if videos.states[1] != "DOWNLOADED" {
		t.Fatalf("expected state DOWNLOADED")
	}
}

func TestHandleDownloadMissingID(t *testing.T) {
	h := NewDefaultHandler(&fakeCreators{}, &fakeVideos{}, &fakeVideoFiles{}, nil, &stubClient{}, 30, "/tmp", 0, 0, nil)
	if err := h.Handle(context.Background(), repo.Job{Type: jobs.TypeDownload}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleDownloadSkipExisting(t *testing.T) {
	dir := t.TempDir()
	path := buildVideoPath(dir, "bilibili", "v1")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	videos := &fakeVideos{
		find: map[int64]repo.Video{
			1: {ID: 1, VideoID: "v1", Platform: "bilibili", State: "DOWNLOADED"},
		},
	}
	files := &fakeVideoFiles{}
	client := &stubClient{downErr: map[string]error{"v1": errors.New("should not download")}}
	h := NewDefaultHandler(&fakeCreators{}, videos, files, nil, client, 30, dir, 0, 0, nil)

	job := repo.Job{Type: jobs.TypeDownload, Payload: map[string]any{"video_id": int64(1)}}
	if err := h.Handle(context.Background(), job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files.created) != 0 {
		t.Fatalf("expected no file record")
	}
	if len(videos.states) != 0 {
		t.Fatalf("expected no state update")
	}
}

func TestHandleDownloadNoStorageRoot(t *testing.T) {
	videos := &fakeVideos{
		find: map[int64]repo.Video{
			1: {ID: 1, VideoID: "v1", Platform: "bilibili", State: "NEW"},
		},
	}
	h := NewDefaultHandler(&fakeCreators{}, videos, &fakeVideoFiles{}, nil, &stubClient{}, 30, "", 0, 0, nil)
	job := repo.Job{Type: jobs.TypeDownload, Payload: map[string]any{"video_id": int64(1)}}
	if err := h.Handle(context.Background(), job); err == nil {
		t.Fatalf("expected error")
	}
}

func TestPayloadInt64(t *testing.T) {
	tests := []struct {
		val any
		ok  bool
	}{
		{int64(1), true},
		{int(2), true},
		{float64(3), true},
		{"4", true},
		{"bad", false},
		{nil, false},
	}
	for _, tt := range tests {
		_, ok := payloadInt64(map[string]any{"k": tt.val}, "k")
		if ok != tt.ok {
			t.Fatalf("unexpected parse result for %v", tt.val)
		}
	}
}

func TestHandleDownloadClientError(t *testing.T) {
	dir := t.TempDir()
	videos := &fakeVideos{
		find: map[int64]repo.Video{
			1: {ID: 1, VideoID: "v1", Platform: "bilibili", State: "NEW"},
		},
	}
	files := &fakeVideoFiles{}
	client := &stubClient{downErr: map[string]error{"v1": errors.New("fail")}}
	h := NewDefaultHandler(&fakeCreators{}, videos, files, nil, client, 30, dir, 0, 0, nil)

	job := repo.Job{Type: jobs.TypeDownload, Payload: map[string]any{"video_id": int64(1)}}
	if err := h.Handle(context.Background(), job); err == nil {
		t.Fatalf("expected error")
	}
	if videos.states[1] != "NEW" {
		t.Fatalf("expected state reset to NEW")
	}
}

func TestWaitForCreatorLimits(t *testing.T) {
	h := NewDefaultHandler(&fakeCreators{}, &fakeVideos{}, nil, nil, &stubClient{}, 30, "/tmp", 1, 1, nil)
	lim := h.getCreatorLimiter(1)
	lim.last = time.Now().Add(-2 * time.Second)
	h.globalLimit.last = time.Now().Add(-2 * time.Second)

	if err := h.waitForCreator(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.getCreatorLimiter(1) != lim {
		t.Fatalf("expected reuse limiter")
	}
}

func TestWaitForCreatorCanceled(t *testing.T) {
	h := NewDefaultHandler(&fakeCreators{}, &fakeVideos{}, nil, nil, &stubClient{}, 30, "/tmp", 1, 1, nil)
	h.globalLimit.last = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.waitForCreator(ctx, 1); err == nil {
		t.Fatalf("expected error")
	}
}
