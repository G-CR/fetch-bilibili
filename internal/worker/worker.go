package worker

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/repo"
)

type Handler interface {
	Handle(ctx context.Context, job repo.Job) error
}

type WorkerPool struct {
	repo      repo.JobRepository
	handler   Handler
	workers   int
	pollEvery time.Duration
	logger    *log.Logger

	wg sync.WaitGroup
}

func New(repo repo.JobRepository, handler Handler, workers int, pollEvery time.Duration, logger *log.Logger) *WorkerPool {
	if workers <= 0 {
		workers = 2
	}
	if pollEvery <= 0 {
		pollEvery = 2 * time.Second
	}
	if logger == nil {
		logger = log.Default()
	}

	return &WorkerPool{
		repo:      repo,
		handler:   handler,
		workers:   workers,
		pollEvery: pollEvery,
		logger:    logger,
	}
}

func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func(id int) {
			defer p.wg.Done()
			p.loop(ctx, id)
		}(i + 1)
	}
}

func (p *WorkerPool) Wait() {
	p.wg.Wait()
}

func (p *WorkerPool) loop(ctx context.Context, id int) {
	ticker := time.NewTicker(p.pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.consumeOnce(ctx, id)
		}
	}
}

func (p *WorkerPool) consumeOnce(ctx context.Context, id int) {
	jobsList, err := p.repo.FetchQueued(ctx, 1)
	if err != nil {
		p.logger.Printf("工作线程 %d 拉取任务失败: %v", id, err)
		return
	}
	if len(jobsList) == 0 {
		return
	}

	job := jobsList[0]
	err = p.handler.Handle(ctx, job)
	if err == nil {
		if err := p.repo.UpdateStatus(ctx, job.ID, jobs.StatusSuccess, ""); err != nil {
			p.logger.Printf("工作线程 %d 更新任务 %d 为成功失败: %v", id, job.ID, err)
		}
		return
	}

	msg := err.Error()
	if errors.Is(err, context.Canceled) {
		msg = "context canceled"
	}
	if err := p.repo.UpdateStatus(ctx, job.ID, jobs.StatusFailed, msg); err != nil {
		p.logger.Printf("工作线程 %d 更新任务 %d 为失败失败: %v", id, job.ID, err)
	}
}
