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

type discoverySearchKey struct {
	keyword string
	page    int
}

type candidateSourceClientStub struct {
	creatorHits  map[discoverySearchKey][]bilibili.CreatorHit
	videoHits    map[discoverySearchKey][]bilibili.VideoHit
	creatorCalls []discoverySearchKey
	videoCalls   []discoverySearchKey
}

func (s *candidateSourceClientStub) SearchCreators(ctx context.Context, keyword string, page, pageSize int) ([]bilibili.CreatorHit, error) {
	s.creatorCalls = append(s.creatorCalls, discoverySearchKey{keyword: keyword, page: page})
	return append([]bilibili.CreatorHit(nil), s.creatorHits[discoverySearchKey{keyword: keyword, page: page}]...), nil
}

func (s *candidateSourceClientStub) SearchVideos(ctx context.Context, keyword string, page, pageSize int) ([]bilibili.VideoHit, error) {
	s.videoCalls = append(s.videoCalls, discoverySearchKey{keyword: keyword, page: page})
	return append([]bilibili.VideoHit(nil), s.videoHits[discoverySearchKey{keyword: keyword, page: page}]...), nil
}

type candidateDiscoveryRepoStub struct {
	items       map[string]repo.CandidateCreator
	nextID      int64
	upserts     []repo.CandidateCreator
	sources     map[int64][]repo.CandidateCreatorSource
	scoreDetail map[int64][]repo.CandidateCreatorScoreDetail
}

func (s *candidateDiscoveryRepoStub) Upsert(ctx context.Context, candidate repo.CandidateCreator) (repo.CandidateCreator, error) {
	if s.items == nil {
		s.items = make(map[string]repo.CandidateCreator)
	}
	if existing, ok := s.items[candidate.UID]; ok && existing.ID != 0 {
		candidate.ID = existing.ID
	}
	if candidate.ID == 0 {
		s.nextID++
		candidate.ID = s.nextID
	}
	s.items[candidate.UID] = candidate
	s.upserts = append(s.upserts, candidate)
	return candidate, nil
}

func (s *candidateDiscoveryRepoStub) FindByID(ctx context.Context, id int64) (repo.CandidateCreator, error) {
	for _, item := range s.items {
		if item.ID == id {
			return item, nil
		}
	}
	return repo.CandidateCreator{}, repo.ErrNotFound
}

func (s *candidateDiscoveryRepoStub) FindByPlatformUID(ctx context.Context, platform, uid string) (repo.CandidateCreator, error) {
	item, ok := s.items[uid]
	if !ok {
		return repo.CandidateCreator{}, repo.ErrNotFound
	}
	return item, nil
}

func (s *candidateDiscoveryRepoStub) List(ctx context.Context, filter repo.CandidateListFilter) ([]repo.CandidateCreator, int64, error) {
	return nil, 0, nil
}

func (s *candidateDiscoveryRepoStub) ListSources(ctx context.Context, candidateID int64) ([]repo.CandidateCreatorSource, error) {
	return append([]repo.CandidateCreatorSource(nil), s.sources[candidateID]...), nil
}

func (s *candidateDiscoveryRepoStub) ListScoreDetails(ctx context.Context, candidateID int64) ([]repo.CandidateCreatorScoreDetail, error) {
	return append([]repo.CandidateCreatorScoreDetail(nil), s.scoreDetail[candidateID]...), nil
}

func (s *candidateDiscoveryRepoStub) ReplaceSources(ctx context.Context, candidateID int64, sources []repo.CandidateCreatorSource) error {
	if s.sources == nil {
		s.sources = make(map[int64][]repo.CandidateCreatorSource)
	}
	copied := make([]repo.CandidateCreatorSource, len(sources))
	copy(copied, sources)
	for i := range copied {
		copied[i].CandidateCreatorID = candidateID
	}
	s.sources[candidateID] = copied
	return nil
}

func (s *candidateDiscoveryRepoStub) ReplaceScoreDetails(ctx context.Context, candidateID int64, details []repo.CandidateCreatorScoreDetail) error {
	if s.scoreDetail == nil {
		s.scoreDetail = make(map[int64][]repo.CandidateCreatorScoreDetail)
	}
	copied := make([]repo.CandidateCreatorScoreDetail, len(details))
	copy(copied, details)
	for i := range copied {
		copied[i].CandidateCreatorID = candidateID
	}
	s.scoreDetail[candidateID] = copied
	return nil
}

func (s *candidateDiscoveryRepoStub) UpdateReviewStatus(ctx context.Context, id int64, from []string, to string, at time.Time) error {
	return nil
}

func TestKeywordDiscovererAggregatesSourcesAndPreservesIgnoredStatus(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	client := &candidateSourceClientStub{
		creatorHits: map[discoverySearchKey][]bilibili.CreatorHit{
			{keyword: "补档", page: 1}: {
				{UID: "1001", Name: "作者 A", FollowerCount: 8800, ProfileURL: "https://space.bilibili.com/1001"},
				{UID: "2002", Name: "作者 B", FollowerCount: 9900, ProfileURL: "https://space.bilibili.com/2002"},
			},
			{keyword: "重传", page: 1}: {
				{UID: "1001", Name: "作者 A", FollowerCount: 8800, ProfileURL: "https://space.bilibili.com/1001"},
				{UID: "3003", Name: "作者 C", FollowerCount: 1200, ProfileURL: "https://space.bilibili.com/3003"},
			},
		},
		videoHits: map[discoverySearchKey][]bilibili.VideoHit{
			{keyword: "补档", page: 1}: {
				{UID: "1001", CreatorName: "作者 A", VideoID: "BV1001", Title: "补档一", PublishTime: now.Add(-24 * time.Hour), ViewCount: 11, FavoriteCount: 2},
				{UID: "2002", CreatorName: "作者 B", VideoID: "BV2002", Title: "补档二", PublishTime: now.Add(-48 * time.Hour), ViewCount: 22, FavoriteCount: 3},
			},
			{keyword: "重传", page: 1}: {
				{UID: "1001", CreatorName: "作者 A", VideoID: "BV1002", Title: "重传一", PublishTime: now.Add(-72 * time.Hour), ViewCount: 33, FavoriteCount: 4},
				{UID: "3003", CreatorName: "作者 C", VideoID: "BV3003", Title: "重传二", PublishTime: now.Add(-96 * time.Hour), ViewCount: 44, FavoriteCount: 5},
			},
		},
	}
	repoStub := &candidateDiscoveryRepoStub{
		items: map[string]repo.CandidateCreator{
			"2002": {ID: 2, Platform: "bilibili", UID: "2002", Name: "作者 B", Status: "blocked", BlockedAt: now.Add(-time.Hour)},
			"3003": {ID: 3, Platform: "bilibili", UID: "3003", Name: "作者 C", Status: "ignored", IgnoredAt: now.Add(-2 * time.Hour)},
		},
		nextID: 10,
	}
	cfg := config.Default().Discovery
	cfg.Keywords = []string{"补档", "重传"}
	cfg.MaxKeywordsPerRun = 2
	cfg.MaxPagesPerKeyword = 1
	cfg.MaxCandidatesPerRun = 10

	discoverer := NewKeywordDiscoverer(client, repoStub, NewScorer(cfg), cfg)
	discoverer.now = func() time.Time { return now }

	result, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if result.Discovered != 2 || result.SkippedBlocked != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(repoStub.upserts) != 2 {
		t.Fatalf("expected 2 upserts, got %+v", repoStub.upserts)
	}

	var uid1001, uid3003 repo.CandidateCreator
	for _, item := range repoStub.upserts {
		switch item.UID {
		case "1001":
			uid1001 = item
		case "3003":
			uid3003 = item
		case "2002":
			t.Fatalf("blocked candidate should not be upserted")
		}
	}
	if uid1001.Status != "reviewing" {
		t.Fatalf("expected uid1001 reviewing, got %+v", uid1001)
	}
	if uid3003.Status != "ignored" {
		t.Fatalf("expected uid3003 ignored, got %+v", uid3003)
	}
	if !uid3003.LastDiscoveredAt.Equal(now) {
		t.Fatalf("expected ignored candidate last_discovered_at refreshed, got %s", uid3003.LastDiscoveredAt)
	}

	sources1001 := repoStub.sources[uid1001.ID]
	if len(sources1001) != 2 {
		t.Fatalf("expected 2 keyword sources for uid1001, got %+v", sources1001)
	}
	var detail struct {
		Keyword string              `json:"keyword"`
		Videos  []bilibili.VideoHit `json:"videos"`
	}
	if err := json.Unmarshal(sources1001[0].DetailJSON, &detail); err != nil {
		t.Fatalf("unmarshal detail_json: %v", err)
	}
	if detail.Keyword == "" || len(detail.Videos) == 0 {
		t.Fatalf("expected keyword detail with videos, got %+v", detail)
	}
	if len(repoStub.scoreDetail[uid1001.ID]) == 0 {
		t.Fatalf("expected score details for uid1001")
	}
	if len(repoStub.scoreDetail[uid3003.ID]) == 0 {
		t.Fatalf("expected score details for uid3003")
	}
}

func TestKeywordDiscovererStopsAtConfiguredLimits(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	client := &candidateSourceClientStub{
		creatorHits: map[discoverySearchKey][]bilibili.CreatorHit{
			{keyword: "补档", page: 1}: {
				{UID: "1001", Name: "作者 A", FollowerCount: 8800},
			},
			{keyword: "补档", page: 2}: {
				{UID: "1002", Name: "作者 B", FollowerCount: 9900},
			},
			{keyword: "重传", page: 1}: {
				{UID: "1003", Name: "作者 C", FollowerCount: 7700},
			},
		},
		videoHits: map[discoverySearchKey][]bilibili.VideoHit{
			{keyword: "补档", page: 1}: {
				{UID: "1001", CreatorName: "作者 A", VideoID: "BV1", Title: "补档", PublishTime: now.Add(-time.Hour)},
			},
		},
	}
	repoStub := &candidateDiscoveryRepoStub{}
	cfg := config.Default().Discovery
	cfg.Keywords = []string{"补档", "重传", "演唱会"}
	cfg.MaxKeywordsPerRun = 2
	cfg.MaxPagesPerKeyword = 1
	cfg.MaxCandidatesPerRun = 1

	discoverer := NewKeywordDiscoverer(client, repoStub, NewScorer(cfg), cfg)
	discoverer.now = func() time.Time { return now }

	result, err := discoverer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if result.Discovered != 1 {
		t.Fatalf("expected discovered=1, got %+v", result)
	}
	if len(client.creatorCalls) != 1 || client.creatorCalls[0] != (discoverySearchKey{keyword: "补档", page: 1}) {
		t.Fatalf("expected only first keyword/page search, got %+v", client.creatorCalls)
	}
	if len(client.videoCalls) != 1 || client.videoCalls[0] != (discoverySearchKey{keyword: "补档", page: 1}) {
		t.Fatalf("expected only first keyword/page video search, got %+v", client.videoCalls)
	}
	if len(repoStub.upserts) != 1 || repoStub.upserts[0].UID != "1001" {
		t.Fatalf("unexpected upserts: %+v", repoStub.upserts)
	}
}
