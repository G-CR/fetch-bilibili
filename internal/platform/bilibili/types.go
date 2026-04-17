package bilibili

import "time"

type VideoMeta struct {
	VideoID       string
	Title         string
	Description   string
	PublishTime   time.Time
	Duration      int
	CoverURL      string
	ViewCount     int64
	FavoriteCount int64
}

type CreatorHit struct {
	UID           string
	Name          string
	AvatarURL     string
	ProfileURL    string
	FollowerCount int64
	Signature     string
}

type VideoHit struct {
	UID           string
	CreatorName   string
	VideoID       string
	Title         string
	Description   string
	PublishTime   time.Time
	Duration      int
	CoverURL      string
	ViewCount     int64
	FavoriteCount int64
}
