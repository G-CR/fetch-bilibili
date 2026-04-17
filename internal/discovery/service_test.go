package discovery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/repo"
)

type candidateRepoStub struct {
	items       map[int64]repo.CandidateCreator
	sources     map[int64][]repo.CandidateCreatorSource
	scoreDetail map[int64][]repo.CandidateCreatorScoreDetail
	list        []repo.CandidateCreator
	total       int64
	updates     []candidateStatusUpdate
}

type candidateStatusUpdate struct {
	id   int64
	from []string
	to   string
	at   time.Time
}

func (s *candidateRepoStub) Upsert(ctx context.Context, candidate repo.CandidateCreator) (repo.CandidateCreator, error) {
	if s.items == nil {
		s.items = make(map[int64]repo.CandidateCreator)
	}
	if candidate.ID == 0 {
		candidate.ID = int64(len(s.items) + 1)
	}
	s.items[candidate.ID] = candidate
	return candidate, nil
}

func (s *candidateRepoStub) FindByID(ctx context.Context, id int64) (repo.CandidateCreator, error) {
	candidate, ok := s.items[id]
	if !ok {
		return repo.CandidateCreator{}, repo.ErrNotFound
	}
	return candidate, nil
}

func (s *candidateRepoStub) FindByPlatformUID(ctx context.Context, platform, uid string) (repo.CandidateCreator, error) {
	for _, item := range s.items {
		if item.Platform == platform && item.UID == uid {
			return item, nil
		}
	}
	return repo.CandidateCreator{}, repo.ErrNotFound
}

func (s *candidateRepoStub) List(ctx context.Context, filter repo.CandidateListFilter) ([]repo.CandidateCreator, int64, error) {
	return append([]repo.CandidateCreator(nil), s.list...), s.total, nil
}

func (s *candidateRepoStub) ListSources(ctx context.Context, candidateID int64) ([]repo.CandidateCreatorSource, error) {
	return append([]repo.CandidateCreatorSource(nil), s.sources[candidateID]...), nil
}

func (s *candidateRepoStub) ListScoreDetails(ctx context.Context, candidateID int64) ([]repo.CandidateCreatorScoreDetail, error) {
	return append([]repo.CandidateCreatorScoreDetail(nil), s.scoreDetail[candidateID]...), nil
}

func (s *candidateRepoStub) ReplaceSources(ctx context.Context, candidateID int64, sources []repo.CandidateCreatorSource) error {
	s.sources[candidateID] = append([]repo.CandidateCreatorSource(nil), sources...)
	return nil
}

func (s *candidateRepoStub) ReplaceScoreDetails(ctx context.Context, candidateID int64, details []repo.CandidateCreatorScoreDetail) error {
	s.scoreDetail[candidateID] = append([]repo.CandidateCreatorScoreDetail(nil), details...)
	return nil
}

func (s *candidateRepoStub) UpdateReviewStatus(ctx context.Context, id int64, from []string, to string, at time.Time) error {
	item, ok := s.items[id]
	if !ok {
		return repo.ErrNotFound
	}
	allowed := false
	for _, status := range from {
		if item.Status == status {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("非法状态流转: %s -> %s", item.Status, to)
	}
	item.Status = to
	switch to {
	case "approved":
		item.ApprovedAt = at
	case "ignored":
		item.IgnoredAt = at
	case "blocked":
		item.BlockedAt = at
	}
	s.items[id] = item
	s.updates = append(s.updates, candidateStatusUpdate{id: id, from: append([]string(nil), from...), to: to, at: at})
	return nil
}

type creatorWriterStub struct {
	upserted []repo.Creator
	result   repo.Creator
	err      error
}

func (s *creatorWriterStub) UpsertCreator(ctx context.Context, creator repo.Creator) (repo.Creator, error) {
	s.upserted = append(s.upserted, creator)
	if s.err != nil {
		return repo.Creator{}, s.err
	}
	if s.result.ID == 0 {
		s.result = creator
		s.result.ID = 9
	}
	return s.result, nil
}

type fetchEnqueuerStub struct {
	creatorIDs []int64
	err        error
}

func (s *fetchEnqueuerStub) EnqueueFetchCreator(ctx context.Context, creatorID int64) error {
	if s.err != nil {
		return s.err
	}
	s.creatorIDs = append(s.creatorIDs, creatorID)
	return nil
}

func TestServiceListCandidates(t *testing.T) {
	candidates := &candidateRepoStub{
		list: []repo.CandidateCreator{
			{ID: 1, Platform: "bilibili", UID: "123", Name: "候选 A", Status: "reviewing", Score: 80},
		},
		total: 1,
		sources: map[int64][]repo.CandidateCreatorSource{
			1: {
				{CandidateCreatorID: 1, SourceType: "keyword", SourceLabel: "关键词：补档", Weight: 12},
				{CandidateCreatorID: 1, SourceType: "related_creator", SourceLabel: "来自已追踪博主", Weight: 8},
			},
		},
	}
	svc := NewService(candidates, nil, nil, config.Default().Discovery)

	items, total, err := svc.ListCandidates(context.Background(), repo.CandidateListFilter{Status: "reviewing"})
	if err != nil {
		t.Fatalf("ListCandidates error: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("unexpected list result: total=%d len=%d", total, len(items))
	}
	if len(items[0].Sources) != 2 {
		t.Fatalf("expected 2 sources, got %+v", items[0].Sources)
	}
}

func TestServiceGetCandidate(t *testing.T) {
	candidates := &candidateRepoStub{
		items: map[int64]repo.CandidateCreator{
			5: {ID: 5, Platform: "bilibili", UID: "abc", Name: "候选详情", Status: "reviewing", Score: 66},
		},
		sources: map[int64][]repo.CandidateCreatorSource{
			5: {{CandidateCreatorID: 5, SourceType: "keyword", SourceLabel: "关键词：影视剪辑", Weight: 15}},
		},
		scoreDetail: map[int64][]repo.CandidateCreatorScoreDetail{
			5: {{CandidateCreatorID: 5, FactorKey: "keyword_risk", ScoreDelta: 15}},
		},
	}
	svc := NewService(candidates, nil, nil, config.Default().Discovery)

	detail, err := svc.GetCandidate(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetCandidate error: %v", err)
	}
	if detail.Candidate.ID != 5 || len(detail.Sources) != 1 || len(detail.ScoreDetails) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestServiceApproveIsIdempotentAndEnqueuesFetchOnce(t *testing.T) {
	now := time.Now().UTC()
	candidates := &candidateRepoStub{
		items: map[int64]repo.CandidateCreator{
			1: {ID: 1, Platform: "bilibili", UID: "123", Name: "候选博主", FollowerCount: 77, Status: "reviewing"},
		},
	}
	creatorWriter := &creatorWriterStub{}
	fetcher := &fetchEnqueuerStub{}
	cfg := config.Default().Discovery
	cfg.AutoEnqueueFetchOnApprove = true

	svc := NewService(candidates, creatorWriter, fetcher, cfg)
	svc.now = func() time.Time { return now }

	creator, err := svc.Approve(context.Background(), 1)
	if err != nil {
		t.Fatalf("Approve error: %v", err)
	}
	if creator.ID == 0 || creator.UID != "123" {
		t.Fatalf("unexpected creator: %+v", creator)
	}
	if len(candidates.updates) != 1 || candidates.updates[0].to != "approved" {
		t.Fatalf("expected approved update, got %+v", candidates.updates)
	}
	if len(fetcher.creatorIDs) != 1 || fetcher.creatorIDs[0] != creator.ID {
		t.Fatalf("expected one fetch enqueue for creator %d, got %+v", creator.ID, fetcher.creatorIDs)
	}

	creatorAgain, err := svc.Approve(context.Background(), 1)
	if err != nil {
		t.Fatalf("Approve second time error: %v", err)
	}
	if creatorAgain.ID != creator.ID {
		t.Fatalf("expected same creator id, got %d vs %d", creatorAgain.ID, creator.ID)
	}
	if len(fetcher.creatorIDs) != 1 {
		t.Fatalf("expected no duplicate fetch enqueue, got %+v", fetcher.creatorIDs)
	}
}

func TestServiceRejectsIllegalReviewTransitions(t *testing.T) {
	now := time.Now().UTC()
	candidates := &candidateRepoStub{
		items: map[int64]repo.CandidateCreator{
			2: {ID: 2, Platform: "bilibili", UID: "222", Name: "已批准", Status: "approved"},
			3: {ID: 3, Platform: "bilibili", UID: "333", Name: "审核中", Status: "reviewing"},
			4: {ID: 4, Platform: "bilibili", UID: "444", Name: "已拉黑", Status: "blocked"},
		},
	}
	svc := NewService(candidates, nil, nil, config.Default().Discovery)
	svc.now = func() time.Time { return now }

	if err := svc.Ignore(context.Background(), 2); err == nil {
		t.Fatalf("expected ignore to reject approved candidate")
	}
	if err := svc.Review(context.Background(), 3); err == nil {
		t.Fatalf("expected review to reject reviewing candidate")
	}
	if _, err := svc.Approve(context.Background(), 4); err == nil {
		t.Fatalf("expected approve to reject blocked candidate")
	}
}
