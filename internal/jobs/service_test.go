package jobs

import (
	"context"
	"errors"
	"testing"

	"fetch-bilibili/internal/repo"
)

type stubJobRepo struct {
	last repo.Job
	err  error
}

func (s *stubJobRepo) Enqueue(ctx context.Context, job repo.Job) (int64, error) {
	s.last = job
	return 1, s.err
}

func (s *stubJobRepo) FetchQueued(ctx context.Context, limit int) ([]repo.Job, error) {
	return nil, nil
}

func (s *stubJobRepo) UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error {
	return nil
}

func TestEnqueueFetch(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo)
	if err := svc.EnqueueFetch(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeFetch {
		t.Fatalf("expected type fetch")
	}
	if repo.last.Status != StatusQueued {
		t.Fatalf("expected status queued")
	}
}

func TestEnqueueCleanup(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo)
	if err := svc.EnqueueCleanup(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeCleanup {
		t.Fatalf("expected type cleanup")
	}
}

func TestEnqueueCheck(t *testing.T) {
	repo := &stubJobRepo{}
	svc := NewService(repo)
	if err := svc.EnqueueCheck(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.last.Type != TypeCheck {
		t.Fatalf("expected type check")
	}
}

func TestEnqueueError(t *testing.T) {
	repo := &stubJobRepo{err: errors.New("fail")}
	svc := NewService(repo)
	if err := svc.EnqueueCheck(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}
