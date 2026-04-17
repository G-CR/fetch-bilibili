package discovery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
)

const defaultKeywordDiscoverPageSize = 20

type CandidateSourceClient interface {
	SearchCreators(ctx context.Context, keyword string, page, pageSize int) ([]bilibili.CreatorHit, error)
	SearchVideos(ctx context.Context, keyword string, page, pageSize int) ([]bilibili.VideoHit, error)
}

type KeywordDiscoverResult struct {
	Keywords       int
	Discovered     int
	SkippedBlocked int
}

type KeywordDiscoverer struct {
	client   CandidateSourceClient
	repo     repo.CandidateRepository
	scorer   *Scorer
	cfg      config.DiscoveryConfig
	now      func() time.Time
	pageSize int
}

type candidateLookup struct {
	candidate repo.CandidateCreator
	found     bool
}

func NewKeywordDiscoverer(client CandidateSourceClient, repo repo.CandidateRepository, scorer *Scorer, cfg config.DiscoveryConfig) *KeywordDiscoverer {
	return &KeywordDiscoverer{
		client:   client,
		repo:     repo,
		scorer:   scorer,
		cfg:      cfg,
		now:      time.Now,
		pageSize: defaultKeywordDiscoverPageSize,
	}
}

func (d *KeywordDiscoverer) Discover(ctx context.Context) (KeywordDiscoverResult, error) {
	if d.client == nil || d.repo == nil || d.scorer == nil {
		return KeywordDiscoverResult{}, errors.New("关键词发现器未初始化")
	}

	keywords := d.runtimeKeywords()
	result := KeywordDiscoverResult{Keywords: len(keywords)}
	if len(keywords) == 0 {
		return result, nil
	}

	aggregates := make(map[string]*keywordCandidateAggregate)
	existingCache := make(map[string]candidateLookup)
	order := make([]string, 0)
	skippedBlocked := make(map[string]struct{})

	pageLimit := d.cfg.MaxPagesPerKeyword
	if pageLimit <= 0 {
		pageLimit = 1
	}
	pageSize := d.pageSize
	if pageSize <= 0 {
		pageSize = defaultKeywordDiscoverPageSize
	}

outer:
	for _, keyword := range keywords {
		for page := 1; page <= pageLimit; page++ {
			creators, err := d.client.SearchCreators(ctx, keyword, page, pageSize)
			if err != nil {
				return result, fmt.Errorf("关键词发现失败: 关键词=%s 页码=%d 搜索作者失败: %w", keyword, page, err)
			}
			videos, err := d.client.SearchVideos(ctx, keyword, page, pageSize)
			if err != nil {
				return result, fmt.Errorf("关键词发现失败: 关键词=%s 页码=%d 搜索视频失败: %w", keyword, page, err)
			}
			if len(creators) == 0 && len(videos) == 0 {
				break
			}

			pageHits, pageOrder := mergeKeywordPage(keyword, creators, videos)
			for _, uid := range pageOrder {
				hit := pageHits[uid]
				if hit == nil {
					continue
				}
				agg := aggregates[uid]
				if agg == nil {
					lookup, ok := existingCache[uid]
					if !ok {
						candidate, err := d.repo.FindByPlatformUID(ctx, "bilibili", uid)
						if err != nil {
							if !errors.Is(err, repo.ErrNotFound) {
								return result, fmt.Errorf("查询候选失败: uid=%s: %w", uid, err)
							}
							existingCache[uid] = candidateLookup{}
							lookup = candidateLookup{}
						} else {
							lookup = candidateLookup{candidate: candidate, found: true}
							existingCache[uid] = lookup
						}
					}
					if lookup.found && lookup.candidate.Status == "blocked" {
						skippedBlocked[uid] = struct{}{}
						continue
					}
					if d.cfg.MaxCandidatesPerRun > 0 && len(order) >= d.cfg.MaxCandidatesPerRun {
						break outer
					}
					agg = newKeywordCandidateAggregate(keyword, lookup)
					aggregates[uid] = agg
					order = append(order, uid)
				}
				agg.merge(hit)
			}
			if d.cfg.MaxCandidatesPerRun > 0 && len(order) >= d.cfg.MaxCandidatesPerRun {
				break outer
			}
		}
	}

	now := d.now().UTC()
	for _, uid := range order {
		agg := aggregates[uid]
		if agg == nil {
			continue
		}
		candidate := agg.buildCandidate(now, d.cfg.ScoreVersion)
		score := d.scorer.Score(agg.buildScoreInput(now))
		candidate.Score = score.Total
		candidate.ScoreVersion = score.ScoreVersion
		candidate.LastScoredAt = now

		saved, err := d.repo.Upsert(ctx, candidate)
		if err != nil {
			return result, fmt.Errorf("写入候选失败: uid=%s: %w", uid, err)
		}
		sources := agg.buildSources(saved.ID)
		if err := d.repo.ReplaceSources(ctx, saved.ID, sources); err != nil {
			return result, fmt.Errorf("写入候选来源失败: uid=%s: %w", uid, err)
		}
		for i := range score.Details {
			score.Details[i].CandidateCreatorID = saved.ID
		}
		if err := d.repo.ReplaceScoreDetails(ctx, saved.ID, score.Details); err != nil {
			return result, fmt.Errorf("写入候选评分明细失败: uid=%s: %w", uid, err)
		}
	}

	result.Discovered = len(order)
	result.SkippedBlocked = len(skippedBlocked)
	return result, nil
}

func (d *KeywordDiscoverer) runtimeKeywords() []string {
	if len(d.cfg.Keywords) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(d.cfg.Keywords))
	keywords := make([]string, 0, len(d.cfg.Keywords))
	for _, raw := range d.cfg.Keywords {
		keyword := strings.TrimSpace(raw)
		if keyword == "" {
			continue
		}
		if _, ok := seen[keyword]; ok {
			continue
		}
		seen[keyword] = struct{}{}
		keywords = append(keywords, keyword)
		if d.cfg.MaxKeywordsPerRun > 0 && len(keywords) >= d.cfg.MaxKeywordsPerRun {
			break
		}
	}
	return keywords
}

type keywordPageHit struct {
	keyword string
	creator bilibili.CreatorHit
	videos  []bilibili.VideoHit
}

func mergeKeywordPage(keyword string, creators []bilibili.CreatorHit, videos []bilibili.VideoHit) (map[string]*keywordPageHit, []string) {
	items := make(map[string]*keywordPageHit)
	order := make([]string, 0, len(creators)+len(videos))
	appendUID := func(uid string) *keywordPageHit {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			return nil
		}
		if item, ok := items[uid]; ok {
			return item
		}
		item := &keywordPageHit{}
		items[uid] = item
		order = append(order, uid)
		return item
	}

	for _, creator := range creators {
		item := appendUID(creator.UID)
		if item == nil {
			continue
		}
		item.keyword = keyword
		item.creator = mergeCreatorHit(item.creator, creator)
	}
	for _, video := range videos {
		item := appendUID(video.UID)
		if item == nil {
			continue
		}
		item.keyword = keyword
		item.creator = mergeCreatorHit(item.creator, bilibili.CreatorHit{
			UID:        video.UID,
			Name:       video.CreatorName,
			ProfileURL: profileURLForUID(video.UID),
		})
		item.videos = append(item.videos, video)
	}
	return items, order
}

func mergeCreatorHit(base, next bilibili.CreatorHit) bilibili.CreatorHit {
	if strings.TrimSpace(base.UID) == "" {
		base.UID = strings.TrimSpace(next.UID)
	}
	if strings.TrimSpace(next.Name) != "" {
		base.Name = strings.TrimSpace(next.Name)
	}
	if strings.TrimSpace(next.AvatarURL) != "" {
		base.AvatarURL = strings.TrimSpace(next.AvatarURL)
	}
	if strings.TrimSpace(next.ProfileURL) != "" {
		base.ProfileURL = strings.TrimSpace(next.ProfileURL)
	}
	if next.FollowerCount > base.FollowerCount {
		base.FollowerCount = next.FollowerCount
	}
	if strings.TrimSpace(next.Signature) != "" {
		base.Signature = strings.TrimSpace(next.Signature)
	}
	return base
}

type keywordCandidateAggregate struct {
	existing      repo.CandidateCreator
	foundExisting bool
	creator       bilibili.CreatorHit
	sources       map[string]*keywordSourceAggregate
	sourceOrder   []string
	videos        map[string]bilibili.VideoHit
	videoOrder    []string
}

type keywordSourceAggregate struct {
	keyword    string
	weight     int
	videos     map[string]bilibili.VideoHit
	videoOrder []string
}

func newKeywordCandidateAggregate(keyword string, lookup candidateLookup) *keywordCandidateAggregate {
	agg := &keywordCandidateAggregate{
		existing:      lookup.candidate,
		foundExisting: lookup.found,
		sources:       make(map[string]*keywordSourceAggregate),
		videos:        make(map[string]bilibili.VideoHit),
	}
	_ = keyword
	return agg
}

func (a *keywordCandidateAggregate) merge(hit *keywordPageHit) {
	if a == nil || hit == nil {
		return
	}
	a.creator = mergeCreatorHit(a.creator, hit.creator)
	source := a.addKeyword(hit.keyword)
	if source.weight == 0 {
		source.weight = keywordRiskWeight(hit.keyword)
	}
	for _, video := range hit.videos {
		a.addVideo(hit.keyword, video)
	}
}

func (a *keywordCandidateAggregate) ensureSource(keyword string) *keywordSourceAggregate {
	if source, ok := a.sources[keyword]; ok {
		return source
	}
	source := &keywordSourceAggregate{
		keyword: keyword,
		weight:  keywordRiskWeight(keyword),
		videos:  make(map[string]bilibili.VideoHit),
	}
	a.sources[keyword] = source
	a.sourceOrder = append(a.sourceOrder, keyword)
	return source
}

func (a *keywordCandidateAggregate) addKeyword(keyword string) *keywordSourceAggregate {
	return a.ensureSource(keyword)
}

func (a *keywordCandidateAggregate) addVideo(keyword string, video bilibili.VideoHit) {
	if a == nil {
		return
	}
	source := a.ensureSource(keyword)
	if videoID := strings.TrimSpace(video.VideoID); videoID != "" {
		if _, ok := source.videos[videoID]; !ok {
			source.videoOrder = append(source.videoOrder, videoID)
		}
		source.videos[videoID] = video
		if _, ok := a.videos[videoID]; !ok {
			a.videoOrder = append(a.videoOrder, videoID)
		}
		a.videos[videoID] = video
	}
	if strings.TrimSpace(a.creator.Name) == "" && strings.TrimSpace(video.CreatorName) != "" {
		a.creator.Name = strings.TrimSpace(video.CreatorName)
	}
	if strings.TrimSpace(a.creator.ProfileURL) == "" {
		a.creator.ProfileURL = profileURLForUID(video.UID)
	}
}

func (a *keywordCandidateAggregate) buildCandidate(now time.Time, scoreVersion string) repo.CandidateCreator {
	candidate := repo.CandidateCreator{
		Platform:         "bilibili",
		UID:              a.creator.UID,
		Name:             strings.TrimSpace(a.creator.Name),
		AvatarURL:        strings.TrimSpace(a.creator.AvatarURL),
		ProfileURL:       strings.TrimSpace(a.creator.ProfileURL),
		FollowerCount:    a.creator.FollowerCount,
		Status:           "reviewing",
		ScoreVersion:     scoreVersion,
		LastDiscoveredAt: now,
	}
	if a.foundExisting {
		candidate.ID = a.existing.ID
		if candidate.Name == "" {
			candidate.Name = a.existing.Name
		}
		if candidate.AvatarURL == "" {
			candidate.AvatarURL = a.existing.AvatarURL
		}
		if candidate.ProfileURL == "" {
			candidate.ProfileURL = a.existing.ProfileURL
		}
		if candidate.FollowerCount == 0 {
			candidate.FollowerCount = a.existing.FollowerCount
		}
		candidate.Status = strings.TrimSpace(a.existing.Status)
		if candidate.Status == "" {
			candidate.Status = "reviewing"
		}
		candidate.ApprovedAt = a.existing.ApprovedAt
		candidate.IgnoredAt = a.existing.IgnoredAt
		candidate.BlockedAt = a.existing.BlockedAt
	}
	return candidate
}

func (a *keywordCandidateAggregate) buildScoreInput(now time.Time) ScoreInput {
	keywordHits := make([]KeywordHit, 0, len(a.sourceOrder))
	deletionTraceSet := make(map[string]struct{})
	activity30d := 0
	for _, keyword := range a.sourceOrder {
		source := a.sources[keyword]
		if source == nil {
			continue
		}
		keywordHits = append(keywordHits, KeywordHit{Keyword: keyword, Score: source.weight})
		for _, hit := range detectDeletionTrace(keyword) {
			deletionTraceSet[hit] = struct{}{}
		}
	}
	for _, videoID := range a.videoOrder {
		video := a.videos[videoID]
		if !video.PublishTime.IsZero() && now.Sub(video.PublishTime) <= 30*24*time.Hour && now.After(video.PublishTime) {
			activity30d++
		}
		for _, hit := range detectDeletionTrace(video.Title) {
			deletionTraceSet[hit] = struct{}{}
		}
	}
	deletionTraceHits := make([]string, 0, len(deletionTraceSet))
	for hit := range deletionTraceSet {
		deletionTraceHits = append(deletionTraceHits, hit)
	}
	sort.Strings(deletionTraceHits)

	ignoreCount := 0
	if a.foundExisting && a.existing.Status == "ignored" {
		ignoreCount = 1
	}
	return ScoreInput{
		KeywordHits:       keywordHits,
		Activity30d:       activity30d,
		FollowerCount:     maxInt64(a.creator.FollowerCount, a.existing.FollowerCount),
		DeletionTraceHits: deletionTraceHits,
		IgnoreCount:       ignoreCount,
	}
}

func (a *keywordCandidateAggregate) buildSources(candidateID int64) []repo.CandidateCreatorSource {
	sources := make([]repo.CandidateCreatorSource, 0, len(a.sourceOrder))
	for _, keyword := range a.sourceOrder {
		source := a.sources[keyword]
		if source == nil {
			continue
		}
		videos := make([]bilibili.VideoHit, 0, len(source.videoOrder))
		for _, videoID := range source.videoOrder {
			videos = append(videos, source.videos[videoID])
		}
		sort.Slice(videos, func(i, j int) bool {
			if videos[i].PublishTime.Equal(videos[j].PublishTime) {
				return videos[i].VideoID < videos[j].VideoID
			}
			return videos[i].PublishTime.After(videos[j].PublishTime)
		})
		if len(videos) > 5 {
			videos = videos[:5]
		}
		sources = append(sources, repo.CandidateCreatorSource{
			CandidateCreatorID: candidateID,
			SourceType:         "keyword",
			SourceValue:        keyword,
			SourceLabel:        "关键词：" + keyword,
			Weight:             source.weight,
			DetailJSON: marshalJSON(map[string]any{
				"keyword": keyword,
				"videos":  videos,
			}),
		})
	}
	return sources
}

func keywordRiskWeight(keyword string) int {
	switch {
	case containsAny(keyword, "补档", "重传", "未删减", "删减"):
		return 15
	case containsAny(keyword, "切片", "熟肉", "片段", "合集"):
		return 12
	default:
		if strings.TrimSpace(keyword) == "" {
			return 0
		}
		return 10
	}
}

func detectDeletionTrace(text string) []string {
	markers := []string{"补档", "重传", "删减", "未删减", "限时", "补发"}
	hits := make([]string, 0, len(markers))
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			hits = append(hits, marker)
		}
	}
	return hits
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func profileURLForUID(uid string) string {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return ""
	}
	return "https://space.bilibili.com/" + uid
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
