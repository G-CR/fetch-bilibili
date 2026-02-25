package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

type stubHandler struct {
	err error
}

func (h *stubHandler) Handle(ctx context.Context, job repo.Job) error {
	return h.err
}

func TestWorkerSuccess(t *testing.T) {
	repo := &stubRepo{job: repo.Job{ID: 1}, updates: make(chan updateRecord, 1)}
	handler := &stubHandler{}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil)

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
	pool := New(repo, handler, 1, 1*time.Millisecond, nil)

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
	pool := New(repo, handler, 1, 1*time.Millisecond, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	pool.Start(ctx)
	pool.Wait()
}

func TestWorkerUpdateStatusError(t *testing.T) {
	repo := &stubRepo{job: repo.Job{ID: 3}, updateErr: errors.New("update error")}
	handler := &stubHandler{}
	pool := New(repo, handler, 1, 1*time.Millisecond, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	pool.Start(ctx)
	pool.Wait()
}

func TestWorkerDefaults(t *testing.T) {
	repo := &stubRepo{}
	handler := &stubHandler{}
	pool := New(repo, handler, 0, 0, nil)
	if pool.workers <= 0 {
		t.Fatalf("expected default workers")
	}
}
