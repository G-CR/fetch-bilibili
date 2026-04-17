package scheduler

import (
	"context"
	"errors"
	"log"
	"time"

	"fetch-bilibili/internal/config"
)

type JobService interface {
	EnqueueFetch(ctx context.Context) error
	EnqueueCheck(ctx context.Context) error
	EnqueueCleanup(ctx context.Context) error
	EnqueueDiscover(ctx context.Context) error
}

type Scheduler struct {
	cfg          config.SchedulerConfig
	discoveryCfg config.DiscoveryConfig
	jobs         JobService
	logger       *log.Logger
}

func New(cfg config.SchedulerConfig, discoveryCfg config.DiscoveryConfig, jobs JobService, logger *log.Logger) (*Scheduler, error) {
	if cfg.FetchInterval <= 0 || cfg.CheckInterval <= 0 || cfg.CleanupInterval <= 0 {
		return nil, errors.New("调度间隔必须大于 0")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{cfg: cfg, discoveryCfg: discoveryCfg, jobs: jobs, logger: logger}, nil
}

func (s *Scheduler) Start(ctx context.Context) {
	if s.jobs == nil {
		s.logger.Print("调度器未启用：未配置任务服务")
		return
	}

	fetchTicker := time.NewTicker(s.cfg.FetchInterval)
	checkTicker := time.NewTicker(s.cfg.CheckInterval)
	cleanupTicker := time.NewTicker(s.cfg.CleanupInterval)
	var discoverTicker *time.Ticker
	var discoverCh <-chan time.Time
	if s.discoveryCfg.Enabled && s.discoveryCfg.Interval > 0 {
		discoverTicker = time.NewTicker(s.discoveryCfg.Interval)
		discoverCh = discoverTicker.C
	}
	defer fetchTicker.Stop()
	defer checkTicker.Stop()
	defer cleanupTicker.Stop()
	if discoverTicker != nil {
		defer discoverTicker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-fetchTicker.C:
			if err := s.jobs.EnqueueFetch(ctx); err != nil {
				s.logger.Printf("调度拉取任务失败: %v", err)
			}
		case <-checkTicker.C:
			if err := s.jobs.EnqueueCheck(ctx); err != nil {
				s.logger.Printf("调度检查任务失败: %v", err)
			}
		case <-cleanupTicker.C:
			if err := s.jobs.EnqueueCleanup(ctx); err != nil {
				s.logger.Printf("调度清理任务失败: %v", err)
			}
		case <-discoverCh:
			if err := s.jobs.EnqueueDiscover(ctx); err != nil {
				s.logger.Printf("调度发现任务失败: %v", err)
			}
		}
	}
}

type NoopJobService struct{}

func (s *NoopJobService) EnqueueFetch(ctx context.Context) error    { return nil }
func (s *NoopJobService) EnqueueCheck(ctx context.Context) error    { return nil }
func (s *NoopJobService) EnqueueCleanup(ctx context.Context) error  { return nil }
func (s *NoopJobService) EnqueueDiscover(ctx context.Context) error { return nil }
