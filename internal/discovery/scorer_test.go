package discovery

import (
	"testing"

	"fetch-bilibili/internal/config"
)

func TestScorerAggregatesSignalsAndAppliesCaps(t *testing.T) {
	cfg := config.Default()
	cfg.Discovery.ScoreVersion = "v1"

	scorer := NewScorer(cfg.Discovery)
	result := scorer.Score(ScoreInput{
		KeywordHits: []KeywordHit{
			{Keyword: "补档", Score: 20},
			{Keyword: "重传", Score: 25},
		},
		Activity30d:       6,
		SimilarityLevel:   SimilarityStrong,
		DeletionTraceHits: []string{"补档", "删稿", "限时"},
		FollowerCount:     8_800,
		IgnoreCount:       1,
	})

	if result.ScoreVersion != "v1" {
		t.Fatalf("expected score version v1, got %q", result.ScoreVersion)
	}
	if result.Total != 90 {
		t.Fatalf("expected total score 90, got %d", result.Total)
	}

	got := make(map[string]int, len(result.Details))
	for _, detail := range result.Details {
		got[detail.FactorKey] = detail.ScoreDelta
	}
	if got["keyword_risk"] != 40 {
		t.Fatalf("expected keyword_risk 40, got %d", got["keyword_risk"])
	}
	if got["activity_30d"] != 15 {
		t.Fatalf("expected activity_30d 15, got %d", got["activity_30d"])
	}
	if got["similarity"] != 20 {
		t.Fatalf("expected similarity 20, got %d", got["similarity"])
	}
	if got["deletion_trace"] != 20 {
		t.Fatalf("expected deletion_trace 20, got %d", got["deletion_trace"])
	}
	if got["account_size"] != 10 {
		t.Fatalf("expected account_size 10, got %d", got["account_size"])
	}
	if got["feedback"] != -15 {
		t.Fatalf("expected feedback -15, got %d", got["feedback"])
	}
}

func TestScorerUsesConfiguredWeights(t *testing.T) {
	cfg := config.Default()
	cfg.Discovery.ScoreVersion = "custom-v2"
	cfg.Discovery.ScoreWeights.Similarity.Strong = 30
	cfg.Discovery.ScoreWeights.Feedback.IgnorePenalty = -20
	cfg.Discovery.ScoreWeights.KeywordRisk.Max = 50

	scorer := NewScorer(cfg.Discovery)
	result := scorer.Score(ScoreInput{
		KeywordHits:     []KeywordHit{{Keyword: "影视剪辑", Score: 50}},
		SimilarityLevel: SimilarityStrong,
		FollowerCount:   6_000_000,
		IgnoreCount:     1,
		Activity30d:     0,
	})

	if result.ScoreVersion != "custom-v2" {
		t.Fatalf("expected custom-v2, got %q", result.ScoreVersion)
	}
	if result.Total != 55 {
		t.Fatalf("expected total score 55, got %d", result.Total)
	}
}
