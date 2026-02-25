package jobs

import (
	"context"

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
	return err
}

func (s *Service) EnqueueCheck(ctx context.Context) error {
	_, err := s.repo.Enqueue(ctx, repo.Job{Type: TypeCheck, Status: StatusQueued})
	return err
}

func (s *Service) EnqueueCleanup(ctx context.Context) error {
	_, err := s.repo.Enqueue(ctx, repo.Job{Type: TypeCleanup, Status: StatusQueued})
	return err
}
