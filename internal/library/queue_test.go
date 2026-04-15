package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

func TestSyncerStartConsumesBrokerEventsAndDeduplicatesCreatorRebuild(t *testing.T) {
	root := t.TempDir()
	storePath := seedStoreFile(t, root, "bilibili", "BV-sync", "sync")

	creators := &exportTestCreators{
		findByID: map[int64]repo.Creator{
			1: {
				ID:       1,
				Platform: "bilibili",
				UID:      "352981594",
				Name:     "同步博主",
				Status:   "active",
			},
		},
	}
	videos := &exportTestVideos{
		listByCID: map[int64][]repo.LibraryVideo{
			1: {
				{
					Video: repo.Video{
						ID:          10,
						Platform:    "bilibili",
						VideoID:     "BV-sync",
						CreatorID:   1,
						Title:       "同步视频",
						State:       "DOWNLOADED",
						PublishTime: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
					},
					FilePath:  storePath,
					SizeBytes: 4,
				},
			},
		},
		findByID: map[int64]repo.Video{
			10: {ID: 10, CreatorID: 1},
		},
	}

	broker := live.NewBroker()
	syncer := NewSyncer(root, NewExporter(creators, videos), broker, WithReconcileInterval(0))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go syncer.Start(ctx)

	broker.Publish(live.Event{
		ID:   "creator-1",
		Type: "creator.changed",
		At:   time.Now(),
		Payload: map[string]any{
			"id": int64(1),
		},
	})
	broker.Publish(live.Event{
		ID:   "video-10",
		Type: "video.changed",
		At:   time.Now(),
		Payload: map[string]any{
			"id":         int64(10),
			"creator_id": int64(1),
		},
	})

	creatorDir := filepath.Join(root, "library", "bilibili", "creators", "352981594_同步博主")
	waitForPath(t, filepath.Join(creatorDir, "_meta", "index.json"))
	assertSymlinkTarget(t, filepath.Join(creatorDir, "videos", "BV-sync.mp4"), storePath)
}

func TestSyncerRebuildAllRemovesStaleCreatorDirectories(t *testing.T) {
	root := t.TempDir()
	projectedPath := seedStoreFile(t, root, "bilibili", "BV-current", "current")
	staleDir := filepath.Join(root, "library", "bilibili", "creators", "999999999_旧博主")
	if err := os.MkdirAll(filepath.Join(staleDir, "_meta"), 0o755); err != nil {
		t.Fatalf("mkdir stale dir: %v", err)
	}

	creators := &exportTestCreators{
		findByID: map[int64]repo.Creator{
			1: {
				ID:       1,
				Platform: "bilibili",
				UID:      "352981594",
				Name:     "现有博主",
				Status:   "active",
			},
		},
		listForLib: [][]repo.Creator{
			{
				{ID: 1, Platform: "bilibili", UID: "352981594", Name: "现有博主", Status: "active"},
			},
			nil,
		},
	}
	videos := &exportTestVideos{
		listByCID: map[int64][]repo.LibraryVideo{
			1: {
				{
					Video: repo.Video{
						ID:        20,
						Platform:  "bilibili",
						VideoID:   "BV-current",
						CreatorID: 1,
						Title:     "现有视频",
						State:     "DOWNLOADED",
					},
					FilePath:  projectedPath,
					SizeBytes: 7,
				},
			},
		},
	}

	syncer := NewSyncer(root, NewExporter(creators, videos), nil, WithReconcileInterval(0))
	if err := syncer.RebuildAll(context.Background()); err != nil {
		t.Fatalf("rebuild all: %v", err)
	}

	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("expected stale dir removed, got err=%v", err)
	}
	currentDir := filepath.Join(root, "library", "bilibili", "creators", "352981594_现有博主")
	assertSymlinkTarget(t, filepath.Join(currentDir, "videos", "BV-current.mp4"), projectedPath)
}

func waitForPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for path %s", path)
}
