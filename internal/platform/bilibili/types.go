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
