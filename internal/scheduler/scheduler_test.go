package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
)

type counterJobs struct {
	fetch   int64
	check   int64
	cleanup int64
}

func (c *counterJobs) EnqueueFetch(ctx context.Context) error {
	atomic.AddInt64(&c.fetch, 1)
	return nil
}

func (c *counterJobs) EnqueueCheck(ctx context.Context) error {
	atomic.AddInt64(&c.check, 1)
	return nil
}

func (c *counterJobs) EnqueueCleanup(ctx context.Context) error {
	atomic.AddInt64(&c.cleanup, 1)
	return nil
}

func TestNewInvalidIntervals(t *testing.T) {
	cfg := config.SchedulerConfig{FetchInterval: 0, CheckInterval: time.Second, CleanupInterval: time.Second}
	if _, err := New(cfg, &NoopJobService{}, nil); err == nil {
		t.Fatalf("expected error for invalid fetch interval")
	}
}

func TestStartWithNilJobs(t *testing.T) {
	cfg := config.SchedulerConfig{FetchInterval: 10 * time.Millisecond, CheckInterval: 10 * time.Millisecond, CleanupInterval: 10 * time.Millisecond}
	s, err := New(cfg, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Start(ctx)
}

func TestStartTriggersJobs(t *testing.T) {
	cfg := config.SchedulerConfig{FetchInterval: 5 * time.Millisecond, CheckInterval: 5 * time.Millisecond, CleanupInterval: 5 * time.Millisecond}
	jobs := &counterJobs{}
	s, err := New(cfg, jobs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	go s.Start(ctx)
	<-ctx.Done()

	if atomic.LoadInt64(&jobs.fetch) == 0 {
		t.Fatalf("expected fetch jobs to be scheduled")
	}
	if atomic.LoadInt64(&jobs.check) == 0 {
		t.Fatalf("expected check jobs to be scheduled")
	}
	if atomic.LoadInt64(&jobs.cleanup) == 0 {
		t.Fatalf("expected cleanup jobs to be scheduled")
	}
}

type errJobs struct {
	count int64
}

func (e *errJobs) EnqueueFetch(ctx context.Context) error {
	atomic.AddInt64(&e.count, 1)
	return errors.New("fetch error")
}

func (e *errJobs) EnqueueCheck(ctx context.Context) error {
	atomic.AddInt64(&e.count, 1)
	return errors.New("check error")
}

func (e *errJobs) EnqueueCleanup(ctx context.Context) error {
	atomic.AddInt64(&e.count, 1)
	return errors.New("cleanup error")
}

func TestStartLogsErrors(t *testing.T) {
	cfg := config.SchedulerConfig{FetchInterval: 5 * time.Millisecond, CheckInterval: 5 * time.Millisecond, CleanupInterval: 5 * time.Millisecond}
	jobs := &errJobs{}
	s, err := New(cfg, jobs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	go s.Start(ctx)
	<-ctx.Done()

	if atomic.LoadInt64(&jobs.count) == 0 {
		t.Fatalf("expected error jobs to be scheduled")
	}
}
