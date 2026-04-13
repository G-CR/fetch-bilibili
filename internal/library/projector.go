package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Projector struct {
	root string
	now  func() time.Time
}

type Option func(*Projector)

func WithClock(now func() time.Time) Option {
	return func(p *Projector) {
		if now != nil {
			p.now = now
		}
	}
}

func NewProjector(root string, opts ...Option) *Projector {
	projector := &Projector{
		root: root,
		now:  time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(projector)
		}
	}
	return projector
}

func (p *Projector) RebuildCreator(ctx context.Context, snapshot CreatorSnapshot) error {
	if p == nil {
		return errors.New("投影器未初始化")
	}
	if snapshot.UID == "" {
		return errors.New("博主 UID 不能为空")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	creatorDir := CreatorDirectoryPath(p.root, snapshot)
	if err := p.removeLegacyCreatorDirs(snapshot, creatorDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(creatorDir, "_meta"), 0o755); err != nil {
		return err
	}
	if err := rebuildBucketDir(filepath.Join(creatorDir, "videos")); err != nil {
		return err
	}
	if err := rebuildBucketDir(filepath.Join(creatorDir, "rare")); err != nil {
		return err
	}

	generatedAt := p.now().UTC()
	projected := collectProjectedVideos(snapshot)
	creatorManifest := CreatorManifest{
		ManifestVersion: ManifestVersion,
		GeneratedAt:     generatedAt,
		Platform:        normalizePlatform(snapshot.Platform),
		UID:             snapshot.UID,
		Name:            snapshot.Name,
		Status:          snapshot.Status,
		FollowerCount:   snapshot.FollowerCount,
		Directory:       filepath.Base(creatorDir),
	}
	indexManifest := IndexManifest{
		ManifestVersion: ManifestVersion,
		GeneratedAt:     generatedAt,
		Platform:        normalizePlatform(snapshot.Platform),
		UID:             snapshot.UID,
		Videos:          make([]IndexVideoItem, 0, len(projected)),
	}

	for _, item := range projected {
		if err := ctx.Err(); err != nil {
			return err
		}
		linkPath := filepath.Join(creatorDir, item.bucket, item.video.VideoID+".mp4")
		if err := os.Symlink(item.video.FilePath, linkPath); err != nil {
			return fmt.Errorf("创建符号链接失败 %s: %w", linkPath, err)
		}
		if item.bucket == "rare" {
			creatorManifest.LocalRareCount++
		} else {
			creatorManifest.LocalVideoCount++
		}
		creatorManifest.StorageBytes += item.size
		indexManifest.Videos = append(indexManifest.Videos, IndexVideoItem{
			VideoID:       item.video.VideoID,
			Title:         item.video.Title,
			State:         item.video.State,
			PublishTime:   item.video.PublishTime.UTC(),
			OutOfPrintAt:  item.video.OutOfPrintAt.UTC(),
			StableAt:      item.video.StableAt.UTC(),
			RelativePath:  filepath.Join(item.bucket, item.video.VideoID+".mp4"),
			SizeBytes:     item.size,
			ViewCount:     item.video.ViewCount,
			FavoriteCount: item.video.FavoriteCount,
		})
	}

	metaDir := filepath.Join(creatorDir, "_meta")
	if err := atomicWriteJSON(filepath.Join(metaDir, "creator.json"), creatorManifest); err != nil {
		return err
	}
	if err := atomicWriteJSON(filepath.Join(metaDir, "index.json"), indexManifest); err != nil {
		return err
	}
	return nil
}

type projectedVideo struct {
	video  VideoSnapshot
	bucket string
	size   int64
}

func collectProjectedVideos(snapshot CreatorSnapshot) []projectedVideo {
	items := make([]projectedVideo, 0, len(snapshot.Videos))
	for _, video := range snapshot.Videos {
		bucket := projectionBucket(video.State)
		if bucket == "" || video.FilePath == "" {
			continue
		}
		info, err := os.Stat(video.FilePath)
		if err != nil || info.IsDir() {
			continue
		}
		size := video.SizeBytes
		if size <= 0 {
			size = info.Size()
		}
		items = append(items, projectedVideo{
			video:  video,
			bucket: bucket,
			size:   size,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].video.PublishTime.Equal(items[j].video.PublishTime) {
			return items[i].video.PublishTime.After(items[j].video.PublishTime)
		}
		return items[i].video.VideoID < items[j].video.VideoID
	})
	return items
}

func projectionBucket(state string) string {
	switch state {
	case StateDownloaded, StateStable:
		return "videos"
	case StateOutOfPrint:
		return "rare"
	default:
		return ""
	}
}

func rebuildBucketDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0o755)
}

func (p *Projector) removeLegacyCreatorDirs(snapshot CreatorSnapshot, keep string) error {
	parent := filepath.Join(p.root, "library", normalizePlatform(snapshot.Platform), "creators")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	prefix := snapshot.UID + "_"
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		fullPath := filepath.Join(parent, entry.Name())
		if fullPath == keep {
			continue
		}
		if err := os.RemoveAll(fullPath); err != nil {
			return err
		}
	}
	return nil
}

func atomicWriteJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
