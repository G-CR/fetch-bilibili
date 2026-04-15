package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

type EventPublisher interface {
	Publish(evt live.Event)
}

type Service struct {
	repo      repo.JobRepository
	publisher EventPublisher
	now       func() time.Time
}

func NewService(repo repo.JobRepository, publisher EventPublisher) *Service {
	return &Service{
		repo:      repo,
		publisher: publisher,
		now:       time.Now,
	}
}

func (s *Service) EnqueueFetch(ctx context.Context) error {
	return s.enqueue(ctx, repo.Job{Type: TypeFetch, Status: StatusQueued})
}

func (s *Service) EnqueueCheck(ctx context.Context) error {
	return s.enqueue(ctx, repo.Job{Type: TypeCheck, Status: StatusQueued})
}

func (s *Service) EnqueueCleanup(ctx context.Context) error {
	return s.enqueue(ctx, repo.Job{Type: TypeCleanup, Status: StatusQueued})
}

func (s *Service) EnqueueDownload(ctx context.Context, videoID int64) error {
	return s.enqueue(ctx, repo.Job{
		Type:   TypeDownload,
		Status: StatusQueued,
		Payload: map[string]any{
			"video_id": videoID,
		},
	})
}

func (s *Service) EnqueueCheckVideo(ctx context.Context, videoID int64) error {
	return s.enqueue(ctx, repo.Job{
		Type:   TypeCheck,
		Status: StatusQueued,
		Payload: map[string]any{
			"video_id": videoID,
		},
	})
}

func (s *Service) enqueue(ctx context.Context, job repo.Job) error {
	id, err := s.repo.Enqueue(ctx, job)
	if errors.Is(err, ErrJobAlreadyActive) {
		return nil
	}
	if err != nil {
		return err
	}
	job.ID = id
	job.UpdatedAt = s.now()
	s.publishJobChanged(job)
	return nil
}

func (s *Service) publishJobChanged(job repo.Job) {
	if s.publisher == nil {
		return
	}
	updatedAt := job.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = s.now()
	}

	s.publisher.Publish(live.Event{
		ID:   fmt.Sprintf("job-%d-%d", job.ID, updatedAt.UnixNano()),
		Type: "job.changed",
		At:   updatedAt,
		Payload: map[string]any{
			"id":         job.ID,
			"type":       job.Type,
			"status":     job.Status,
			"payload":    job.Payload,
			"updated_at": updatedAt,
		},
	})
}
