package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
)

type VideoClient interface {
	ListVideos(ctx context.Context, uid string) ([]bilibili.VideoMeta, error)
	CheckAvailable(ctx context.Context, videoID string) (bool, error)
	Download(ctx context.Context, videoID, dst string) (int64, error)
}

type DefaultHandler struct {
	creators    repo.CreatorRepository
	videos      repo.VideoRepository
	videoFiles  repo.VideoFileRepository
	jobs        repo.JobRepository
	client      VideoClient
	stableDays  int
	storageRoot string
	logger      *log.Logger
	fetchLimit  int
	checkLimit  int
	globalLimit *limiter
	perLimitQPS int
	perLimitMu  sync.Mutex
	perLimiters map[int64]*limiter
}

func NewDefaultHandler(creators repo.CreatorRepository, videos repo.VideoRepository, videoFiles repo.VideoFileRepository, jobs repo.JobRepository, client VideoClient, stableDays int, storageRoot string, globalQPS, perCreatorQPS int, logger *log.Logger) *DefaultHandler {
	if logger == nil {
		logger = log.Default()
	}
	if stableDays <= 0 {
		stableDays = 30
	}

	return &DefaultHandler{
		creators:    creators,
		videos:      videos,
		videoFiles:  videoFiles,
		jobs:        jobs,
		client:      client,
		stableDays:  stableDays,
		storageRoot: storageRoot,
		logger:      logger,
		fetchLimit:  200,
		checkLimit:  200,
		globalLimit: newLimiter(globalQPS),
		perLimitQPS: perCreatorQPS,
		perLimiters: make(map[int64]*limiter),
	}
}

func (h *DefaultHandler) Handle(ctx context.Context, job repo.Job) error {
	switch job.Type {
	case jobs.TypeFetch:
		return h.handleFetch(ctx)
	case jobs.TypeCheck:
		return h.handleCheck(ctx)
	case jobs.TypeCleanup:
		return nil
	case jobs.TypeDownload:
		return h.handleDownload(ctx, job)
	default:
		return fmt.Errorf("未知任务类型: %s", job.Type)
	}
}

func (h *DefaultHandler) handleFetch(ctx context.Context) error {
	if h.creators == nil || h.videos == nil || h.client == nil {
		return errors.New("拉取处理器未初始化")
	}

	creators, err := h.creators.ListActive(ctx, h.fetchLimit)
	if err != nil {
		return err
	}
	if len(creators) == 0 {
		return nil
	}

	var lastErr error
	for _, creator := range creators {
		if err := h.waitForCreator(ctx, creator.ID); err != nil {
			return err
		}
		metas, err := h.client.ListVideos(ctx, creator.UID)
		if err != nil {
			h.logger.Printf("拉取视频列表失败 uid=%s: %v", creator.UID, err)
			lastErr = err
			continue
		}
		for _, meta := range metas {
			video := repo.Video{
				Platform:      "bilibili",
				VideoID:       meta.VideoID,
				CreatorID:     creator.ID,
				Title:         meta.Title,
				Description:   meta.Description,
				PublishTime:   meta.PublishTime,
				Duration:      meta.Duration,
				CoverURL:      meta.CoverURL,
				ViewCount:     meta.ViewCount,
				FavoriteCount: meta.FavoriteCount,
				State:         "NEW",
			}
			id, created, err := h.videos.Upsert(ctx, video)
			if err != nil {
				h.logger.Printf("保存视频失败 video_id=%s: %v", meta.VideoID, err)
				lastErr = err
				continue
			}
			if created {
				if h.jobs == nil {
					h.logger.Printf("下载任务未创建：未配置任务仓库 video_id=%s", meta.VideoID)
					continue
				}
				if _, err := h.jobs.Enqueue(ctx, repo.Job{
					Type:   jobs.TypeDownload,
					Status: jobs.StatusQueued,
					Payload: map[string]any{
						"video_id": id,
					},
				}); err != nil {
					h.logger.Printf("创建下载任务失败 video_id=%s: %v", meta.VideoID, err)
					lastErr = err
				}
			}
		}
	}
	return lastErr
}

func (h *DefaultHandler) handleCheck(ctx context.Context) error {
	if h.videos == nil || h.client == nil {
		return errors.New("检查处理器未初始化")
	}

	videos, err := h.videos.ListForCheck(ctx, h.checkLimit)
	if err != nil {
		return err
	}
	if len(videos) == 0 {
		return nil
	}

	var lastErr error
	now := time.Now()
	stableCutoff := now.Add(-time.Duration(h.stableDays) * 24 * time.Hour)

	for _, video := range videos {
		if err := h.waitForCreator(ctx, video.CreatorID); err != nil {
			return err
		}
		available, err := h.client.CheckAvailable(ctx, video.VideoID)
		if err != nil {
			h.logger.Printf("检查视频可用性失败 video_id=%s: %v", video.VideoID, err)
			lastErr = err
			continue
		}

		newState := video.State
		var outOfPrintAt *time.Time
		var stableAt *time.Time

		if !available {
			newState = "OUT_OF_PRINT"
			outOfPrintAt = &now
		} else if !video.PublishTime.IsZero() && video.PublishTime.Before(stableCutoff) {
			newState = "STABLE"
			stableAt = &now
		}

		if err := h.videos.UpdateCheckStatus(ctx, video.ID, newState, outOfPrintAt, stableAt, now); err != nil {
			h.logger.Printf("更新视频状态失败 video_id=%s: %v", video.VideoID, err)
			lastErr = err
		}
	}

	return lastErr
}

func (h *DefaultHandler) handleDownload(ctx context.Context, job repo.Job) error {
	if h.videos == nil || h.client == nil || h.videoFiles == nil {
		return errors.New("下载处理器未初始化")
	}
	if h.storageRoot == "" {
		return errors.New("storage.root_dir 未配置")
	}
	videoID, ok := payloadInt64(job.Payload, "video_id")
	if !ok || videoID <= 0 {
		return errors.New("下载任务缺少 video_id")
	}

	video, err := h.videos.FindByID(ctx, videoID)
	if err != nil {
		return err
	}

	dst := buildVideoPath(h.storageRoot, video.Platform, video.VideoID)
	if video.State == "DOWNLOADED" {
		if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	if err := h.videos.UpdateState(ctx, videoID, "DOWNLOADING"); err != nil {
		return err
	}

	if err := h.waitForCreator(ctx, video.CreatorID); err != nil {
		_ = h.videos.UpdateState(ctx, videoID, "NEW")
		return err
	}

	size, err := h.client.Download(ctx, video.VideoID, dst)
	if err != nil {
		_ = h.videos.UpdateState(ctx, videoID, "NEW")
		return err
	}

	if _, err := h.videoFiles.Create(ctx, repo.VideoFile{
		VideoID:   videoID,
		Path:      dst,
		SizeBytes: size,
		Type:      "video",
	}); err != nil {
		h.logger.Printf("保存视频文件记录失败 video_id=%s: %v", video.VideoID, err)
	}

	if err := h.videos.UpdateState(ctx, videoID, "DOWNLOADED"); err != nil {
		return err
	}
	return nil
}

func (h *DefaultHandler) waitForCreator(ctx context.Context, creatorID int64) error {
	if h.globalLimit != nil {
		if err := h.globalLimit.Wait(ctx); err != nil {
			return err
		}
	}
	if h.perLimitQPS <= 0 || creatorID == 0 {
		return nil
	}
	lim := h.getCreatorLimiter(creatorID)
	return lim.Wait(ctx)
}

func (h *DefaultHandler) getCreatorLimiter(creatorID int64) *limiter {
	h.perLimitMu.Lock()
	defer h.perLimitMu.Unlock()
	if lim, ok := h.perLimiters[creatorID]; ok {
		return lim
	}
	lim := newLimiter(h.perLimitQPS)
	h.perLimiters[creatorID] = lim
	return lim
}

func buildVideoPath(root, platform, videoID string) string {
	ext := ".mp4"
	if platform == "" {
		platform = "bilibili"
	}
	return filepath.Join(root, platform, videoID+ext)
}

func payloadInt64(payload map[string]any, key string) (int64, bool) {
	if payload == nil {
		return 0, false
	}
	val, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
