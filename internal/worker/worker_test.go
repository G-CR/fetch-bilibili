package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

type updateRecord struct {
	id     int64
	status string
	errMsg string
}

type stubRepo struct {
	job       repo.Job
	fetched   bool
	fetchErr  error
	updateErr error
	updates   chan updateRecord
	mu        sync.Mutex
}

func (s *stubRepo) Enqueue(ctx context.Context, job repo.Job) (int64, error) {
	return 0, nil
}

func (s *stubRepo) FetchQueued(ctx context.Context, limit int) ([]repo.Job, error) {
	if s.fetchErr != nil {
		return nil, s.fetchErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fetched {
		return nil, nil
	}
	s.fetched = true
	return []repo.Job{s.job}, nil
}

func (s *stubRepo) UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error {
	if s.updates != nil {
		s.updates <- updateRecord{id: id, status: status, errMsg: errMsg}
	}
	return s.updateErr
}

func (s *stubRepo) ListRecent(ctx context.Context, filter repo.JobListFilter) ([]repo.Job, error) {
	return nil, nil
}

func (s *stubRepo) CountByStatuses(ctx context.Context, statuses []string) (int64, error) {
	return 0, nil
}

type stubHandler struct {
	err error
}

func (h *stubHandler) Handle(ctx context.Context, job repo.Job) error {
	return h.err
}

type stubEventPublisher struct {
	events chan live.Event
}

func (s *stubEventPublisher) Publish(evt live.Event) {
	s.events <- evt
}

func waitEvent(t *testing.T, ch <-chan live.Event, timeout time.Duration) live.Event {
	t.Helper()

	select {
	case evt := <-ch:
		return evt
	case <-time.After(timeout):
		t.Fatal("timeout waiting for event")
		return live.Event{}
	}
}

func TestWorkerSuccess(t *testing.T) {
	repo := &stubRepo{job: repo.Job{ID: 1}, updates: make(chan updateRecord, 1)}
	handler := &stubHandler{}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	select {
	case upd := <-repo.updates:
		if upd.status != "success" {
			t.Fatalf("expected success, got %s", upd.status)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for update")
	}

	pool.Wait()
}

func TestWorkerFailure(t *testing.T) {
	repo := &stubRepo{job: repo.Job{ID: 2}, updates: make(chan updateRecord, 1)}
	handler := &stubHandler{err: errors.New("boom")}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	select {
	case upd := <-repo.updates:
		if upd.status != "failed" {
			t.Fatalf("expected failed, got %s", upd.status)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for update")
	}

	pool.Wait()
}

func TestWorkerFetchError(t *testing.T) {
	repo := &stubRepo{fetchErr: errors.New("fetch error")}
	handler := &stubHandler{}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	pool.Start(ctx)
	pool.Wait()
}

func TestWorkerUpdateStatusError(t *testing.T) {
	repo := &stubRepo{job: repo.Job{ID: 3}, updateErr: errors.New("update error")}
	handler := &stubHandler{}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	pool.Start(ctx)
	pool.Wait()
}

func TestWorkerDefaults(t *testing.T) {
	repo := &stubRepo{}
	handler := &stubHandler{}
	pool := New(repo, handler, 0, 0, nil, nil)
	if pool.workers <= 0 {
		t.Fatalf("expected default workers")
	}
}

func TestWorkerSuccessPublishesJobEvent(t *testing.T) {
	job := repo.Job{
		ID:      10,
		Type:    jobs.TypeFetch,
		Status:  jobs.StatusRunning,
		Payload: map[string]any{"key": "value"},
	}
	repo := &stubRepo{job: job, updates: make(chan updateRecord, 1)}
	handler := &stubHandler{}
	publisher := &stubEventPublisher{events: make(chan live.Event, 2)}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil, publisher)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	running := waitEvent(t, publisher.events, 80*time.Millisecond)
	success := waitEvent(t, publisher.events, 80*time.Millisecond)

	if payload, ok := running.Payload.(map[string]any); !ok {
		t.Fatalf("expected running payload map, got %T", running.Payload)
	} else {
		if payload["id"] != int64(10) || payload["status"] != jobs.StatusRunning {
			t.Fatalf("unexpected running payload: %#v", payload)
		}
	}

	if payload, ok := success.Payload.(map[string]any); !ok {
		t.Fatalf("expected success payload map, got %T", success.Payload)
	} else {
		if payload["id"] != int64(10) || payload["status"] != jobs.StatusSuccess {
			t.Fatalf("unexpected success payload: %#v", payload)
		}
	}

	pool.Wait()
}

func TestWorkerFailurePublishesJobEvent(t *testing.T) {
	job := repo.Job{
		ID:     20,
		Type:   jobs.TypeCheck,
		Status: jobs.StatusRunning,
	}
	repo := &stubRepo{job: job, updates: make(chan updateRecord, 1)}
	handler := &stubHandler{err: errors.New("boom")}
	publisher := &stubEventPublisher{events: make(chan live.Event, 2)}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil, publisher)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	_ = waitEvent(t, publisher.events, 80*time.Millisecond)
	failed := waitEvent(t, publisher.events, 80*time.Millisecond)
	if payload, ok := failed.Payload.(map[string]any); !ok {
		t.Fatalf("expected failed payload map, got %T", failed.Payload)
	} else {
		if payload["status"] != jobs.StatusFailed {
			t.Fatalf("expected failed status, got %#v", payload["status"])
		}
		if payload["error_msg"] != "boom" {
			t.Fatalf("expected error_msg boom, got %#v", payload["error_msg"])
		}
	}

	pool.Wait()
}

func TestWorkerRunningPublishesJobEventWithFreshUpdatedAt(t *testing.T) {
	oldUpdatedAt := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	freshUpdatedAt := time.Date(2026, 4, 15, 10, 0, 5, 0, time.UTC)
	job := repo.Job{
		ID:        30,
		Type:      jobs.TypeFetch,
		Status:    jobs.StatusRunning,
		UpdatedAt: oldUpdatedAt,
	}
	repo := &stubRepo{job: job, updates: make(chan updateRecord, 1)}
	handler := &stubHandler{}
	publisher := &stubEventPublisher{events: make(chan live.Event, 2)}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil, publisher)
	pool.now = func() time.Time { return freshUpdatedAt }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	pool.Start(ctx)

	running := waitEvent(t, publisher.events, 80*time.Millisecond)
	payload, ok := running.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected payload map, got %T", running.Payload)
	}
	gotUpdatedAt, ok := payload["updated_at"].(time.Time)
	if !ok {
		t.Fatalf("expected updated_at time.Time, got %T", payload["updated_at"])
	}
	if gotUpdatedAt.Equal(oldUpdatedAt) {
		t.Fatalf("expected running updated_at not old time, got %v", gotUpdatedAt)
	}
	if !gotUpdatedAt.Equal(freshUpdatedAt) {
		t.Fatalf("expected running updated_at %v, got %v", freshUpdatedAt, gotUpdatedAt)
	}

	pool.Wait()
}
