package repo

import (
	"encoding/json"
	"time"
)

type Creator struct {
	ID            int64
	Platform      string
	UID           string
	Name          string
	FollowerCount int64
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Video struct {
	ID            int64
	Platform      string
	VideoID       string
	CreatorID     int64
	Title         string
	Description   string
	PublishTime   time.Time
	Duration      int
	CoverURL      string
	ViewCount     int64
	FavoriteCount int64
	State         string
	OutOfPrintAt  time.Time
	StableAt      time.Time
	LastCheckAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type VideoFile struct {
	ID        int64
	VideoID   int64
	Path      string
	SizeBytes int64
	Checksum  string
	Type      string
	CreatedAt time.Time
}

type LibraryVideo struct {
	Video     Video
	FilePath  string
	SizeBytes int64
}

type CleanupCandidate struct {
	VideoID              int64
	SourceVideoID        string
	Platform             string
	Title                string
	State                string
	CreatorID            int64
	CreatorName          string
	CreatorFollowerCount int64
	ViewCount            int64
	FavoriteCount        int64
	FileID               int64
	FilePath             string
	FileSizeBytes        int64
	FileCreatedAt        time.Time
}

type Job struct {
	ID         int64
	Type       string
	Status     string
	Payload    map[string]any
	ErrorMsg   string
	NotBefore  time.Time
	StartedAt  time.Time
	FinishedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type VideoListFilter struct {
	Limit     int
	CreatorID int64
	State     string
}

type JobListFilter struct {
	Limit  int
	Status string
	Type   string
}

type CleanupCandidateFilter struct {
	Limit             int
	IncludeOutOfPrint bool
}

type CandidateCreator struct {
	ID               int64
	Platform         string
	UID              string
	Name             string
	AvatarURL        string
	ProfileURL       string
	FollowerCount    int64
	Status           string
	Score            int
	ScoreVersion     string
	LastDiscoveredAt time.Time
	LastScoredAt     time.Time
	ApprovedAt       time.Time
	IgnoredAt        time.Time
	BlockedAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CandidateCreatorSource struct {
	ID                 int64
	CandidateCreatorID int64
	SourceType         string
	SourceValue        string
	SourceLabel        string
	Weight             int
	DetailJSON         json.RawMessage
	CreatedAt          time.Time
}

type CandidateCreatorScoreDetail struct {
	ID                 int64
	CandidateCreatorID int64
	FactorKey          string
	FactorLabel        string
	ScoreDelta         int
	DetailJSON         json.RawMessage
	CreatedAt          time.Time
}

type CandidateListFilter struct {
	Status   string
	MinScore int
	Keyword  string
	Page     int
	PageSize int
}
