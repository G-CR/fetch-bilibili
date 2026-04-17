package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/dashboard"
	"fetch-bilibili/internal/discovery"
	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

type stubCreatorService struct {
	last     creator.Entry
	result   repo.Creator
	err      error
	list     []repo.Creator
	listErr  error
	patchID  int64
	patch    creator.Patch
	deleteID int64
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

func (s *stubCreatorService) Delete(ctx context.Context, id int64) error {
	s.deleteID = id
	return s.err
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

type stubConfigService struct {
	doc      ConfigDocument
	loadErr  error
	saved    string
	saveResp ConfigSaveResult
	saveErr  error
}

func (s *stubConfigService) Load(context.Context) (ConfigDocument, error) {
	if s.loadErr != nil {
		return ConfigDocument{}, s.loadErr
	}
	return s.doc, nil
}

func (s *stubConfigService) Save(_ context.Context, content string) (ConfigSaveResult, error) {
	s.saved = content
	if s.saveErr != nil {
		return ConfigSaveResult{}, s.saveErr
	}
	return s.saveResp, nil
}

func newTestRouter(creatorSvc CreatorService, jobSvc JobService, dashboardSvc DashboardService) http.Handler {
	return newTestRouterWithBroker(creatorSvc, jobSvc, dashboardSvc, nil, live.NewBroker())
}

func newTestRouterWithCandidate(creatorSvc CreatorService, jobSvc JobService, dashboardSvc DashboardService, candidateSvc CandidateService) http.Handler {
	return newTestRouterWithBroker(creatorSvc, jobSvc, dashboardSvc, candidateSvc, live.NewBroker())
}

func newTestRouterWithBroker(creatorSvc CreatorService, jobSvc JobService, dashboardSvc DashboardService, candidateSvc CandidateService, broker *live.Broker) http.Handler {
	return NewRouter(creatorSvc, jobSvc, dashboardSvc, nil, candidateSvc, broker)
}

type stubCandidateService struct {
	listViews    []discovery.CandidateView
	total        int64
	listFilter   repo.CandidateListFilter
	listErr      error
	detail       discovery.CandidateDetailView
	detailID     int64
	detailErr    error
	discoverCall int
	discoverErr  error
	approveID    int64
	approveErr   error
	approveResp  repo.Creator
	ignoreID     int64
	ignoreErr    error
	blockID      int64
	blockErr     error
	reviewID     int64
	reviewErr    error
}

func (s *stubCandidateService) ListCandidates(_ context.Context, filter repo.CandidateListFilter) ([]discovery.CandidateView, int64, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return append([]discovery.CandidateView(nil), s.listViews...), s.total, nil
}

func (s *stubCandidateService) GetCandidate(_ context.Context, id int64) (discovery.CandidateDetailView, error) {
	s.detailID = id
	if s.detailErr != nil {
		return discovery.CandidateDetailView{}, s.detailErr
	}
	return s.detail, nil
}

func (s *stubCandidateService) TriggerDiscover(_ context.Context) error {
	s.discoverCall++
	return s.discoverErr
}

func (s *stubCandidateService) Approve(_ context.Context, id int64) (repo.Creator, error) {
	s.approveID = id
	if s.approveErr != nil {
		return repo.Creator{}, s.approveErr
	}
	return s.approveResp, nil
}

func (s *stubCandidateService) Ignore(_ context.Context, id int64) error {
	s.ignoreID = id
	return s.ignoreErr
}

func (s *stubCandidateService) Block(_ context.Context, id int64) error {
	s.blockID = id
	return s.blockErr
}

func (s *stubCandidateService) Review(_ context.Context, id int64) error {
	s.reviewID = id
	return s.reviewErr
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

func TestEventsStreamHeadersHelloAndHeartbeat(t *testing.T) {
	previousHeartbeatInterval := heartbeatInterval
	heartbeatInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		heartbeatInterval = previousHeartbeatInterval
	})

	r := newTestRouter(nil, nil, nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/events/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected no-cache, got %q", got)
	}
	if got := resp.Header.Get("Connection"); got != "keep-alive" {
		t.Fatalf("expected keep-alive, got %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	helloMessage := readSSEMessage(t, reader)
	eventType, data := parseSSEMessage(t, helloMessage)
	if eventType != "hello" {
		t.Fatalf("expected hello event, got %q", eventType)
	}

	var helloPayload map[string]string
	if err := json.Unmarshal([]byte(data), &helloPayload); err != nil {
		t.Fatalf("unmarshal hello payload: %v", err)
	}
	if helloPayload["server_time"] == "" {
		t.Fatalf("expected hello payload to include server_time, got %q", data)
	}

	heartbeatMessage := readSSEMessage(t, reader)
	eventType, data = parseSSEMessage(t, heartbeatMessage)
	if eventType != "heartbeat" {
		t.Fatalf("expected heartbeat event, got %q", eventType)
	}

	var heartbeatPayload map[string]string
	if err := json.Unmarshal([]byte(data), &heartbeatPayload); err != nil {
		t.Fatalf("unmarshal heartbeat payload: %v", err)
	}
	if heartbeatPayload["server_time"] == "" {
		t.Fatalf("expected heartbeat payload to include server_time, got %q", data)
	}
}

func TestEventsStreamWritesBrokerPayloadAsJSON(t *testing.T) {
	previousHeartbeatInterval := heartbeatInterval
	heartbeatInterval = time.Hour
	t.Cleanup(func() {
		heartbeatInterval = previousHeartbeatInterval
	})

	broker := live.NewBroker()
	r := newTestRouterWithBroker(nil, nil, nil, nil, broker)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/events/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	_ = readSSEMessage(t, reader) // hello

	wantPayload := map[string]string{"id": "11", "status": "running"}
	broker.Publish(live.Event{
		ID:      "evt-1",
		Type:    "job.changed",
		At:      time.Now(),
		Payload: wantPayload,
	})

	msg := readSSEMessage(t, reader)
	eventType, data := parseSSEMessage(t, msg)
	if eventType != "job.changed" {
		t.Fatalf("expected event type job.changed, got %q", eventType)
	}

	var gotPayload map[string]string
	if err := json.Unmarshal([]byte(data), &gotPayload); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if gotPayload["id"] != wantPayload["id"] || gotPayload["status"] != wantPayload["status"] {
		t.Fatalf("expected payload %v, got %v", wantPayload, gotPayload)
	}
	if _, ok := gotPayload["type"]; ok {
		t.Fatalf("expected SSE data to contain payload only, got %q", data)
	}
}

func TestRouterRegistersEventsStream(t *testing.T) {
	previousHeartbeatInterval := heartbeatInterval
	heartbeatInterval = time.Hour
	t.Cleanup(func() {
		heartbeatInterval = previousHeartbeatInterval
	})

	r := newTestRouter(nil, nil, nil)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/events/stream")
	if err != nil {
		t.Fatalf("get events stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	eventType, _ := parseSSEMessage(t, readSSEMessage(t, bufio.NewReader(resp.Body)))
	if eventType != "hello" {
		t.Fatalf("expected router to serve hello event, got %q", eventType)
	}
}

func TestEventsStreamResistsServerWriteTimeout(t *testing.T) {
	previousHeartbeatInterval := heartbeatInterval
	heartbeatInterval = time.Hour
	t.Cleanup(func() {
		heartbeatInterval = previousHeartbeatInterval
	})

	broker := live.NewBroker()
	srv := httptest.NewUnstartedServer(newTestRouterWithBroker(nil, nil, nil, nil, broker))
	srv.Config.WriteTimeout = 150 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/events/stream")
	if err != nil {
		t.Fatalf("get events stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	_ = readSSEMessage(t, reader) // hello

	go func() {
		time.Sleep(250 * time.Millisecond)
		broker.Publish(live.Event{
			ID:      "evt-timeout",
			Type:    "job.changed",
			At:      time.Now(),
			Payload: map[string]any{"id": 99},
		})
	}()

	message := readSSEMessage(t, reader)
	eventType, _ := parseSSEMessage(t, message)
	if eventType != "job.changed" {
		t.Fatalf("expected event after write timeout window, got %q", eventType)
	}
}

func TestEventsStreamSubscribesBeforeHelloAndCancelsOnEarlyWriteFailure(t *testing.T) {
	subscriber := &subscribeOrderSpy{
		subscribed: make(chan struct{}),
		ctxDone:    make(chan struct{}),
	}
	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil)
	writer := &failOnFirstWriteRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		subscribed:       subscriber.subscribed,
	}

	newEventsStreamHandler(subscriber).ServeHTTP(writer, req)

	if !writer.firstWriteSawSubscription {
		t.Fatal("expected subscribe to happen before hello write")
	}

	select {
	case <-subscriber.ctxDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected subscription context to be canceled on early return")
	}
}

type subscribeOrderSpy struct {
	subscribed chan struct{}
	ctxDone    chan struct{}
}

func (s *subscribeOrderSpy) Subscribe(ctx context.Context, buffer int) <-chan live.Event {
	close(s.subscribed)

	ch := make(chan live.Event)
	go func() {
		<-ctx.Done()
		close(s.ctxDone)
		close(ch)
	}()

	return ch
}

type failOnFirstWriteRecorder struct {
	*httptest.ResponseRecorder
	subscribed                <-chan struct{}
	firstWriteSawSubscription bool
	wrote                     bool
}

func (w *failOnFirstWriteRecorder) Write(p []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
		select {
		case <-w.subscribed:
			w.firstWriteSawSubscription = true
		default:
		}
		return 0, errors.New("boom")
	}

	return w.ResponseRecorder.Write(p)
}

func readSSEMessage(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	done := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		var lines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			lines = append(lines, line)
			if line == "\n" {
				done <- strings.Join(lines, "")
				return
			}
		}
	}()

	select {
	case msg := <-done:
		return msg
	case err := <-errCh:
		t.Fatalf("read stream: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading SSE message")
	}
	return ""
}

func parseSSEMessage(t *testing.T, message string) (string, string) {
	t.Helper()

	var eventType string
	var data string

	for _, line := range strings.Split(message, "\n") {
		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}

	if eventType == "" {
		t.Fatalf("expected event line in message %q", message)
	}
	if data == "" {
		t.Fatalf("expected data line in message %q", message)
	}

	return eventType, data
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

func TestDeleteCreator(t *testing.T) {
	service := &stubCreatorService{}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/creators/7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if service.deleteID != 7 {
		t.Fatalf("expected delete id 7, got %d", service.deleteID)
	}
}

func TestDeleteCreatorNotFound(t *testing.T) {
	service := &stubCreatorService{err: repo.ErrNotFound}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/creators/7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteCreatorConflict(t *testing.T) {
	service := &stubCreatorService{err: repo.ErrConflict}
	r := newTestRouter(service, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/creators/7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
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
				Source:           "config",
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
	if !payload.AuthEnabled || payload.Cookie.Source != "config" {
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
	if got := w.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodDelete) || !strings.Contains(got, http.MethodPut) {
		t.Fatalf("expected DELETE and PUT in CORS allow methods, got %q", got)
	}
}

func TestGetSystemConfig(t *testing.T) {
	configSvc := &stubConfigService{
		doc: ConfigDocument{
			Path:    "/app/config.yaml",
			Content: "storage:\n  root_dir: /data\nmysql:\n  dsn: test\n",
		},
	}
	r := NewRouter(nil, nil, nil, configSvc, nil, live.NewBroker())
	req := httptest.NewRequest(http.MethodGet, "/system/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var payload struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Path != "/app/config.yaml" {
		t.Fatalf("unexpected path: %s", payload.Path)
	}
	if !strings.Contains(payload.Content, "root_dir") {
		t.Fatalf("unexpected content: %q", payload.Content)
	}
}

func TestPutSystemConfig(t *testing.T) {
	configSvc := &stubConfigService{
		saveResp: ConfigSaveResult{
			Changed:          true,
			RestartScheduled: true,
			Path:             "/app/config.yaml",
		},
	}
	r := NewRouter(nil, nil, nil, configSvc, nil, live.NewBroker())
	req := httptest.NewRequest(http.MethodPut, "/system/config", bytes.NewReader([]byte(`{"content":"storage:\n  root_dir: /data\nmysql:\n  dsn: test\n"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(configSvc.saved, "root_dir") {
		t.Fatalf("expected content saved")
	}

	var payload struct {
		Changed          bool   `json:"changed"`
		RestartScheduled bool   `json:"restart_scheduled"`
		Path             string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !payload.Changed || !payload.RestartScheduled {
		t.Fatalf("unexpected save response: %+v", payload)
	}
}

func TestCandidateCreatorsList(t *testing.T) {
	service := &stubCandidateService{
		listViews: []discovery.CandidateView{
			{
				Candidate: repo.CandidateCreator{ID: 7, Platform: "bilibili", UID: "123", Name: "候选 A", Status: "reviewing", Score: 88},
				Sources: []repo.CandidateCreatorSource{
					{SourceType: "keyword", SourceValue: "补档", SourceLabel: "关键词：补档", Weight: 15},
				},
			},
		},
		total: 1,
	}
	r := newTestRouterWithCandidate(nil, nil, nil, service)
	req := httptest.NewRequest(http.MethodGet, "/candidate-creators?status=reviewing&min_score=80&keyword=补档&page=2&page_size=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.listFilter.Status != "reviewing" || service.listFilter.MinScore != 80 || service.listFilter.Keyword != "补档" || service.listFilter.Page != 2 || service.listFilter.PageSize != 5 {
		t.Fatalf("unexpected filter: %+v", service.listFilter)
	}

	var payload struct {
		Items []struct {
			ID      int64 `json:"id"`
			Sources []struct {
				SourceType string `json:"source_type"`
			} `json:"sources"`
		} `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Total != 1 || payload.Page != 2 || payload.PageSize != 5 || len(payload.Items) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(payload.Items[0].Sources) != 1 || payload.Items[0].Sources[0].SourceType != "keyword" {
		t.Fatalf("unexpected item sources: %+v", payload.Items[0].Sources)
	}
}

func TestCandidateCreatorsDetail(t *testing.T) {
	service := &stubCandidateService{
		detail: discovery.CandidateDetailView{
			Candidate: repo.CandidateCreator{ID: 9, Platform: "bilibili", UID: "789", Name: "候选详情", Status: "reviewing", Score: 91},
			Sources: []repo.CandidateCreatorSource{
				{SourceType: "keyword", SourceValue: "补档", SourceLabel: "关键词：补档", Weight: 15, DetailJSON: json.RawMessage(`{"keyword":"补档"}`)},
			},
			ScoreDetails: []repo.CandidateCreatorScoreDetail{
				{FactorKey: "keyword_risk", FactorLabel: "命中高风险关键词", ScoreDelta: 15, DetailJSON: json.RawMessage(`{"keywords":["补档"]}`)},
			},
		},
	}
	r := newTestRouterWithCandidate(nil, nil, nil, service)
	req := httptest.NewRequest(http.MethodGet, "/candidate-creators/9", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.detailID != 9 {
		t.Fatalf("expected detail id 9, got %d", service.detailID)
	}

	var payload struct {
		Candidate struct {
			ID int64 `json:"id"`
		} `json:"candidate"`
		Sources []struct {
			SourceValue string          `json:"source_value"`
			DetailJSON  json.RawMessage `json:"detail_json"`
		} `json:"sources"`
		ScoreDetails []struct {
			FactorKey string `json:"factor_key"`
		} `json:"score_details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Candidate.ID != 9 || len(payload.Sources) != 1 || len(payload.ScoreDetails) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Sources[0].SourceValue != "补档" || len(payload.Sources[0].DetailJSON) == 0 {
		t.Fatalf("unexpected source payload: %+v", payload.Sources[0])
	}
}

func TestCandidateCreatorsDiscoverAndActions(t *testing.T) {
	service := &stubCandidateService{
		approveResp: repo.Creator{ID: 5, UID: "123", Name: "正式博主", Platform: "bilibili", Status: "active"},
	}
	r := newTestRouterWithCandidate(nil, nil, nil, service)

	t.Run("discover", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/candidate-creators/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		if service.discoverCall != 1 {
			t.Fatalf("expected discover called once, got %d", service.discoverCall)
		}
	})

	t.Run("approve", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/candidate-creators/5/approve", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		if service.approveID != 5 {
			t.Fatalf("expected approve id 5, got %d", service.approveID)
		}
	})

	t.Run("ignore", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/candidate-creators/6/ignore", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		if service.ignoreID != 6 {
			t.Fatalf("expected ignore id 6, got %d", service.ignoreID)
		}
	})

	t.Run("block", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/candidate-creators/7/block", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		if service.blockID != 7 {
			t.Fatalf("expected block id 7, got %d", service.blockID)
		}
	})

	t.Run("review", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/candidate-creators/8/review", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		if service.reviewID != 8 {
			t.Fatalf("expected review id 8, got %d", service.reviewID)
		}
	})
}

func TestCandidateCreatorsErrors(t *testing.T) {
	t.Run("service unavailable", func(t *testing.T) {
		r := newTestRouterWithCandidate(nil, nil, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/candidate-creators", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", w.Code)
		}
	})

	t.Run("invalid method", func(t *testing.T) {
		r := newTestRouterWithCandidate(nil, nil, nil, &stubCandidateService{})
		req := httptest.NewRequest(http.MethodDelete, "/candidate-creators", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", w.Code)
		}
	})

	t.Run("invalid query", func(t *testing.T) {
		r := newTestRouterWithCandidate(nil, nil, nil, &stubCandidateService{})
		req := httptest.NewRequest(http.MethodGet, "/candidate-creators?page=bad", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		r := newTestRouterWithCandidate(nil, nil, nil, &stubCandidateService{})
		req := httptest.NewRequest(http.MethodGet, "/candidate-creators/bad", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
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
