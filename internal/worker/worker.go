package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

type Handler interface {
	Handle(ctx context.Context, job repo.Job) error
}

type eventPublisher interface {
	Publish(evt live.Event)
}

type WorkerPool struct {
	repo      repo.JobRepository
	handler   Handler
	workers   int
	pollEvery time.Duration
	logger    *log.Logger
	publisher eventPublisher
	now       func() time.Time

	wg sync.WaitGroup
}

func New(repo repo.JobRepository, handler Handler, workers int, pollEvery time.Duration, logger *log.Logger, publisher eventPublisher) *WorkerPool {
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
		publisher: publisher,
		now:       time.Now,
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
	job.UpdatedAt = p.now()
	if job.StartedAt.IsZero() {
		job.StartedAt = job.UpdatedAt
	}
	p.publishJobChanged(job)

	err = p.handler.Handle(ctx, job)
	if err == nil {
		if err := p.repo.UpdateStatus(ctx, job.ID, jobs.StatusSuccess, ""); err != nil {
			p.logger.Printf("工作线程 %d 更新任务 %d 为成功失败: %v", id, job.ID, err)
			return
		}
		job.Status = jobs.StatusSuccess
		job.ErrorMsg = ""
		job.UpdatedAt = p.now()
		job.FinishedAt = job.UpdatedAt
		p.publishJobChanged(job)
		return
	}

	msg := err.Error()
	if errors.Is(err, context.Canceled) {
		msg = "context canceled"
	}
	if err := p.repo.UpdateStatus(ctx, job.ID, jobs.StatusFailed, msg); err != nil {
		p.logger.Printf("工作线程 %d 更新任务 %d 为失败失败: %v", id, job.ID, err)
		return
	}
	job.Status = jobs.StatusFailed
	job.ErrorMsg = msg
	job.UpdatedAt = p.now()
	job.FinishedAt = job.UpdatedAt
	p.publishJobChanged(job)
}

func (p *WorkerPool) publishJobChanged(job repo.Job) {
	if p.publisher == nil {
		return
	}
	updatedAt := job.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = p.now()
	}
	payload := map[string]any{
		"id":          job.ID,
		"type":        job.Type,
		"status":      job.Status,
		"payload":     job.Payload,
		"error_msg":   job.ErrorMsg,
		"not_before":  formatEventTime(job.NotBefore),
		"started_at":  formatEventTime(job.StartedAt),
		"finished_at": formatEventTime(job.FinishedAt),
		"created_at":  formatEventTime(job.CreatedAt),
		"updated_at":  formatEventTime(updatedAt),
	}
	p.publisher.Publish(live.Event{
		ID:      fmt.Sprintf("job-%d-%d", job.ID, updatedAt.UnixNano()),
		Type:    "job.changed",
		At:      updatedAt,
		Payload: payload,
	})
}

func formatEventTime(v time.Time) string {
	if v.IsZero() {
		return ""
	}
	return v.Format(time.RFC3339)
}
