package library

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"fetch-bilibili/internal/repo"
)

type exportTestCreators struct {
	findByID    map[int64]repo.Creator
	listForLib  [][]repo.Creator
	findErr     error
	listErr     error
	listCalls    int
}

func (f *exportTestCreators) Upsert(ctx context.Context, c repo.Creator) (int64, error) {
	return 0, repo.ErrNotImplemented
}
func (f *exportTestCreators) Create(ctx context.Context, c repo.Creator) (int64, error) {
	return 0, repo.ErrNotImplemented
}
func (f *exportTestCreators) Update(ctx context.Context, c repo.Creator) error {
	return repo.ErrNotImplemented
}
func (f *exportTestCreators) UpdateStatus(ctx context.Context, id int64, status string) error {
	return repo.ErrNotImplemented
}
func (f *exportTestCreators) DeleteByID(ctx context.Context, id int64) (int64, error) {
	return 0, repo.ErrNotImplemented
}
func (f *exportTestCreators) FindByID(ctx context.Context, id int64) (repo.Creator, error) {
	if f.findErr != nil {
		return repo.Creator{}, f.findErr
	}
	if creator, ok := f.findByID[id]; ok {
		return creator, nil
	}
	return repo.Creator{}, sql.ErrNoRows
}
func (f *exportTestCreators) FindByPlatformUID(ctx context.Context, platform, uid string) (repo.Creator, error) {
	return repo.Creator{}, repo.ErrNotImplemented
}
func (f *exportTestCreators) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	return nil, repo.ErrNotImplemented
}
func (f *exportTestCreators) ListActiveAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	return nil, repo.ErrNotImplemented
}
func (f *exportTestCreators) CountActive(ctx context.Context) (int64, error) {
	return 0, repo.ErrNotImplemented
}
func (f *exportTestCreators) ListForLibraryAfter(ctx context.Context, lastID int64, limit int) ([]repo.Creator, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listCalls >= len(f.listForLib) {
		return nil, nil
	}
	list := f.listForLib[f.listCalls]
	f.listCalls++
	return list, nil
}

type exportTestVideos struct {
	findByID  map[int64]repo.Video
	listByCID map[int64][]repo.LibraryVideo
	findErr   error
	listErr   error
}

func (f *exportTestVideos) Upsert(ctx context.Context, v repo.Video) (int64, bool, error) {
	return 0, false, repo.ErrNotImplemented
}
func (f *exportTestVideos) UpdateState(ctx context.Context, id int64, state string) error {
	return repo.ErrNotImplemented
}
func (f *exportTestVideos) FindByID(ctx context.Context, id int64) (repo.Video, error) {
	if f.findErr != nil {
		return repo.Video{}, f.findErr
	}
	if video, ok := f.findByID[id]; ok {
		return video, nil
	}
	return repo.Video{}, sql.ErrNoRows
}
func (f *exportTestVideos) ListForCheck(ctx context.Context, limit int) ([]repo.Video, error) {
	return nil, repo.ErrNotImplemented
}
func (f *exportTestVideos) ListRecent(ctx context.Context, filter repo.VideoListFilter) ([]repo.Video, error) {
	return nil, repo.ErrNotImplemented
}
func (f *exportTestVideos) ListCleanupCandidates(ctx context.Context, filter repo.CleanupCandidateFilter) ([]repo.CleanupCandidate, error) {
	return nil, repo.ErrNotImplemented
}
func (f *exportTestVideos) CountByState(ctx context.Context, state string) (int64, error) {
	return 0, repo.ErrNotImplemented
}
func (f *exportTestVideos) UpdateCheckStatus(ctx context.Context, id int64, state string, outOfPrintAt *time.Time, stableAt *time.Time, lastCheckAt time.Time) error {
	return repo.ErrNotImplemented
}
func (f *exportTestVideos) ListLibraryByCreator(ctx context.Context, creatorID int64) ([]repo.LibraryVideo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listByCID[creatorID], nil
}

func TestExporterExportCreatorBuildsSnapshotFromRepo(t *testing.T) {
	exporter := NewExporter(
		&exportTestCreators{
			findByID: map[int64]repo.Creator{
				7: {
					ID:            7,
					Platform:      "bilibili",
					UID:           "352981594",
					Name:          "猫南北",
					Status:        "paused",
					FollowerCount: 99,
				},
			},
		},
		&exportTestVideos{
			listByCID: map[int64][]repo.LibraryVideo{
				7: {
					{
						Video: repo.Video{
							ID:            100,
							Platform:      "bilibili",
							VideoID:       "BV1",
							CreatorID:     7,
							Title:         "绝版",
							State:         "OUT_OF_PRINT",
							PublishTime:   time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
							OutOfPrintAt:  time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
							ViewCount:     12,
							FavoriteCount: 3,
						},
						FilePath:  "/data/store/bilibili/BV1.mp4",
						SizeBytes: 1024,
					},
					{
						Video: repo.Video{
							ID:          101,
							Platform:    "bilibili",
							VideoID:     "BV2",
							CreatorID:   7,
							Title:       "普通",
							State:       "DOWNLOADED",
							PublishTime: time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
						},
						FilePath:  "/data/store/bilibili/BV2.mp4",
						SizeBytes: 2048,
					},
				},
			},
		},
	)

	snapshot, err := exporter.ExportCreator(context.Background(), 7)
	if err != nil {
		t.Fatalf("export creator: %v", err)
	}
	if snapshot.UID != "352981594" || snapshot.Name != "猫南北" || snapshot.Status != "paused" {
		t.Fatalf("unexpected creator snapshot: %+v", snapshot)
	}
	if len(snapshot.Videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(snapshot.Videos))
	}
	if snapshot.Videos[0].VideoID != "BV1" || snapshot.Videos[0].FilePath == "" {
		t.Fatalf("unexpected first video snapshot: %+v", snapshot.Videos[0])
	}
}

func TestExporterListCreatorsForRebuildPages(t *testing.T) {
	exporter := NewExporter(
		&exportTestCreators{
			listForLib: [][]repo.Creator{
				{
					{ID: 1, UID: "u1", Platform: "bilibili"},
					{ID: 2, UID: "u2", Platform: "bilibili"},
				},
				{
					{ID: 3, UID: "u3", Platform: "bilibili"},
				},
				nil,
			},
		},
		&exportTestVideos{},
	)

	list, err := exporter.ListCreatorsForRebuild(context.Background(), 2)
	if err != nil {
		t.Fatalf("list creators: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 creators, got %d", len(list))
	}
	if list[2].ID != 3 {
		t.Fatalf("unexpected last creator: %+v", list[2])
	}
}

func TestExporterCreatorIDForVideo(t *testing.T) {
	exporter := NewExporter(
		&exportTestCreators{},
		&exportTestVideos{
			findByID: map[int64]repo.Video{
				11: {ID: 11, CreatorID: 22},
			},
		},
	)

	creatorID, err := exporter.CreatorIDForVideo(context.Background(), 11)
	if err != nil {
		t.Fatalf("creator id for video: %v", err)
	}
	if creatorID != 22 {
		t.Fatalf("expected creator id 22, got %d", creatorID)
	}
}

func TestExporterCreatorIDForVideoNotFound(t *testing.T) {
	exporter := NewExporter(&exportTestCreators{}, &exportTestVideos{})
	if _, err := exporter.CreatorIDForVideo(context.Background(), 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
