package library

import (
	"context"

	"fetch-bilibili/internal/repo"
)

type Exporter struct {
	creators repo.CreatorRepository
	videos   repo.VideoRepository
}

func NewExporter(creators repo.CreatorRepository, videos repo.VideoRepository) *Exporter {
	return &Exporter{
		creators: creators,
		videos:   videos,
	}
}

func (e *Exporter) ExportCreator(ctx context.Context, creatorID int64) (CreatorSnapshot, error) {
	creator, err := e.creators.FindByID(ctx, creatorID)
	if err != nil {
		return CreatorSnapshot{}, err
	}
	items, err := e.videos.ListLibraryByCreator(ctx, creatorID)
	if err != nil {
		return CreatorSnapshot{}, err
	}

	snapshot := CreatorSnapshot{
		Platform:      creator.Platform,
		UID:           creator.UID,
		Name:          creator.Name,
		Status:        creator.Status,
		FollowerCount: creator.FollowerCount,
		Videos:        make([]VideoSnapshot, 0, len(items)),
	}
	for _, item := range items {
		video := item.Video
		snapshot.Videos = append(snapshot.Videos, VideoSnapshot{
			VideoID:       video.VideoID,
			Title:         video.Title,
			State:         video.State,
			PublishTime:   video.PublishTime,
			OutOfPrintAt:  video.OutOfPrintAt,
			StableAt:      video.StableAt,
			ViewCount:     video.ViewCount,
			FavoriteCount: video.FavoriteCount,
			FilePath:      item.FilePath,
			SizeBytes:     item.SizeBytes,
		})
	}
	return snapshot, nil
}

func (e *Exporter) ListCreatorsForRebuild(ctx context.Context, pageSize int) ([]repo.Creator, error) {
	if pageSize <= 0 {
		pageSize = 100
	}

	var (
		lastID int64
		out    []repo.Creator
	)
	for {
		items, err := e.creators.ListForLibraryAfter(ctx, lastID, pageSize)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return out, nil
		}
		out = append(out, items...)
		lastID = items[len(items)-1].ID
	}
}

func (e *Exporter) CreatorIDForVideo(ctx context.Context, videoID int64) (int64, error) {
	video, err := e.videos.FindByID(ctx, videoID)
	if err != nil {
		return 0, err
	}
	return video.CreatorID, nil
}
