package repo

import "time"

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
