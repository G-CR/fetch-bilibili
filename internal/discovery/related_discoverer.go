package discovery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
)

const defaultRelatedSearchPageSize = 8
const defaultRelatedCreatorLimit = 200

type RelatedSourceClient interface {
	ListVideos(ctx context.Context, uid string) ([]bilibili.VideoMeta, error)
	SearchRelatedVideos(ctx context.Context, keyword string, page, pageSize int) ([]bilibili.VideoHit, error)
	SearchCreators(ctx context.Context, keyword string, page, pageSize int) ([]bilibili.CreatorHit, error)
}

type RelatedDiscoverResult struct {
	SourceCreators int
	Discovered     int
	SkippedTracked int
	SkippedBlocked int
}

type RelatedDiscoverer struct {
	creators   repo.CreatorRepository
	candidates repo.CandidateRepository
	client     RelatedSourceClient
	scorer     *Scorer
	cfg        config.DiscoveryConfig
	now        func() time.Time
	pageSize   int
}

type relatedCandidateLookup struct {
	candidate    repo.CandidateCreator
	sources      []repo.CandidateCreatorSource
	scoreDetails []repo.CandidateCreatorScoreDetail
	found        bool
}

type relatedCandidateHit struct {
	sourceCreator repo.Creator
	sourceVideo   bilibili.VideoMeta
	candidate     bilibili.VideoHit
	keyword       string
	similarity    string
}

type relatedSourceEntry struct {
	creator       repo.Creator
	keywords      []string
	sourceVideos  []bilibili.VideoMeta
	candidateHits []bilibili.VideoHit
	similarity    string
}

type relatedCandidateAggregate struct {
	uid              string
	existing         relatedCandidateLookup
	bestHit          relatedCandidateHit
	creator          bilibili.CreatorHit
	sourceOrder      []string
	sourcesByCreator map[string]*relatedSourceEntry
}

func NewRelatedDiscoverer(creators repo.CreatorRepository, candidates repo.CandidateRepository, client RelatedSourceClient, scorer *Scorer, cfg config.DiscoveryConfig) *RelatedDiscoverer {
	return &RelatedDiscoverer{
		creators:   creators,
		candidates: candidates,
		client:     client,
		scorer:     scorer,
		cfg:        cfg,
		now:        time.Now,
		pageSize:   defaultRelatedSearchPageSize,
	}
}

func (d *RelatedDiscoverer) Discover(ctx context.Context) (RelatedDiscoverResult, error) {
	if d.creators == nil || d.candidates == nil || d.client == nil || d.scorer == nil {
		return RelatedDiscoverResult{}, errors.New("关系扩散发现器未初始化")
	}

	trackedCreators, err := d.creators.ListActive(ctx, defaultRelatedCreatorLimit)
	if err != nil {
		return RelatedDiscoverResult{}, fmt.Errorf("读取已追踪博主失败: %w", err)
	}

	result := RelatedDiscoverResult{SourceCreators: len(trackedCreators)}
	if len(trackedCreators) == 0 {
		return result, nil
	}

	trackedSet := make(map[string]repo.Creator, len(trackedCreators))
	for _, creator := range trackedCreators {
		trackedSet[strings.TrimSpace(creator.UID)] = creator
	}

	now := d.now().UTC()
	existingCache := make(map[string]relatedCandidateLookup)
	creatorCache := make(map[string]bilibili.CreatorHit)
	aggregates := make(map[string]*relatedCandidateAggregate)
	order := make([]string, 0)
	skippedBlocked := make(map[string]struct{})
	skippedTracked := make(map[string]struct{})

	for _, creator := range trackedCreators {
		metas, err := d.client.ListVideos(ctx, creator.UID)
		if err != nil {
			if bilibili.IsRiskError(err) {
				continue
			}
			return result, fmt.Errorf("关系扩散失败: source_uid=%s 拉取公开视频失败: %w", creator.UID, err)
		}

		perCreator := collectRelatedHitsForCreator(ctx, d.candidates, d.client, creator, metas, trackedSet, existingCache, skippedBlocked, skippedTracked, d.pageSize, d.cfg.Keywords)
		if perCreator.err != nil {
			return result, perCreator.err
		}

		for _, hit := range limitRelatedHits(perCreator.hits, d.cfg.MaxRelatedPerCreator) {
			agg := aggregates[hit.candidate.UID]
			if agg == nil {
				lookup := existingCache[hit.candidate.UID]
				agg = &relatedCandidateAggregate{
					uid:              hit.candidate.UID,
					existing:         lookup,
					bestHit:          hit,
					sourcesByCreator: make(map[string]*relatedSourceEntry),
				}
				aggregates[hit.candidate.UID] = agg
				order = append(order, hit.candidate.UID)
			}
			agg.merge(hit)
		}
	}

	for _, uid := range order {
		agg := aggregates[uid]
		if agg == nil {
			continue
		}
		agg.enrichCreator(ctx, d.client, creatorCache)
		candidate := agg.buildCandidate(now, d.cfg.ScoreVersion)
		scoreDetails := agg.buildScoreDetails(d.scorer)
		candidate.Score = sumScoreDetails(scoreDetails)
		candidate.ScoreVersion = d.cfg.ScoreVersion
		candidate.LastScoredAt = now

		saved, err := d.candidates.Upsert(ctx, candidate)
		if err != nil {
			return result, fmt.Errorf("关系扩散写入候选失败: uid=%s: %w", uid, err)
		}
		sources := mergeCandidateSources(agg.existing.sources, agg.buildSources(saved.ID))
		if err := d.candidates.ReplaceSources(ctx, saved.ID, sources); err != nil {
			return result, fmt.Errorf("关系扩散写入候选来源失败: uid=%s: %w", uid, err)
		}
		for i := range scoreDetails {
			scoreDetails[i].CandidateCreatorID = saved.ID
		}
		if err := d.candidates.ReplaceScoreDetails(ctx, saved.ID, scoreDetails); err != nil {
			return result, fmt.Errorf("关系扩散写入评分明细失败: uid=%s: %w", uid, err)
		}
		result.Discovered++
	}

	result.SkippedBlocked = len(skippedBlocked)
	result.SkippedTracked = len(skippedTracked)
	return result, nil
}

type relatedPerCreatorResult struct {
	hits []relatedCandidateHit
	err  error
}

func collectRelatedHitsForCreator(
	ctx context.Context,
	candidates repo.CandidateRepository,
	client RelatedSourceClient,
	creator repo.Creator,
	metas []bilibili.VideoMeta,
	trackedSet map[string]repo.Creator,
	existingCache map[string]relatedCandidateLookup,
	skippedBlocked map[string]struct{},
	skippedTracked map[string]struct{},
	pageSize int,
	configuredKeywords []string,
) relatedPerCreatorResult {
	perCreator := make(map[string]relatedCandidateHit)
	for _, meta := range metas {
		keywords := buildRelatedKeywords(meta.Title, configuredKeywords)
		for _, keyword := range keywords {
			hits, err := client.SearchRelatedVideos(ctx, keyword, 1, pageSize)
			if err != nil {
				if bilibili.IsRiskError(err) {
					continue
				}
				return relatedPerCreatorResult{
					err: fmt.Errorf("关系扩散失败: source_uid=%s keyword=%s 搜索相似视频失败: %w", creator.UID, keyword, err),
				}
			}
			for _, hit := range hits {
				uid := strings.TrimSpace(hit.UID)
				if uid == "" {
					continue
				}
				if uid == creator.UID {
					skippedTracked[uid] = struct{}{}
					continue
				}
				if _, ok := trackedSet[uid]; ok {
					skippedTracked[uid] = struct{}{}
					continue
				}
				lookup, ok := existingCache[uid]
				if !ok {
					loaded, err := loadExistingCandidate(ctx, candidates, uid)
					if err != nil {
						return relatedPerCreatorResult{err: err}
					}
					existingCache[uid] = loaded
					lookup = loaded
				}
				if lookup.found && lookup.candidate.Status == "blocked" {
					skippedBlocked[uid] = struct{}{}
					continue
				}
				candidateHit := relatedCandidateHit{
					sourceCreator: creator,
					sourceVideo:   meta,
					candidate:     hit,
					keyword:       keyword,
					similarity:    classifyTitleSimilarity(meta.Title, hit.Title, keyword),
				}
				current, ok := perCreator[uid]
				if !ok || compareRelatedHit(candidateHit, current) < 0 {
					perCreator[uid] = candidateHit
				}
			}
		}
	}

	hits := make([]relatedCandidateHit, 0, len(perCreator))
	for _, hit := range perCreator {
		hits = append(hits, hit)
	}
	return relatedPerCreatorResult{hits: hits}
}

func loadExistingCandidate(
	ctx context.Context,
	candidates repo.CandidateRepository,
	uid string,
) (relatedCandidateLookup, error) {
	item, err := candidates.FindByPlatformUID(ctx, "bilibili", uid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return relatedCandidateLookup{}, nil
		}
		return relatedCandidateLookup{}, fmt.Errorf("查询候选失败: uid=%s: %w", uid, err)
	}
	sources, err := candidates.ListSources(ctx, item.ID)
	if err != nil {
		return relatedCandidateLookup{}, fmt.Errorf("读取候选来源失败: uid=%s: %w", uid, err)
	}
	scoreDetails, err := candidates.ListScoreDetails(ctx, item.ID)
	if err != nil {
		return relatedCandidateLookup{}, fmt.Errorf("读取候选评分明细失败: uid=%s: %w", uid, err)
	}
	return relatedCandidateLookup{
		candidate:    item,
		sources:      sources,
		scoreDetails: scoreDetails,
		found:        true,
	}, nil
}

func (a *relatedCandidateAggregate) merge(hit relatedCandidateHit) {
	if compareRelatedHit(hit, a.bestHit) < 0 {
		a.bestHit = hit
	}
	if strings.TrimSpace(a.creator.UID) == "" {
		a.creator = bilibili.CreatorHit{
			UID:        strings.TrimSpace(hit.candidate.UID),
			Name:       strings.TrimSpace(hit.candidate.CreatorName),
			ProfileURL: profileURLForUID(hit.candidate.UID),
		}
	}
	sourceUID := strings.TrimSpace(hit.sourceCreator.UID)
	entry := a.sourcesByCreator[sourceUID]
	if entry == nil {
		entry = &relatedSourceEntry{
			creator:    hit.sourceCreator,
			similarity: hit.similarity,
		}
		a.sourcesByCreator[sourceUID] = entry
		a.sourceOrder = append(a.sourceOrder, sourceUID)
	}
	entry.keywords = appendUniqueString(entry.keywords, hit.keyword)
	entry.sourceVideos = appendUniqueSourceVideo(entry.sourceVideos, hit.sourceVideo)
	entry.candidateHits = appendUniqueCandidateHit(entry.candidateHits, hit.candidate)
	if compareSimilarity(hit.similarity, entry.similarity) < 0 {
		entry.similarity = hit.similarity
	}
}

func (a *relatedCandidateAggregate) buildCandidate(now time.Time, scoreVersion string) repo.CandidateCreator {
	candidate := repo.CandidateCreator{
		ID:               a.existing.candidate.ID,
		Platform:         "bilibili",
		UID:              a.uid,
		Name:             firstNonEmptyString(a.creator.Name, a.bestHit.candidate.CreatorName),
		AvatarURL:        strings.TrimSpace(a.creator.AvatarURL),
		ProfileURL:       firstNonEmptyString(a.creator.ProfileURL, profileURLForUID(a.uid)),
		FollowerCount:    maxInt64(a.creator.FollowerCount, a.existing.candidate.FollowerCount),
		Status:           "reviewing",
		ScoreVersion:     scoreVersion,
		LastDiscoveredAt: now,
		ApprovedAt:       a.existing.candidate.ApprovedAt,
		IgnoredAt:        a.existing.candidate.IgnoredAt,
		BlockedAt:        a.existing.candidate.BlockedAt,
	}
	if a.existing.found {
		candidate.Status = strings.TrimSpace(a.existing.candidate.Status)
		if candidate.Status == "" {
			candidate.Status = "reviewing"
		}
		if candidate.Name == "" {
			candidate.Name = a.existing.candidate.Name
		}
		if candidate.AvatarURL == "" {
			candidate.AvatarURL = a.existing.candidate.AvatarURL
		}
		if candidate.ProfileURL == "" {
			candidate.ProfileURL = a.existing.candidate.ProfileURL
		}
	}
	return candidate
}

func (a *relatedCandidateAggregate) enrichCreator(ctx context.Context, client RelatedSourceClient, cache map[string]bilibili.CreatorHit) {
	if a == nil || client == nil {
		return
	}
	uid := strings.TrimSpace(a.uid)
	if uid == "" {
		return
	}
	if hit, ok := cache[uid]; ok {
		if hit.UID != "" {
			a.creator = mergeCreatorHit(a.creator, hit)
		}
		return
	}
	for _, query := range uniqueNonEmptyStrings(a.creator.Name, a.bestHit.candidate.CreatorName, uid) {
		hits, err := client.SearchCreators(ctx, query, 1, 10)
		if err != nil {
			continue
		}
		for _, hit := range hits {
			if strings.TrimSpace(hit.UID) != uid {
				continue
			}
			a.creator = mergeCreatorHit(a.creator, hit)
			cache[uid] = hit
			return
		}
	}
	cache[uid] = a.creator
}

func (a *relatedCandidateAggregate) buildSources(candidateID int64) []repo.CandidateCreatorSource {
	sources := make([]repo.CandidateCreatorSource, 0, len(a.sourceOrder))
	for _, sourceUID := range a.sourceOrder {
		entry := a.sourcesByCreator[sourceUID]
		if entry == nil {
			continue
		}
		sources = append(sources, repo.CandidateCreatorSource{
			CandidateCreatorID: candidateID,
			SourceType:         "related_creator",
			SourceValue:        sourceUID,
			SourceLabel:        "关联博主：" + entry.creator.Name,
			Weight:             similarityWeight(entry.similarity),
			DetailJSON: marshalJSON(map[string]any{
				"source_creator": map[string]any{
					"id":       entry.creator.ID,
					"uid":      entry.creator.UID,
					"name":     entry.creator.Name,
					"platform": entry.creator.Platform,
				},
				"keywords":         entry.keywords,
				"similarity_level": entry.similarity,
				"source_videos":    entry.sourceVideos,
				"matched_videos":   entry.candidateHits,
			}),
		})
	}
	return sources
}

func (a *relatedCandidateAggregate) buildScoreDetails(scorer *Scorer) []repo.CandidateCreatorScoreDetail {
	details := make([]repo.CandidateCreatorScoreDetail, 0, len(a.existing.scoreDetails)+1)
	for _, item := range a.existing.scoreDetails {
		if item.FactorKey == "similarity" {
			continue
		}
		details = append(details, item)
	}
	score := scorer.Score(ScoreInput{
		CandidateID:     a.existing.candidate.ID,
		SimilarityLevel: a.bestSimilarity(),
	})
	for _, item := range score.Details {
		if item.FactorKey == "similarity" {
			details = append(details, item)
		}
	}
	return details
}

func (a *relatedCandidateAggregate) bestSimilarity() string {
	best := SimilarityWeak
	for _, sourceUID := range a.sourceOrder {
		entry := a.sourcesByCreator[sourceUID]
		if entry == nil {
			continue
		}
		if compareSimilarity(entry.similarity, best) < 0 {
			best = entry.similarity
		}
	}
	return best
}

func mergeCandidateSources(existing, incoming []repo.CandidateCreatorSource) []repo.CandidateCreatorSource {
	type sourceKey struct {
		kind  string
		value string
	}
	merged := make(map[sourceKey]repo.CandidateCreatorSource, len(existing)+len(incoming))
	order := make([]sourceKey, 0, len(existing)+len(incoming))
	appendSource := func(item repo.CandidateCreatorSource) {
		key := sourceKey{kind: strings.TrimSpace(item.SourceType), value: strings.TrimSpace(item.SourceValue)}
		if _, ok := merged[key]; !ok {
			order = append(order, key)
		}
		merged[key] = item
	}
	for _, item := range existing {
		appendSource(item)
	}
	for _, item := range incoming {
		appendSource(item)
	}

	result := make([]repo.CandidateCreatorSource, 0, len(order))
	for _, key := range order {
		result = append(result, merged[key])
	}
	return result
}

func limitRelatedHits(items []relatedCandidateHit, limit int) []relatedCandidateHit {
	sort.Slice(items, func(i, j int) bool {
		return compareRelatedHit(items[i], items[j]) < 0
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func compareRelatedHit(left, right relatedCandidateHit) int {
	if diff := compareSimilarity(left.similarity, right.similarity); diff != 0 {
		return diff
	}
	if !left.candidate.PublishTime.Equal(right.candidate.PublishTime) {
		if left.candidate.PublishTime.After(right.candidate.PublishTime) {
			return -1
		}
		return 1
	}
	if left.candidate.ViewCount != right.candidate.ViewCount {
		if left.candidate.ViewCount > right.candidate.ViewCount {
			return -1
		}
		return 1
	}
	return strings.Compare(left.candidate.UID, right.candidate.UID)
}

func classifyTitleSimilarity(sourceTitle, candidateTitle, keyword string) string {
	sourceNormalized := normalizeTitle(sourceTitle)
	candidateNormalized := normalizeTitle(candidateTitle)
	keywordNormalized := normalizeTitle(keyword)
	switch {
	case sourceNormalized != "" && sourceNormalized == candidateNormalized:
		return SimilarityStrong
	case sourceNormalized != "" && candidateNormalized != "" &&
		(strings.Contains(sourceNormalized, candidateNormalized) || strings.Contains(candidateNormalized, sourceNormalized)):
		return SimilarityStrong
	case hasSharedTerm(sourceTitle, candidateTitle, keyword):
		return SimilarityMedium
	case keywordNormalized != "" && strings.Contains(sourceNormalized, keywordNormalized) && strings.Contains(candidateNormalized, keywordNormalized):
		return SimilarityWeak
	default:
		return SimilarityWeak
	}
}

func hasSharedTerm(sourceTitle, candidateTitle, keyword string) bool {
	sourceTerms := splitTitleTerms(sourceTitle)
	candidateTerms := splitTitleTerms(candidateTitle)
	if len(sourceTerms) == 0 || len(candidateTerms) == 0 {
		return false
	}
	ignored := normalizeTitle(keyword)
	seen := make(map[string]struct{}, len(sourceTerms))
	for _, term := range sourceTerms {
		normalized := normalizeTitle(term)
		if normalized == "" || normalized == ignored {
			continue
		}
		seen[normalized] = struct{}{}
	}
	for _, term := range candidateTerms {
		normalized := normalizeTitle(term)
		if normalized == "" || normalized == ignored {
			continue
		}
		if _, ok := seen[normalized]; ok {
			return true
		}
	}
	return false
}

func splitTitleTerms(title string) []string {
	fields := strings.FieldsFunc(title, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("|丨/\\-_【】[]（）()·:：,，。！？!?", r)
	})
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if utf8Len(field) < 2 || utf8Len(field) > 18 {
			continue
		}
		terms = append(terms, field)
	}
	return terms
}

func buildRelatedKeywords(title string, configured []string) []string {
	keywords := make([]string, 0, 4)
	for _, keyword := range configured {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		if strings.Contains(title, keyword) {
			keywords = appendUniqueString(keywords, keyword)
		}
	}
	if len(keywords) > 0 {
		return keywords
	}
	for _, term := range splitTitleTerms(title) {
		keywords = appendUniqueString(keywords, term)
		if len(keywords) >= 3 {
			break
		}
	}
	return keywords
}

func compareSimilarity(left, right string) int {
	return similarityRank(left) - similarityRank(right)
}

func similarityRank(level string) int {
	switch strings.TrimSpace(level) {
	case SimilarityStrong:
		return 0
	case SimilarityMedium:
		return 1
	default:
		return 2
	}
}

func similarityWeight(level string) int {
	switch strings.TrimSpace(level) {
	case SimilarityStrong:
		return 20
	case SimilarityMedium:
		return 10
	default:
		return 5
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func uniqueNonEmptyStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func normalizeTitle(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsSpace(r) || strings.ContainsRune("|丨/\\-_【】[]（）()·:：,，。！？!?", r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sumScoreDetails(items []repo.CandidateCreatorScoreDetail) int {
	total := 0
	for _, item := range items {
		total += item.ScoreDelta
	}
	return total
}

func appendUniqueString(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func appendUniqueSourceVideo(items []bilibili.VideoMeta, value bilibili.VideoMeta) []bilibili.VideoMeta {
	for _, item := range items {
		if item.VideoID == value.VideoID {
			return items
		}
	}
	return append(items, value)
}

func appendUniqueCandidateHit(items []bilibili.VideoHit, value bilibili.VideoHit) []bilibili.VideoHit {
	for _, item := range items {
		if item.VideoID == value.VideoID {
			return items
		}
	}
	return append(items, value)
}

func utf8Len(value string) int {
	return len([]rune(value))
}

type CompositeDiscoverer struct {
	keyword *KeywordDiscoverer
	related *RelatedDiscoverer
}

func NewCompositeDiscoverer(keyword *KeywordDiscoverer, related *RelatedDiscoverer) *CompositeDiscoverer {
	return &CompositeDiscoverer{
		keyword: keyword,
		related: related,
	}
}

func (d *CompositeDiscoverer) Discover(ctx context.Context) (KeywordDiscoverResult, error) {
	var result KeywordDiscoverResult
	if d.keyword != nil {
		keywordResult, err := d.keyword.Discover(ctx)
		if err != nil {
			return KeywordDiscoverResult{}, err
		}
		result = keywordResult
	}
	if d.related != nil {
		relatedResult, err := d.related.Discover(ctx)
		if err != nil {
			return KeywordDiscoverResult{}, err
		}
		result.Discovered += relatedResult.Discovered
		result.SkippedBlocked += relatedResult.SkippedBlocked
	}
	return result, nil
}
