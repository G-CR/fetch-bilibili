package discovery

import (
	"encoding/json"
	"strings"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/repo"
)

const (
	smallAccountFollowerThreshold    int64 = 500_000
	oversizeAccountFollowerThreshold int64 = 5_000_000
)

type ScoreResult struct {
	Total        int
	ScoreVersion string
	Details      []repo.CandidateCreatorScoreDetail
}

type Scorer struct {
	cfg config.DiscoveryConfig
}

func NewScorer(cfg config.DiscoveryConfig) *Scorer {
	return &Scorer{cfg: cfg}
}

func (s *Scorer) Score(input ScoreInput) ScoreResult {
	result := ScoreResult{
		ScoreVersion: s.cfg.ScoreVersion,
	}

	if detail := s.scoreKeywordRisk(input); detail != nil {
		result.Total += detail.ScoreDelta
		result.Details = append(result.Details, *detail)
	}
	if detail := s.scoreActivity(input); detail != nil {
		result.Total += detail.ScoreDelta
		result.Details = append(result.Details, *detail)
	}
	if detail := s.scoreSimilarity(input); detail != nil {
		result.Total += detail.ScoreDelta
		result.Details = append(result.Details, *detail)
	}
	if detail := s.scoreDeletionTrace(input); detail != nil {
		result.Total += detail.ScoreDelta
		result.Details = append(result.Details, *detail)
	}
	if detail := s.scoreAccountSize(input); detail != nil {
		result.Total += detail.ScoreDelta
		result.Details = append(result.Details, *detail)
	}
	if detail := s.scoreFeedback(input); detail != nil {
		result.Total += detail.ScoreDelta
		result.Details = append(result.Details, *detail)
	}

	return result
}

func (s *Scorer) scoreKeywordRisk(input ScoreInput) *repo.CandidateCreatorScoreDetail {
	if len(input.KeywordHits) == 0 {
		return nil
	}

	total := 0
	keywords := make([]string, 0, len(input.KeywordHits))
	for _, hit := range input.KeywordHits {
		if hit.Score <= 0 {
			continue
		}
		total += hit.Score
		if keyword := strings.TrimSpace(hit.Keyword); keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	if total <= 0 {
		return nil
	}
	if max := s.cfg.ScoreWeights.KeywordRisk.Max; max > 0 && total > max {
		total = max
	}
	return &repo.CandidateCreatorScoreDetail{
		CandidateCreatorID: input.CandidateID,
		FactorKey:          "keyword_risk",
		FactorLabel:        "命中高风险关键词",
		ScoreDelta:         total,
		DetailJSON:         marshalJSON(map[string]any{"keywords": keywords}),
	}
}

func (s *Scorer) scoreActivity(input ScoreInput) *repo.CandidateCreatorScoreDetail {
	score := 0
	switch {
	case input.Activity30d >= 6:
		score = s.cfg.ScoreWeights.Activity30D.High
	case input.Activity30d >= 3:
		score = s.cfg.ScoreWeights.Activity30D.Medium
	case input.Activity30d >= 1:
		score = s.cfg.ScoreWeights.Activity30D.Low
	}
	if score == 0 {
		return nil
	}
	return &repo.CandidateCreatorScoreDetail{
		CandidateCreatorID: input.CandidateID,
		FactorKey:          "activity_30d",
		FactorLabel:        "最近 30 天更新活跃",
		ScoreDelta:         score,
		DetailJSON:         marshalJSON(map[string]any{"video_count": input.Activity30d}),
	}
}

func (s *Scorer) scoreSimilarity(input ScoreInput) *repo.CandidateCreatorScoreDetail {
	score := 0
	switch strings.TrimSpace(input.SimilarityLevel) {
	case SimilarityWeak:
		score = s.cfg.ScoreWeights.Similarity.Weak
	case SimilarityMedium:
		score = s.cfg.ScoreWeights.Similarity.Medium
	case SimilarityStrong:
		score = s.cfg.ScoreWeights.Similarity.Strong
	}
	if score == 0 {
		return nil
	}
	return &repo.CandidateCreatorScoreDetail{
		CandidateCreatorID: input.CandidateID,
		FactorKey:          "similarity",
		FactorLabel:        "与已追踪池内容相似",
		ScoreDelta:         score,
		DetailJSON:         marshalJSON(map[string]any{"level": input.SimilarityLevel}),
	}
}

func (s *Scorer) scoreDeletionTrace(input ScoreInput) *repo.CandidateCreatorScoreDetail {
	hits := uniqueStrings(input.DeletionTraceHits)
	if len(hits) == 0 {
		return nil
	}

	score := len(hits) * s.cfg.ScoreWeights.DeletionTrace.Single
	if max := s.cfg.ScoreWeights.DeletionTrace.Max; max > 0 && score > max {
		score = max
	}
	if score == 0 {
		return nil
	}
	return &repo.CandidateCreatorScoreDetail{
		CandidateCreatorID: input.CandidateID,
		FactorKey:          "deletion_trace",
		FactorLabel:        "命中删稿/补档痕迹",
		ScoreDelta:         score,
		DetailJSON:         marshalJSON(map[string]any{"hits": hits}),
	}
}

func (s *Scorer) scoreAccountSize(input ScoreInput) *repo.CandidateCreatorScoreDetail {
	score := 0
	label := ""
	switch {
	case input.FollowerCount > 0 && input.FollowerCount <= smallAccountFollowerThreshold:
		score = s.cfg.ScoreWeights.AccountSize.SmallBonus
		label = "中小号加权"
	case input.FollowerCount >= oversizeAccountFollowerThreshold:
		score = s.cfg.ScoreWeights.AccountSize.OversizePenalty
		label = "超大号惩罚"
	}
	if score == 0 {
		return nil
	}
	return &repo.CandidateCreatorScoreDetail{
		CandidateCreatorID: input.CandidateID,
		FactorKey:          "account_size",
		FactorLabel:        label,
		ScoreDelta:         score,
		DetailJSON:         marshalJSON(map[string]any{"follower_count": input.FollowerCount}),
	}
}

func (s *Scorer) scoreFeedback(input ScoreInput) *repo.CandidateCreatorScoreDetail {
	if input.IgnoreCount <= 0 || s.cfg.ScoreWeights.Feedback.IgnorePenalty == 0 {
		return nil
	}
	score := input.IgnoreCount * s.cfg.ScoreWeights.Feedback.IgnorePenalty
	return &repo.CandidateCreatorScoreDetail{
		CandidateCreatorID: input.CandidateID,
		FactorKey:          "feedback",
		FactorLabel:        "人工反馈惩罚",
		ScoreDelta:         score,
		DetailJSON:         marshalJSON(map[string]any{"ignore_count": input.IgnoreCount}),
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func marshalJSON(v any) []byte {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}
