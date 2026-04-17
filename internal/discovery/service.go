package discovery

import (
	"context"
	"errors"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/repo"
)

type CreatorWriter interface {
	UpsertCreator(ctx context.Context, creator repo.Creator) (repo.Creator, error)
}

type FetchEnqueuer interface {
	EnqueueFetchCreator(ctx context.Context, creatorID int64) error
}

type CandidateView struct {
	Candidate repo.CandidateCreator
	Sources   []repo.CandidateCreatorSource
}

type CandidateDetailView struct {
	Candidate    repo.CandidateCreator
	Sources      []repo.CandidateCreatorSource
	ScoreDetails []repo.CandidateCreatorScoreDetail
}

type Service struct {
	candidates repo.CandidateRepository
	creators   CreatorWriter
	fetcher    FetchEnqueuer
	cfg        config.DiscoveryConfig
	now        func() time.Time
}

func NewService(candidates repo.CandidateRepository, creators CreatorWriter, fetcher FetchEnqueuer, cfg config.DiscoveryConfig) *Service {
	return &Service{
		candidates: candidates,
		creators:   creators,
		fetcher:    fetcher,
		cfg:        cfg,
		now:        time.Now,
	}
}

func (s *Service) ListCandidates(ctx context.Context, filter repo.CandidateListFilter) ([]CandidateView, int64, error) {
	if s.candidates == nil {
		return nil, 0, errors.New("候选池服务未初始化")
	}
	items, total, err := s.candidates.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	views := make([]CandidateView, 0, len(items))
	for _, item := range items {
		sources, err := s.candidates.ListSources(ctx, item.ID)
		if err != nil {
			return nil, 0, err
		}
		views = append(views, CandidateView{
			Candidate: item,
			Sources:   sources,
		})
	}
	return views, total, nil
}

func (s *Service) GetCandidate(ctx context.Context, id int64) (CandidateDetailView, error) {
	if s.candidates == nil {
		return CandidateDetailView{}, errors.New("候选池服务未初始化")
	}
	candidate, err := s.candidates.FindByID(ctx, id)
	if err != nil {
		return CandidateDetailView{}, err
	}
	sources, err := s.candidates.ListSources(ctx, id)
	if err != nil {
		return CandidateDetailView{}, err
	}
	scoreDetails, err := s.candidates.ListScoreDetails(ctx, id)
	if err != nil {
		return CandidateDetailView{}, err
	}
	return CandidateDetailView{
		Candidate:    candidate,
		Sources:      sources,
		ScoreDetails: scoreDetails,
	}, nil
}

func (s *Service) Approve(ctx context.Context, id int64) (repo.Creator, error) {
	if s.candidates == nil || s.creators == nil {
		return repo.Creator{}, errors.New("候选池服务未初始化")
	}

	candidate, err := s.candidates.FindByID(ctx, id)
	if err != nil {
		return repo.Creator{}, err
	}
	alreadyApproved := candidate.Status == "approved"
	if !alreadyApproved && candidate.Status != "reviewing" {
		return repo.Creator{}, errors.New("候选状态不允许批准")
	}

	creator, err := s.creators.UpsertCreator(ctx, repo.Creator{
		Platform:      candidate.Platform,
		UID:           candidate.UID,
		Name:          candidate.Name,
		FollowerCount: candidate.FollowerCount,
		Status:        "active",
	})
	if err != nil {
		return repo.Creator{}, err
	}

	if !alreadyApproved {
		if err := s.candidates.UpdateReviewStatus(ctx, id, []string{"reviewing"}, "approved", s.now()); err != nil {
			return repo.Creator{}, err
		}
		if s.cfg.AutoEnqueueFetchOnApprove && s.fetcher != nil {
			if err := s.fetcher.EnqueueFetchCreator(ctx, creator.ID); err != nil {
				return repo.Creator{}, err
			}
		}
	}

	return creator, nil
}

func (s *Service) Ignore(ctx context.Context, id int64) error {
	return s.updateReviewStatus(ctx, id, []string{"reviewing"}, "ignored")
}

func (s *Service) Block(ctx context.Context, id int64) error {
	return s.updateReviewStatus(ctx, id, []string{"reviewing"}, "blocked")
}

func (s *Service) Review(ctx context.Context, id int64) error {
	return s.updateReviewStatus(ctx, id, []string{"ignored"}, "reviewing")
}

func (s *Service) updateReviewStatus(ctx context.Context, id int64, from []string, to string) error {
	if s.candidates == nil {
		return errors.New("候选池服务未初始化")
	}
	return s.candidates.UpdateReviewStatus(ctx, id, from, to, s.now())
}
