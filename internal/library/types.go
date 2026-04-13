package library

import "time"

const (
	ManifestVersion = 1

	StateNew         = "NEW"
	StateDownloading = "DOWNLOADING"
	StateDownloaded  = "DOWNLOADED"
	StateStable      = "STABLE"
	StateOutOfPrint  = "OUT_OF_PRINT"
	StateDeleted     = "DELETED"
)

type CreatorSnapshot struct {
	Platform      string
	UID           string
	Name          string
	Status        string
	FollowerCount int64
	Videos        []VideoSnapshot
}

type VideoSnapshot struct {
	VideoID       string
	Title         string
	State         string
	PublishTime   time.Time
	OutOfPrintAt  time.Time
	StableAt      time.Time
	ViewCount     int64
	FavoriteCount int64
	FilePath      string
	SizeBytes     int64
}

type CreatorManifest struct {
	ManifestVersion int       `json:"manifest_version"`
	GeneratedAt     time.Time `json:"generated_at"`
	Platform        string    `json:"platform"`
	UID             string    `json:"uid"`
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	FollowerCount   int64     `json:"follower_count"`
	LocalVideoCount int       `json:"local_video_count"`
	LocalRareCount  int       `json:"local_rare_count"`
	StorageBytes    int64     `json:"storage_bytes"`
	Directory       string    `json:"directory"`
}

type IndexManifest struct {
	ManifestVersion int              `json:"manifest_version"`
	GeneratedAt     time.Time        `json:"generated_at"`
	Platform        string           `json:"platform"`
	UID             string           `json:"uid"`
	Videos          []IndexVideoItem `json:"videos"`
}

type IndexVideoItem struct {
	VideoID       string    `json:"video_id"`
	Title         string    `json:"title"`
	State         string    `json:"state"`
	PublishTime   time.Time `json:"publish_time"`
	OutOfPrintAt  time.Time `json:"out_of_print_at,omitempty"`
	StableAt      time.Time `json:"stable_at,omitempty"`
	RelativePath  string    `json:"relative_path"`
	SizeBytes     int64     `json:"size_bytes"`
	ViewCount     int64     `json:"view_count"`
	FavoriteCount int64     `json:"favorite_count"`
}
