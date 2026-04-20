package discovery

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
)

type relatedCreatorRepoStub struct {
	active []repo.Creator
}

func (s *relatedCreatorRepoStub) Upsert(ctx context.Context, c repo.Creator) (int64, error) {
	return 0, nil
}

func (s *relatedCreatorRepoStub) Create(ctx context.Context, c repo.Creator) (int64, error) {
	return 0, nil
}

func (s *relatedCreatorRepoStub) Update(ctx context.Context, c repo.Creator) error {
	return nil
}

func (s *relatedCreatorRepoStub) UpdateStatus(ctx context.Context, id int64, status string) error {
	return nil
}

func (s *relatedCreatorRepoStub) DeleteByID(ctx context.Context, id int64) (int64, error) {
	return 0, nil
}

func (s *relatedCreatorRepoStub) FindByID(ctx context.Context, id int64) (repo.Creator, error) {
	return repo.Creator{}, repo.ErrNotFound
}

func (s *relatedCreatorRepoStub) FindByPlatformUID(ctx context.Context, platform, uid string) (repo.Creator, error) {
	for _, item := range s.active {
		if item.Platform == platform && item.UID == uid {
			return item, nil
		}
	}
	return repo.Creator{}, repo.ErrNotFound
}

func (s *relatedCreatorRepoStub) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	if limit <= 0 || limit >= len(s.active) {
		return append([]repo.Creator(nil), s.active...), nil
	}
	return append([]repo.Creator(nil), s.active[:limit]...), nil
}

func (s *relatedCreatorRepoStub) ListActiveAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	return nil, nil
}

func (s *relatedCreatorRepoStub) ListForLibraryAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	return nil, nil
}

func (s *relatedCreatorRepoStub) CountActive(ctx context.Context) (int64, error) {
	return int64(len(s.active)), nil
}

type relatedSourceClientStub struct {
	videosByUID     map[string][]bilibili.VideoMeta
	searchByKeyword map[string][]bilibili.VideoHit
	listCalls       []string
	searchCalls     []string
}

func (s *relatedSourceClientStub) ListVideos(ctx context.Context, uid string) ([]bilibili.VideoMeta, error) {
	s.listCalls = append(s.listCalls, uid)
	return append([]bilibili.VideoMeta(nil), s.videosByUID[uid]...), nil
}

func (s *relatedSourceClientStub) SearchRelatedVideos(ctx context.Context, keyword string, page, pageSize int) ([]bilibili.VideoHit, error) {
	s.searchCalls = append(s.searchCalls, keyword)
	return append([]bilibili.VideoHit(nil), s.searchByKeyword[keyword]...), nil
}

func TestRelatedDiscovererExpandsOnlyOneHopAndHonorsPerCreatorLimit(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	creators := &relatedCreatorRepoStub{
		active: []repo.Creator{
			{ID: 1, Platform: "bilibili", UID: "1001", Name: "已追踪 A", Status: "active"},
			{ID: 2, Platform: "bilibili", UID: "1002", Name: "已追踪 B", Status: "active"},
		},
	}
	client := &relatedSourceClientStub{
		videosByUID: map[string][]bilibili.VideoMeta{
			"1001": {
				{VideoID: "BVsrc1", Title: "补档 演唱会 全场", PublishTime: now.Add(-time.Hour)},
			},
		},
		searchByKeyword: map[string][]bilibili.VideoHit{
			"补档": {
				{UID: "1001", CreatorName: "已追踪 A", VideoID: "BVsame", Title: "补档 演唱会 全场", PublishTime: now.Add(-2 * time.Hour), ViewCount: 999},
				{UID: "2001", CreatorName: "候选甲", VideoID: "BV2001", Title: "补档 演唱会 全场", PublishTime: now.Add(-2 * time.Hour), ViewCount: 800},
				{UID: "2002", CreatorName: "候选乙", VideoID: "BV2002", Title: "演唱会 全场 剪辑", PublishTime: now.Add(-3 * time.Hour), ViewCount: 700},
				{UID: "2003", CreatorName: "候选丙", VideoID: "BV2003", Title: "补档片段", PublishTime: now.Add(-4 * time.Hour), ViewCount: 600},
			},
		},
	}
	candidates := &candidateDiscoveryRepoStub{nextID: 10}
	cfg := config.Default().Discovery
	cfg.Keywords = []string{"补档"}
	cfg.MaxRelatedPerCreator = 2

	discoverer := NewRelatedDiscoverer(creators, candidates, client, NewScorer(cfg), cfg)
	discoverer.now = func() time.Time { return now }

	result, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if result.SourceCreators != 2 {
		t.Fatalf("expected 2 source creators, got %+v", result)
	}
	if result.Discovered != 2 {
		t.Fatalf("expected discovered=2, got %+v", result)
	}
	if len(client.listCalls) != 2 || client.listCalls[0] != "1001" || client.listCalls[1] != "1002" {
		t.Fatalf("expected only tracked creators listVideos, got %+v", client.listCalls)
	}
	if len(candidates.upserts) != 2 {
		t.Fatalf("expected 2 upserts, got %+v", candidates.upserts)
	}
	if candidates.upserts[0].UID != "2001" || candidates.upserts[1].UID != "2002" {
		t.Fatalf("expected top 2 related candidates kept, got %+v", candidates.upserts)
	}
	for _, item := range candidates.upserts {
		if item.UID == "1001" {
			t.Fatalf("tracked creator should not become candidate")
		}
	}
	sources := candidates.sources[candidates.upserts[0].ID]
	if len(sources) != 1 || sources[0].SourceType != "related_creator" || sources[0].SourceValue != "1001" {
		t.Fatalf("expected related source from creator 1001, got %+v", sources)
	}
}

func TestRelatedDiscovererMergesWithExistingKeywordCandidate(t *testing.T) {
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	creators := &relatedCreatorRepoStub{
		active: []repo.Creator{
			{ID: 1, Platform: "bilibili", UID: "1001", Name: "已追踪 A", Status: "active"},
		},
	}
	client := &relatedSourceClientStub{
		videosByUID: map[string][]bilibili.VideoMeta{
			"1001": {
				{VideoID: "BVsrc1", Title: "补档 演唱会 全场", PublishTime: now.Add(-time.Hour)},
			},
		},
		searchByKeyword: map[string][]bilibili.VideoHit{
			"补档": {
				{UID: "2001", CreatorName: "候选甲", VideoID: "BV2001", Title: "补档 演唱会 全场", PublishTime: now.Add(-2 * time.Hour), ViewCount: 800},
			},
		},
	}
	candidates := &candidateDiscoveryRepoStub{
		items: map[string]repo.CandidateCreator{
			"2001": {ID: 7, Platform: "bilibili", UID: "2001", Name: "候选甲", Status: "reviewing", Score: 15, ScoreVersion: "v1"},
		},
		sources: map[int64][]repo.CandidateCreatorSource{
			7: {
				{CandidateCreatorID: 7, SourceType: "keyword", SourceValue: "补档", SourceLabel: "关键词：补档", Weight: 15},
			},
		},
		scoreDetail: map[int64][]repo.CandidateCreatorScoreDetail{
			7: {
				{CandidateCreatorID: 7, FactorKey: "keyword_risk", FactorLabel: "命中高风险关键词", ScoreDelta: 15},
			},
		},
		nextID: 7,
	}
	cfg := config.Default().Discovery
	cfg.Keywords = []string{"补档"}
	cfg.MaxRelatedPerCreator = 2

	discoverer := NewRelatedDiscoverer(creators, candidates, client, NewScorer(cfg), cfg)
	discoverer.now = func() time.Time { return now }

	result, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if result.Discovered != 1 {
		t.Fatalf("expected discovered=1, got %+v", result)
	}
	if len(candidates.sources[7]) != 2 {
		t.Fatalf("expected keyword + related sources, got %+v", candidates.sources[7])
	}
	var foundRelated bool
	for _, source := range candidates.sources[7] {
		if source.SourceType == "related_creator" {
			foundRelated = true
			var detail map[string]any
			if err := json.Unmarshal(source.DetailJSON, &detail); err != nil {
				t.Fatalf("unmarshal related detail: %v", err)
			}
		}
	}
	if !foundRelated {
		t.Fatalf("expected related source to be merged")
	}
	if len(candidates.scoreDetail[7]) != 2 {
		t.Fatalf("expected keyword detail + similarity detail, got %+v", candidates.scoreDetail[7])
	}
	if candidates.items["2001"].Score <= 15 {
		t.Fatalf("expected merged score > 15, got %+v", candidates.items["2001"])
	}
}
