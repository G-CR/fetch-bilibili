package discovery

const (
	SimilarityWeak   = "weak"
	SimilarityMedium = "medium"
	SimilarityStrong = "strong"
)

type KeywordHit struct {
	Keyword string
	Score   int
}

type ScoreInput struct {
	CandidateID       int64
	KeywordHits       []KeywordHit
	Activity30d       int
	SimilarityLevel   string
	DeletionTraceHits []string
	FollowerCount     int64
	IgnoreCount       int
}
