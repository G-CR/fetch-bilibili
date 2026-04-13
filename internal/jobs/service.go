package jobs

import (
	"context"
	"errors"

	"fetch-bilibili/internal/repo"
)

type Service struct {
	repo repo.JobRepository
}

func NewService(repo repo.JobRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) EnqueueFetch(ctx context.Context) error {
	_, err := s.repo.Enqueue(ctx, repo.Job{Type: TypeFetch, Status: StatusQueued})
	if errors.Is(err, ErrJobAlreadyActive) {
		return nil
	}
	return err
}

func (s *Service) EnqueueCheck(ctx context.Context) error {
	_, err := s.repo.Enqueue(ctx, repo.Job{Type: TypeCheck, Status: StatusQueued})
	if errors.Is(err, ErrJobAlreadyActive) {
		return nil
	}
	return err
}

func (s *Service) EnqueueCleanup(ctx context.Context) error {
	_, err := s.repo.Enqueue(ctx, repo.Job{Type: TypeCleanup, Status: StatusQueued})
	if errors.Is(err, ErrJobAlreadyActive) {
		return nil
	}
	return err
}

func (s *Service) EnqueueDownload(ctx context.Context, videoID int64) error {
	_, err := s.repo.Enqueue(ctx, repo.Job{
		Type:   TypeDownload,
		Status: StatusQueued,
		Payload: map[string]any{
			"video_id": videoID,
		},
	})
	if errors.Is(err, ErrJobAlreadyActive) {
		return nil
	}
	return err
}

func (s *Service) EnqueueCheckVideo(ctx context.Context, videoID int64) error {
	_, err := s.repo.Enqueue(ctx, repo.Job{
		Type:   TypeCheck,
		Status: StatusQueued,
		Payload: map[string]any{
			"video_id": videoID,
		},
	})
	if errors.Is(err, ErrJobAlreadyActive) {
		return nil
	}
	return err
}
