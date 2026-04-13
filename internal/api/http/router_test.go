package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/dashboard"
	"fetch-bilibili/internal/repo"
)

type stubCreatorService struct {
	last    creator.Entry
	result  repo.Creator
	err     error
	list    []repo.Creator
	listErr error
	patchID int64
	patch   creator.Patch
}

func (s *stubCreatorService) Upsert(ctx context.Context, entry creator.Entry) (repo.Creator, error) {
	s.last = entry
	if s.err != nil {
		return repo.Creator{}, s.err
	}
	return s.result, nil
}

func (s *stubCreatorService) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := append([]repo.Creator(nil), s.list...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubCreatorService) Patch(ctx context.Context, id int64, patch creator.Patch) (repo.Creator, error) {
	s.patchID = id
	s.patch = patch
	if s.err != nil {
		return repo.Creator{}, s.err
	}
	if s.result.ID == 0 {
		s.result = repo.Creator{ID: id}
	}
	return s.result, nil
}

type stubJobService struct {
	fetchCalled   int
	checkCalled   int
	cleanupCalled int
	downloadID    int64
	checkVideoID  int64
	err           error
}

func (s *stubJobService) EnqueueFetch(context.Context) error {
	s.fetchCalled++
	return s.err
}

func (s *stubJobService) EnqueueCheck(context.Context) error {
	s.checkCalled++
	return s.err
}

func (s *stubJobService) EnqueueCleanup(context.Context) error {
	s.cleanupCalled++
	return s.err
}

func (s *stubJobService) EnqueueDownload(_ context.Context, videoID int64) error {
	s.downloadID = videoID
	return s.err
}

func (s *stubJobService) EnqueueCheckVideo(_ context.Context, videoID int64) error {
	s.checkVideoID = videoID
	return s.err
}

type stubDashboardService struct {
	jobs            []repo.Job
	jobsErr         error
	lastJobFilter   repo.JobListFilter
	videos          []repo.Video
	videosErr       error
	lastVideoFilter repo.VideoListFilter
	video           repo.Video
	videoErr        error
	system          dashboard.SystemStatus
	systemErr       error
	storage         dashboard.StorageStats
	storageErr      error
}

func (s *stubDashboardService) ListJobs(_ context.Context, filter repo.JobListFilter) ([]repo.Job, error) {
	s.lastJobFilter = filter
	if s.jobsErr != nil {
		return nil, s.jobsErr
	}
	return append([]repo.Job(nil), s.jobs...), nil
}

func (s *stubDashboardService) ListVideos(_ context.Context, filter repo.VideoListFilter) ([]repo.Video, error) {
	s.lastVideoFilter = filter
	if s.videosErr != nil {
		return nil, s.videosErr
	}
	return append([]repo.Video(nil), s.videos...), nil
}

func (s *stubDashboardService) GetVideo(context.Context, int64) (repo.Video, error) {
	return s.video, s.videoErr
}

func (s *stubDashboardService) GetSystemStatus(context.Context) (dashboard.SystemStatus, error) {
	return s.system, s.systemErr
}

func (s *stubDashboardService) GetStorageStats(context.Context) (dashboard.StorageStats, error) {
	return s.storage, s.storageErr
}

func newTestRouter(creatorSvc CreatorService, jobSvc JobService, dashboardSvc DashboardService) http.Handler {
	return NewRouter(creatorSvc, jobSvc, dashboardSvc)
}

func TestHealthz(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("expected body ok, got %s", w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ready" {
		t.Fatalf("expected body ready, got %s", w.Body.String())
	}
}

func TestNotFound(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateCreator(t *testing.T) {
	service := &stubCreatorService{
		result: repo.Creator{ID: 1, UID: "123", Name: "name", Platform: "bilibili", Status: "active"},
	}
	r := newTestRouter(service, nil, nil)

	body := map[string]string{"uid": "123", "name": "name"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if service.last.UID != "123" {
		t.Fatalf("expected service to be called")
	}
}

func TestCreateCreatorMissingService(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader([]byte(`{"uid":"1"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestCreateCreatorBadJSON(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateCreatorMissingFields(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateCreatorMethodNotAllowed(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPut, "/creators", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestListCreators(t *testing.T) {
	service := &stubCreatorService{
		list: []repo.Creator{
			{ID: 1, UID: "1", Name: "one", Platform: "bilibili", Status: "active"},
		},
	}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/creators?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var payload map[string][]createCreatorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json error: %v", err)
	}
	if len(payload["items"]) != 1 {
		t.Fatalf("expected 1 item")
	}
}

func TestListCreatorsBadLimit(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/creators?limit=bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListCreatorsMissingService(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/creators", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestListCreatorsServiceError(t *testing.T) {
	service := &stubCreatorService{listErr: context.DeadlineExceeded}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/creators", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateJob(t *testing.T) {
	jobSvc := &stubJobService{}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte(`{"type":"fetch"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if jobSvc.fetchCalled != 1 {
		t.Fatalf("expected enqueue fetch called")
	}
}

func TestCreateJobBadJSON(t *testing.T) {
	jobSvc := &stubJobService{}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateJobMissingType(t *testing.T) {
	jobSvc := &stubJobService{}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateJobBadType(t *testing.T) {
	jobSvc := &stubJobService{}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte(`{"type":"bad"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateJobMethodNotAllowed(t *testing.T) {
	jobSvc := &stubJobService{}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPut, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestCreateJobServiceError(t *testing.T) {
	jobSvc := &stubJobService{err: context.DeadlineExceeded}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte(`{"type":"fetch"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateJobMissingService(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte(`{"type":"fetch"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestListJobs(t *testing.T) {
	createdAt := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	dashboardSvc := &stubDashboardService{
		jobs: []repo.Job{
			{
				ID:        11,
				Type:      "fetch",
				Status:    "queued",
				Payload:   map[string]any{"scope": "all"},
				CreatedAt: createdAt,
			},
		},
	}

	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/jobs?limit=5&status=queued&type=fetch", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if dashboardSvc.lastJobFilter.Limit != 5 || dashboardSvc.lastJobFilter.Status != "queued" || dashboardSvc.lastJobFilter.Type != "fetch" {
		t.Fatalf("unexpected filter: %+v", dashboardSvc.lastJobFilter)
	}

	var payload listJobsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json error: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item")
	}
	if payload.Items[0].CreatedAt != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected created_at: %s", payload.Items[0].CreatedAt)
	}
}

func TestListJobsBadLimit(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{})
	req := httptest.NewRequest(http.MethodGet, "/jobs?limit=bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListJobsMissingDashboard(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestListVideos(t *testing.T) {
	publishTime := time.Date(2026, 4, 12, 12, 30, 0, 0, time.UTC)
	dashboardSvc := &stubDashboardService{
		videos: []repo.Video{
			{
				ID:            21,
				Platform:      "bilibili",
				VideoID:       "BV1xx411c7mD",
				CreatorID:     9,
				Title:         "稀有投稿",
				ViewCount:     1000,
				FavoriteCount: 88,
				State:         "OUT_OF_PRINT",
				PublishTime:   publishTime,
			},
		},
	}

	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/videos?limit=3&creator_id=9&state=OUT_OF_PRINT", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if dashboardSvc.lastVideoFilter.Limit != 3 || dashboardSvc.lastVideoFilter.CreatorID != 9 || dashboardSvc.lastVideoFilter.State != "OUT_OF_PRINT" {
		t.Fatalf("unexpected filter: %+v", dashboardSvc.lastVideoFilter)
	}

	var payload listVideosResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json error: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item")
	}
	if payload.Items[0].PublishTime != publishTime.Format(time.RFC3339) {
		t.Fatalf("unexpected publish_time: %s", payload.Items[0].PublishTime)
	}
}

func TestListVideosBadCreatorID(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{})
	req := httptest.NewRequest(http.MethodGet, "/videos?creator_id=bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListVideosBadLimit(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{})
	req := httptest.NewRequest(http.MethodGet, "/videos?limit=bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListVideosServiceError(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{videosErr: errors.New("boom")})
	req := httptest.NewRequest(http.MethodGet, "/videos", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestListVideosMethodNotAllowed(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{})
	req := httptest.NewRequest(http.MethodPost, "/videos", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestListVideosMissingDashboard(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/videos", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestPatchCreator(t *testing.T) {
	service := &stubCreatorService{
		result: repo.Creator{ID: 7, UID: "7", Name: "new", Platform: "bilibili", Status: "paused"},
	}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/creators/7", bytes.NewReader([]byte(`{"name":"new","status":"paused"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if service.patchID != 7 {
		t.Fatalf("expected patch id 7, got %d", service.patchID)
	}
}

func TestPatchCreatorMissingFields(t *testing.T) {
	service := &stubCreatorService{err: creator.ErrInvalidPatch}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/creators/7", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPatchCreatorBadJSON(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/creators/7", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPatchCreatorNotFound(t *testing.T) {
	service := &stubCreatorService{err: sql.ErrNoRows}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/creators/7", bytes.NewReader([]byte(`{"name":"new"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPatchCreatorInvalidID(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodPatch, "/creators/bad", bytes.NewReader([]byte(`{"name":"new"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPatchCreatorMethodNotAllowed(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/creators/7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestGetVideoByID(t *testing.T) {
	dashboardSvc := &stubDashboardService{
		video: repo.Video{ID: 21, VideoID: "BV1xx411c7mD", Title: "稀有投稿", State: "OUT_OF_PRINT"},
	}
	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/videos/21", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var payload videoListItem
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json error: %v", err)
	}
	if payload.ID != 21 || payload.VideoID != "BV1xx411c7mD" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestGetVideoByIDNotFound(t *testing.T) {
	dashboardSvc := &stubDashboardService{videoErr: sql.ErrNoRows}
	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/videos/21", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetVideoByIDInternalError(t *testing.T) {
	dashboardSvc := &stubDashboardService{videoErr: errors.New("boom")}
	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/videos/21", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestGetVideoByIDMissingDashboard(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/videos/21", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestVideoItemBadPathID(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{})
	req := httptest.NewRequest(http.MethodGet, "/videos/bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestVideoItemMethodNotAllowed(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{})
	req := httptest.NewRequest(http.MethodDelete, "/videos/21", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestCreateVideoDownloadJob(t *testing.T) {
	jobSvc := &stubJobService{}
	dashboardSvc := &stubDashboardService{video: repo.Video{ID: 21, VideoID: "BV1"}}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/21/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if jobSvc.downloadID != 21 {
		t.Fatalf("expected download id 21, got %d", jobSvc.downloadID)
	}
}

func TestCreateVideoDownloadJobMissingService(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{video: repo.Video{ID: 21}})
	req := httptest.NewRequest(http.MethodPost, "/videos/21/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestCreateVideoDownloadJobNotFound(t *testing.T) {
	jobSvc := &stubJobService{}
	dashboardSvc := &stubDashboardService{videoErr: sql.ErrNoRows}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/21/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateVideoDownloadJobLookupError(t *testing.T) {
	jobSvc := &stubJobService{}
	dashboardSvc := &stubDashboardService{videoErr: errors.New("boom")}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/21/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateVideoDownloadJobEnqueueError(t *testing.T) {
	jobSvc := &stubJobService{err: errors.New("boom")}
	dashboardSvc := &stubDashboardService{video: repo.Video{ID: 21, VideoID: "BV1"}}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/21/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateVideoCheckJob(t *testing.T) {
	jobSvc := &stubJobService{}
	dashboardSvc := &stubDashboardService{video: repo.Video{ID: 22, VideoID: "BV2"}}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/22/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if jobSvc.checkVideoID != 22 {
		t.Fatalf("expected check video id 22, got %d", jobSvc.checkVideoID)
	}
}

func TestCreateVideoCheckJobMissingService(t *testing.T) {
	r := newTestRouter(nil, nil, &stubDashboardService{video: repo.Video{ID: 22}})
	req := httptest.NewRequest(http.MethodPost, "/videos/22/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestCreateVideoCheckJobNotFound(t *testing.T) {
	jobSvc := &stubJobService{}
	dashboardSvc := &stubDashboardService{videoErr: sql.ErrNoRows}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/22/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateVideoCheckJobLookupError(t *testing.T) {
	jobSvc := &stubJobService{}
	dashboardSvc := &stubDashboardService{videoErr: errors.New("boom")}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/22/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateVideoCheckJobEnqueueError(t *testing.T) {
	jobSvc := &stubJobService{err: errors.New("boom")}
	dashboardSvc := &stubDashboardService{video: repo.Video{ID: 22, VideoID: "BV2"}}
	r := newTestRouter(nil, jobSvc, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/videos/22/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateStorageCleanupJob(t *testing.T) {
	jobSvc := &stubJobService{}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/storage/cleanup", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if jobSvc.cleanupCalled != 1 {
		t.Fatalf("expected cleanup called")
	}
}

func TestCreateStorageCleanupJobMissingService(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/storage/cleanup", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestCreateStorageCleanupJobServiceError(t *testing.T) {
	jobSvc := &stubJobService{err: errors.New("boom")}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/storage/cleanup", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateStorageCleanupJobMethodNotAllowed(t *testing.T) {
	jobSvc := &stubJobService{}
	r := newTestRouter(nil, jobSvc, nil)
	req := httptest.NewRequest(http.MethodGet, "/storage/cleanup", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestSystemStatus(t *testing.T) {
	dashboardSvc := &stubDashboardService{
		system: dashboard.SystemStatus{
			Health:      "online",
			AuthEnabled: true,
			Cookie: dashboard.CookieStatus{
				Configured:       true,
				Status:           "valid",
				Source:           "cookie_file",
				LastCheckResult:  "valid",
				LastReloadResult: "success",
				LastError:        "上次刷新失败",
				LastCheckAt:      time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
				LastReloadAt:     time.Date(2026, 4, 13, 11, 50, 0, 0, time.UTC),
			},
			RiskLevel: "高",
			Risk: dashboard.RiskStatus{
				Level:          "高",
				Active:         true,
				BackoffSeconds: 18,
				BackoffUntil:   time.Date(2026, 4, 13, 12, 1, 0, 0, time.UTC),
				LastHitAt:      time.Date(2026, 4, 13, 11, 59, 42, 0, time.UTC),
				LastReason:     "/x/web-interface/nav 返回风控码 -412",
			},
			Limits: config.LimitsConfig{
				GlobalQPS:           2,
				PerCreatorQPS:       1,
				DownloadConcurrency: 4,
				CheckConcurrency:    8,
			},
			Scheduler: config.SchedulerConfig{
				FetchInterval:   45 * time.Minute,
				CheckInterval:   24 * time.Hour,
				CleanupInterval: 24 * time.Hour,
				CheckStableDays: 30,
			},
		},
	}

	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/system/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var payload systemStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json error: %v", err)
	}
	if payload.Health != "online" || payload.Cookie.Status != "valid" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if !payload.AuthEnabled || payload.Cookie.Source != "cookie_file" {
		t.Fatalf("unexpected auth payload: %+v", payload)
	}
	if payload.Risk.Level != "高" || payload.Risk.BackoffSeconds != 18 {
		t.Fatalf("unexpected risk payload: %+v", payload)
	}
	if payload.Limits.GlobalQPS != 2 || payload.Scheduler.CheckStableDays != 30 {
		t.Fatalf("unexpected nested payload: %+v", payload)
	}
}

func TestSystemStatusServiceError(t *testing.T) {
	dashboardSvc := &stubDashboardService{systemErr: errors.New("boom")}
	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/system/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSystemStatusMethodNotAllowed(t *testing.T) {
	dashboardSvc := &stubDashboardService{}
	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/system/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestSystemStatusMissingDashboard(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/system/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestStorageStats(t *testing.T) {
	dashboardSvc := &stubDashboardService{
		storage: dashboard.StorageStats{
			RootDir:      "/data/archive",
			UsedBytes:    123,
			UsagePercent: 12,
		},
	}

	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/storage/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var payload dashboard.StorageStats
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json error: %v", err)
	}
	if payload.RootDir != "/data/archive" || payload.UsedBytes != 123 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestStorageStatsServiceError(t *testing.T) {
	dashboardSvc := &stubDashboardService{storageErr: errors.New("boom")}
	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodGet, "/storage/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestStorageStatsMethodNotAllowed(t *testing.T) {
	dashboardSvc := &stubDashboardService{}
	r := newTestRouter(nil, nil, dashboardSvc)
	req := httptest.NewRequest(http.MethodPost, "/storage/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestStorageStatsMissingDashboard(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/storage/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	r := newTestRouter(nil, nil, nil)
	req := httptest.NewRequest(http.MethodOptions, "/creators", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Fatalf("expected CORS header")
	}
}

func TestParsePathID(t *testing.T) {
	t.Run("prefix mismatch", func(t *testing.T) {
		if _, _, err := parsePathID("/jobs/1", "/videos/"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing id", func(t *testing.T) {
		if _, _, err := parsePathID("/videos/", "/videos/"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		if _, _, err := parsePathID("/videos/abc", "/videos/"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("success with tail", func(t *testing.T) {
		id, tail, err := parsePathID("/videos/11/check", "/videos/")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 11 || tail != "check" {
			t.Fatalf("unexpected parsed values: id=%d tail=%q", id, tail)
		}
	})
}
