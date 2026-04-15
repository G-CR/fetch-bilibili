package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreVideoPathDefaultsPlatform(t *testing.T) {
	got := StoreVideoPath("/data/archive", "", "BV1")
	want := filepath.Join("/data/archive", "store", "bilibili", "BV1.mp4")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestCreatorDirectoryPathSanitizesName(t *testing.T) {
	got := CreatorDirectoryPath("/data/archive", CreatorSnapshot{
		Platform: "bilibili",
		UID:      "352981594",
		Name:     "猫南北/official",
	})
	want := filepath.Join("/data/archive", "library", "bilibili", "creators", "352981594_猫南北_official")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestCreatorDirectoryPathFallsBackToUnknown(t *testing.T) {
	got := CreatorDirectoryName("352981594", " / ")
	want := "352981594_unknown"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestProjectorRebuildRoutesVideosAndWritesMetadata(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
	projector := NewProjector(root, WithClock(func() time.Time { return now }))

	downPath := seedStoreFile(t, root, "bilibili", "BV-down", "down")
	stablePath := seedStoreFile(t, root, "bilibili", "BV-stable", "stable")
	rarePath := seedStoreFile(t, root, "bilibili", "BV-rare", "rare")
	seedStoreFile(t, root, "bilibili", "BV-new", "new")
	missingPath := StoreVideoPath(root, "bilibili", "BV-missing")

	snapshot := CreatorSnapshot{
		Platform:      "bilibili",
		UID:           "352981594",
		Name:          "我是猫南北",
		Status:        "active",
		FollowerCount: 88,
		Videos: []VideoSnapshot{
			{
				VideoID:     "BV-down",
				Title:       "普通视频",
				State:       StateDownloaded,
				PublishTime: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
				FilePath:    downPath,
			},
			{
				VideoID:       "BV-rare",
				Title:         "绝版视频",
				State:         StateOutOfPrint,
				PublishTime:   time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
				OutOfPrintAt:  time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC),
				FilePath:      rarePath,
				SizeBytes:     0,
				ViewCount:     321,
				FavoriteCount: 12,
			},
			{
				VideoID:     "BV-stable",
				Title:       "稳定视频",
				State:       StateStable,
				PublishTime: time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC),
				StableAt:    time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
				FilePath:    stablePath,
			},
			{
				VideoID:  "BV-new",
				Title:    "新视频",
				State:    StateNew,
				FilePath: StoreVideoPath(root, "bilibili", "BV-new"),
			},
			{
				VideoID:  "BV-missing",
				Title:    "丢失视频",
				State:    StateDownloaded,
				FilePath: missingPath,
			},
		},
	}

	if err := projector.RebuildCreator(context.Background(), snapshot); err != nil {
		t.Fatalf("rebuild creator: %v", err)
	}

	creatorDir := CreatorDirectoryPath(root, snapshot)

	assertSymlinkTarget(t, filepath.Join(creatorDir, "videos", "BV-down.mp4"), downPath)
	assertSymlinkTarget(t, filepath.Join(creatorDir, "videos", "BV-stable.mp4"), stablePath)
	assertSymlinkTarget(t, filepath.Join(creatorDir, "rare", "BV-rare.mp4"), rarePath)

	if _, err := os.Lstat(filepath.Join(creatorDir, "videos", "BV-new.mp4")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected NEW video skipped, got err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(creatorDir, "videos", "BV-missing.mp4")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing video skipped, got err=%v", err)
	}

	var creatorMeta CreatorManifest
	readJSONFile(t, filepath.Join(creatorDir, "_meta", "creator.json"), &creatorMeta)
	if creatorMeta.Directory != filepath.Base(creatorDir) {
		t.Fatalf("expected directory %s, got %s", filepath.Base(creatorDir), creatorMeta.Directory)
	}
	if creatorMeta.LocalVideoCount != 2 {
		t.Fatalf("expected local video count 2, got %d", creatorMeta.LocalVideoCount)
	}
	if creatorMeta.LocalRareCount != 1 {
		t.Fatalf("expected local rare count 1, got %d", creatorMeta.LocalRareCount)
	}

	var index IndexManifest
	readJSONFile(t, filepath.Join(creatorDir, "_meta", "index.json"), &index)
	if len(index.Videos) != 3 {
		t.Fatalf("expected 3 indexed videos, got %d", len(index.Videos))
	}
	gotPaths := make(map[string]string, len(index.Videos))
	for _, item := range index.Videos {
		gotPaths[item.VideoID] = item.RelativePath
	}
	if gotPaths["BV-down"] != filepath.Join("videos", "BV-down.mp4") {
		t.Fatalf("unexpected relative path for BV-down: %s", gotPaths["BV-down"])
	}
	if gotPaths["BV-stable"] != filepath.Join("videos", "BV-stable.mp4") {
		t.Fatalf("unexpected relative path for BV-stable: %s", gotPaths["BV-stable"])
	}
	if gotPaths["BV-rare"] != filepath.Join("rare", "BV-rare.mp4") {
		t.Fatalf("unexpected relative path for BV-rare: %s", gotPaths["BV-rare"])
	}
	if _, ok := gotPaths["BV-new"]; ok {
		t.Fatalf("expected NEW video absent from index")
	}
	if _, ok := gotPaths["BV-missing"]; ok {
		t.Fatalf("expected missing video absent from index")
	}
}

func TestProjectorRebuildAtomicallyReplacesMetadataFiles(t *testing.T) {
	root := t.TempDir()
	projector := NewProjector(root, WithClock(func() time.Time {
		return time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
	}))

	snapshot := CreatorSnapshot{
		Platform: "bilibili",
		UID:      "352981594",
		Name:     "元数据测试",
		Videos: []VideoSnapshot{
			{
				VideoID:  "BV-meta",
				Title:    "元数据视频",
				State:    StateDownloaded,
				FilePath: seedStoreFile(t, root, "bilibili", "BV-meta", "meta"),
			},
		},
	}

	creatorDir := CreatorDirectoryPath(root, snapshot)
	metaDir := filepath.Join(creatorDir, "_meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir meta dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "creator.json"), []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("seed creator json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "index.json"), []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("seed index json: %v", err)
	}

	if err := projector.RebuildCreator(context.Background(), snapshot); err != nil {
		t.Fatalf("rebuild creator: %v", err)
	}

	entries, err := os.ReadDir(metaDir)
	if err != nil {
		t.Fatalf("read meta dir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("unexpected temp file left behind: %s", entry.Name())
		}
	}

	var creatorMeta CreatorManifest
	readJSONFile(t, filepath.Join(metaDir, "creator.json"), &creatorMeta)
	if creatorMeta.ManifestVersion != ManifestVersion {
		t.Fatalf("expected manifest version %d, got %d", ManifestVersion, creatorMeta.ManifestVersion)
	}

	var index IndexManifest
	readJSONFile(t, filepath.Join(metaDir, "index.json"), &index)
	if len(index.Videos) != 1 || index.Videos[0].VideoID != "BV-meta" {
		t.Fatalf("unexpected index manifest: %+v", index.Videos)
	}
}

func TestProjectorRebuildRenamesCreatorDirectoryAndCleansLegacyDir(t *testing.T) {
	root := t.TempDir()
	projector := NewProjector(root)

	legacyDir := filepath.Join(root, "library", "bilibili", "creators", "352981594_旧名字")
	if err := os.MkdirAll(filepath.Join(legacyDir, "videos"), 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "videos", "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	snapshot := CreatorSnapshot{
		Platform: "bilibili",
		UID:      "352981594",
		Name:     "新名字",
		Videos: []VideoSnapshot{
			{
				VideoID:  "BV-rename",
				Title:    "改名后的视频",
				State:    StateDownloaded,
				FilePath: seedStoreFile(t, root, "bilibili", "BV-rename", "rename"),
			},
		},
	}

	if err := projector.RebuildCreator(context.Background(), snapshot); err != nil {
		t.Fatalf("rebuild creator: %v", err)
	}

	newDir := CreatorDirectoryPath(root, snapshot)
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("expected new creator dir exists: %v", err)
	}
	if _, err := os.Stat(legacyDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected legacy creator dir removed, got err=%v", err)
	}
}

func TestProjectorRebuildRemovesCreatorDirectoryWhenNoProjectedVideos(t *testing.T) {
	root := t.TempDir()
	projector := NewProjector(root)

	existingDir := filepath.Join(root, "library", "bilibili", "creators", "352981594_旧目录")
	if err := os.MkdirAll(filepath.Join(existingDir, "_meta"), 0o755); err != nil {
		t.Fatalf("mkdir existing dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "_meta", "creator.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("seed creator json: %v", err)
	}

	snapshot := CreatorSnapshot{
		Platform: "bilibili",
		UID:      "352981594",
		Name:     "没有本地视频",
		Videos: []VideoSnapshot{
			{
				VideoID:  "BV-empty",
				Title:    "未下载视频",
				State:    StateNew,
				FilePath: StoreVideoPath(root, "bilibili", "BV-empty"),
			},
		},
	}

	if err := projector.RebuildCreator(context.Background(), snapshot); err != nil {
		t.Fatalf("rebuild creator: %v", err)
	}

	if _, err := os.Stat(existingDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected creator dir removed when no projected videos, got err=%v", err)
	}

	currentDir := CreatorDirectoryPath(root, snapshot)
	if _, err := os.Stat(currentDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected current creator dir absent, got err=%v", err)
	}
}

func seedStoreFile(t *testing.T, root, platform, videoID, content string) string {
	t.Helper()
	path := StoreVideoPath(root, platform, videoID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir store dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write store file: %v", err)
	}
	return path
}

func assertSymlinkTarget(t *testing.T, path, want string) {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	if filepath.IsAbs(target) {
		t.Fatalf("expected relative symlink %s, got absolute target %s", path, target)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(path), target))
	if resolved != want {
		t.Fatalf("expected symlink %s -> %s, got %s (resolved %s)", path, want, target, resolved)
	}
}

func readJSONFile(t *testing.T, path string, dst any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}
