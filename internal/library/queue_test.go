package library

import (
	"context"
	"errors"
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

func TestSyncerHandleEventResolvesVideoCreatorWhenPayloadLacksCreatorID(t *testing.T) {
	syncer := NewSyncer(t.TempDir(), NewExporter(
		&exportTestCreators{},
		&exportTestVideos{
			findByID: map[int64]repo.Video{
				10: {ID: 10, CreatorID: 77},
			},
		},
	), nil, WithReconcileInterval(0))

	syncer.handleEvent(context.Background(), live.Event{
		Type:    "video.changed",
		Payload: map[string]any{"id": int(10)},
	})

	if got := <-syncer.queue; got != 77 {
		t.Fatalf("expected creator 77 queued, got %d", got)
	}
	if _, ok := syncer.pending[77]; !ok {
		t.Fatalf("expected creator 77 marked pending")
	}
}

func TestSyncerHandleEventIgnoresInvalidPayloads(t *testing.T) {
	syncer := NewSyncer(t.TempDir(), NewExporter(&exportTestCreators{}, &exportTestVideos{}), nil, WithReconcileInterval(0))

	events := []live.Event{
		{Type: "creator.changed", Payload: map[string]any{"id": int64(0)}},
		{Type: "video.changed", Payload: map[string]any{"id": "bad"}},
		{Type: "unknown.changed", Payload: map[string]any{"id": int64(1)}},
	}
	for _, event := range events {
		syncer.handleEvent(context.Background(), event)
	}

	select {
	case got := <-syncer.queue:
		t.Fatalf("expected no queued creator, got %d", got)
	default:
	}
}

func TestSyncerEnqueueCreatorDeduplicatesPendingAndMarksDirty(t *testing.T) {
	syncer := NewSyncer(t.TempDir(), nil, nil, WithReconcileInterval(0))

	syncer.enqueueCreator(2)
	syncer.enqueueCreator(2)
	if got := <-syncer.queue; got != 2 {
		t.Fatalf("expected creator 2 queued, got %d", got)
	}
	select {
	case got := <-syncer.queue:
		t.Fatalf("expected duplicate creator not queued, got %d", got)
	default:
	}

	syncer.processing[3] = struct{}{}
	syncer.enqueueCreator(3)
	if _, ok := syncer.dirty[3]; !ok {
		t.Fatalf("expected processing creator marked dirty")
	}
	select {
	case got := <-syncer.queue:
		t.Fatalf("expected dirty creator not queued immediately, got %d", got)
	default:
	}
}

func TestSyncerRebuildAllNoopAndErrorBranches(t *testing.T) {
	if err := (*Syncer)(nil).RebuildAll(context.Background()); err != nil {
		t.Fatalf("expected nil syncer no-op, got %v", err)
	}
	if err := NewSyncer(t.TempDir(), nil, nil).RebuildAll(context.Background()); err != nil {
		t.Fatalf("expected nil exporter no-op, got %v", err)
	}

	wantErr := repo.ErrNotImplemented
	syncer := NewSyncer(t.TempDir(), NewExporter(&exportTestCreators{listErr: wantErr}, &exportTestVideos{}), nil)
	if err := syncer.RebuildAll(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("expected list error %v, got %v", wantErr, err)
	}
}

func TestSyncerRemoveStaleCreatorDirectoriesSkipsNonCreatorEntries(t *testing.T) {
	root := t.TempDir()
	syncer := NewSyncer(root, nil, nil)
	libraryRoot := filepath.Join(root, "library")
	if err := os.MkdirAll(filepath.Join(libraryRoot, "bilibili"), 0o755); err != nil {
		t.Fatalf("mkdir platform: %v", err)
	}
	if err := os.WriteFile(filepath.Join(libraryRoot, "README.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("write platform file: %v", err)
	}

	if err := syncer.removeStaleCreatorDirectories(nil); err != nil {
		t.Fatalf("remove stale with missing creators dir: %v", err)
	}
}

func TestPayloadInt64AndTickerChan(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		want    int64
	}{
		{name: "not map", payload: "bad", want: 0},
		{name: "int64", payload: map[string]any{"id": int64(11)}, want: 11},
		{name: "int", payload: map[string]any{"id": int(12)}, want: 12},
		{name: "float64", payload: map[string]any{"id": float64(13)}, want: 13},
		{name: "unsupported", payload: map[string]any{"id": "14"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := payloadInt64(tc.payload, "id"); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
	if tickerChan(nil) != nil {
		t.Fatalf("expected nil ticker channel")
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	if tickerChan(ticker) == nil {
		t.Fatalf("expected ticker channel")
	}
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
