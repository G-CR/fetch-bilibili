package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"fetch-bilibili/internal/jobs"
	"fetch-bilibili/internal/library"
	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
)

type VideoClient interface {
	ListVideos(ctx context.Context, uid string) ([]bilibili.VideoMeta, error)
	CheckAvailable(ctx context.Context, videoID string) (bool, error)
	Download(ctx context.Context, videoID, dst string) (int64, error)
}

type stateEventPublisher interface {
	Publish(evt live.Event)
}

type storageSnapshot struct {
	initialized bool
	usedBytes   int64
	fileCount   int64
	rareVideos  int64
	dirtyRare   bool
}

type storageDelta struct {
	usedBytes  int64
	fileCount  int64
	rareVideos int64
}

var scanStorageUsageFn = scanStorageUsage

type DefaultHandler struct {
	creators         repo.CreatorRepository
	videos           repo.VideoRepository
	videoFiles       repo.VideoFileRepository
	jobs             repo.JobRepository
	client           VideoClient
	stableDays       int
	storageRoot      string
	logger           *log.Logger
	fetchLimit       int
	checkLimit       int
	globalLimit      *limiter
	perLimitQPS      int
	perLimitMu       sync.Mutex
	perLimiters      map[int64]*limiter
	storageMax       int64
	storageSafe      int64
	keepRare         bool
	cleanupRetention time.Duration
	cleanupLimit     int
	publisher        stateEventPublisher
	now              func() time.Time
	storageMu        sync.Mutex
	storageSnapshot  storageSnapshot
}

func NewDefaultHandler(creators repo.CreatorRepository, videos repo.VideoRepository, videoFiles repo.VideoFileRepository, jobs repo.JobRepository, client VideoClient, stableDays int, storageRoot string, globalQPS, perCreatorQPS int, logger *log.Logger) *DefaultHandler {
	if logger == nil {
		logger = log.Default()
	}
	if stableDays <= 0 {
		stableDays = 30
	}

	return &DefaultHandler{
		creators:     creators,
		videos:       videos,
		videoFiles:   videoFiles,
		jobs:         jobs,
		client:       client,
		stableDays:   stableDays,
		storageRoot:  storageRoot,
		logger:       logger,
		fetchLimit:   200,
		checkLimit:   200,
		globalLimit:  newLimiter(globalQPS),
		perLimitQPS:  perCreatorQPS,
		perLimiters:  make(map[int64]*limiter),
		keepRare:     true,
		cleanupLimit: 500,
		now:          time.Now,
	}
}

func (h *DefaultHandler) SetPublisher(publisher stateEventPublisher) {
	h.publisher = publisher
}

func (h *DefaultHandler) SetStoragePolicy(maxBytes, safeBytes int64, keepOutOfPrint bool) {
	h.storageMax = maxBytes
	if safeBytes <= 0 && maxBytes > 0 {
		safeBytes = maxBytes * 9 / 10
	}
	h.storageSafe = safeBytes
	h.keepRare = keepOutOfPrint
}

func (h *DefaultHandler) SetCleanupRetention(hours int) {
	if hours <= 0 {
		h.cleanupRetention = 0
		return
	}
	h.cleanupRetention = time.Duration(hours) * time.Hour
}

func (h *DefaultHandler) Handle(ctx context.Context, job repo.Job) error {
	switch job.Type {
	case jobs.TypeFetch:
		return h.handleFetch(ctx)
	case jobs.TypeCheck:
		return h.handleCheck(ctx, job)
	case jobs.TypeCleanup:
		return h.handleCleanup(ctx)
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
			if !created {
				current, err := h.videos.FindByID(ctx, id)
				if err != nil {
					h.logger.Printf("读取视频状态失败 video_id=%s: %v", meta.VideoID, err)
					lastErr = err
					continue
				}
				if current.State != "NEW" {
					continue
				}
			}
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
				if errors.Is(err, jobs.ErrJobAlreadyActive) {
					h.logger.Printf("跳过重复下载任务 video_id=%s", meta.VideoID)
					continue
				}
				h.logger.Printf("创建下载任务失败 video_id=%s: %v", meta.VideoID, err)
				lastErr = err
			}
		}
	}
	return lastErr
}

func (h *DefaultHandler) handleCleanup(ctx context.Context) error {
	if h.videos == nil || h.videoFiles == nil {
		return errors.New("清理处理器未初始化")
	}
	if h.storageRoot == "" {
		return errors.New("storage.root_dir 未配置")
	}

	usedBytes, fileCount, err := scanStorageUsageFn(h.storageRoot)
	if err != nil {
		return err
	}

	targetBytes := h.cleanupTarget(usedBytes)
	if targetBytes <= 0 {
		h.logger.Printf("清理任务跳过：当前占用 %d 字节，未超过安全阈值", usedBytes)
		return nil
	}
	h.seedStorageSnapshot(ctx, usedBytes, fileCount)

	candidates, err := h.videos.ListCleanupCandidates(ctx, repo.CleanupCandidateFilter{
		Limit:             h.cleanupLimit,
		IncludeOutOfPrint: !h.keepRare,
	})
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("清理候选不足：当前占用 %d 字节，目标释放 %d 字节", usedBytes, targetBytes)
	}

	candidates = h.filterCleanupCandidates(candidates)
	if len(candidates) == 0 {
		return fmt.Errorf("清理候选不足：当前占用 %d 字节，目标释放 %d 字节", usedBytes, targetBytes)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return lessCleanupCandidate(candidates[i], candidates[j])
	})

	var (
		freedBytes     int64
		deletedFiles   int64
		rareVideosDiff int64
		lastErr        error
	)
	for _, candidate := range candidates {
		if freedBytes >= targetBytes {
			break
		}
		freed, deleted, rareDelta, err := h.deleteCleanupCandidate(ctx, candidate)
		if err != nil {
			h.logger.Printf("清理候选失败 video_id=%s title=%s: %v", candidate.SourceVideoID, candidate.Title, err)
			lastErr = err
			continue
		}
		freedBytes += freed
		deletedFiles += deleted
		rareVideosDiff += rareDelta
	}

	if freedBytes < targetBytes {
		if lastErr != nil {
			return fmt.Errorf("清理未达到目标：已释放 %d 字节，仍需 %d 字节: %w", freedBytes, targetBytes-freedBytes, lastErr)
		}
		return fmt.Errorf("清理候选不足：已释放 %d 字节，仍需 %d 字节", freedBytes, targetBytes-freedBytes)
	}

	h.logger.Printf("清理任务完成：已释放 %d 字节，目标 %d 字节", freedBytes, targetBytes)
	h.publishStorageChanged(ctx, storageDelta{
		usedBytes:  -freedBytes,
		fileCount:  -deletedFiles,
		rareVideos: rareVideosDiff,
	})
	return nil
}

func (h *DefaultHandler) filterCleanupCandidates(candidates []repo.CleanupCandidate) []repo.CleanupCandidate {
	if h.cleanupRetention <= 0 {
		return candidates
	}
	now := time.Now()
	out := make([]repo.CleanupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.FileCreatedAt.IsZero() && candidate.FileCreatedAt.Add(h.cleanupRetention).After(now) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func (h *DefaultHandler) cleanupTarget(usedBytes int64) int64 {
	threshold := h.storageSafe
	if threshold <= 0 {
		threshold = h.storageMax
	}
	if threshold <= 0 {
		return 0
	}
	if usedBytes <= threshold {
		return 0
	}
	return usedBytes - threshold
}

func (h *DefaultHandler) deleteCleanupCandidate(ctx context.Context, candidate repo.CleanupCandidate) (int64, int64, int64, error) {
	freedBytes := candidate.FileSizeBytes
	info, err := os.Stat(candidate.FilePath)
	switch {
	case err == nil:
		freedBytes = info.Size()
		if err := os.Remove(candidate.FilePath); err != nil && !os.IsNotExist(err) {
			return 0, 0, 0, err
		}
	case os.IsNotExist(err):
		freedBytes = 0
	case err != nil:
		return 0, 0, 0, err
	}

	deleted, err := h.videoFiles.DeleteByID(ctx, candidate.FileID)
	if err != nil {
		return 0, 0, 0, err
	}
	if deleted == 0 {
		return 0, 0, 0, fmt.Errorf("文件记录不存在 file_id=%d", candidate.FileID)
	}
	deletedFiles := deleted

	remaining, err := h.videoFiles.CountByVideoID(ctx, candidate.VideoID)
	if err != nil {
		return 0, 0, 0, err
	}
	rareVideosDelta := int64(0)
	if remaining == 0 {
		if err := h.videos.UpdateState(ctx, candidate.VideoID, "DELETED"); err != nil {
			return 0, 0, 0, err
		}
		if candidate.State == "OUT_OF_PRINT" {
			rareVideosDelta = -1
		}
		if h.publisher != nil {
			video, err := h.videos.FindByID(ctx, candidate.VideoID)
			if err != nil {
				return 0, 0, 0, err
			}
			video.State = "DELETED"
			h.publishVideoChanged(video)
		}
	}

	h.logger.Printf(
		"已清理视频：title=%s video_id=%s state=%s follower=%d view=%d favorite=%d freed=%d path=%s",
		candidate.Title,
		candidate.SourceVideoID,
		candidate.State,
		candidate.CreatorFollowerCount,
		candidate.ViewCount,
		candidate.FavoriteCount,
		freedBytes,
		candidate.FilePath,
	)
	return freedBytes, deletedFiles, rareVideosDelta, nil
}

func (h *DefaultHandler) handleCheck(ctx context.Context, job repo.Job) error {
	if h.videos == nil || h.client == nil {
		return errors.New("检查处理器未初始化")
	}

	var (
		videos []repo.Video
		err    error
	)
	if videoID, ok := payloadInt64(job.Payload, "video_id"); ok && videoID > 0 {
		video, err := h.videos.FindByID(ctx, videoID)
		if err != nil {
			return err
		}
		videos = []repo.Video{video}
	} else {
		videos, err = h.videos.ListForCheck(ctx, h.checkLimit)
		if err != nil {
			return err
		}
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
			continue
		}
		h.syncStorageRareVideosOnCheck(video.State, newState)
		if shouldPublishVideoChanged(video.State, newState) {
			next := video
			next.State = newState
			next.LastCheckAt = now
			if outOfPrintAt != nil {
				next.OutOfPrintAt = *outOfPrintAt
			}
			if stableAt != nil {
				next.StableAt = *stableAt
			}
			h.publishVideoChanged(next)
		}
	}

	return lastErr
}

func (h *DefaultHandler) syncStorageRareVideosOnCheck(prevState, nextState string) {
	if prevState == nextState {
		return
	}
	prevRare := prevState == "OUT_OF_PRINT"
	nextRare := nextState == "OUT_OF_PRINT"
	if prevRare == nextRare {
		return
	}
	if h.storageRoot == "" {
		return
	}

	h.storageMu.Lock()
	defer h.storageMu.Unlock()
	h.storageSnapshot.dirtyRare = true
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
		if info, err := os.Stat(dst); err == nil {
			if info.Size() > 0 {
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if _, err := h.videoFiles.DeleteByVideoID(ctx, videoID); err != nil {
			return err
		}
	}
	if err := h.ensureStorageSnapshotInitialized(ctx); err != nil {
		return err
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
		_ = os.Remove(dst)
		_ = h.videos.UpdateState(ctx, videoID, "NEW")
		return fmt.Errorf("保存视频文件记录失败 video_id=%s: %w", video.VideoID, err)
	}

	if err := h.videos.UpdateState(ctx, videoID, "DOWNLOADED"); err != nil {
		return err
	}
	video.State = "DOWNLOADED"
	h.publishVideoChanged(video)
	h.publishStorageChanged(ctx, storageDelta{
		usedBytes: size,
		fileCount: 1,
	})
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
	return library.StoreVideoPath(root, platform, videoID)
}

func lessCleanupCandidate(a, b repo.CleanupCandidate) bool {
	aRare := a.State == "OUT_OF_PRINT"
	bRare := b.State == "OUT_OF_PRINT"
	if aRare != bRare {
		return !aRare && bRare
	}
	if a.CreatorFollowerCount != b.CreatorFollowerCount {
		return a.CreatorFollowerCount < b.CreatorFollowerCount
	}
	if a.ViewCount != b.ViewCount {
		return a.ViewCount < b.ViewCount
	}
	if a.FavoriteCount != b.FavoriteCount {
		return a.FavoriteCount < b.FavoriteCount
	}
	if a.FileSizeBytes != b.FileSizeBytes {
		return a.FileSizeBytes > b.FileSizeBytes
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	return a.FileID < b.FileID
}

func scanStorageUsage(root string) (int64, int64, error) {
	if root == "" {
		return 0, 0, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if !info.IsDir() {
		return 0, 0, nil
	}
	root = library.StoreRootPath(root)
	info, err = os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if !info.IsDir() {
		return 0, 0, nil
	}

	var (
		usedBytes int64
		fileCount int64
	)
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		usedBytes += info.Size()
		fileCount++
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return usedBytes, fileCount, nil
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

func (h *DefaultHandler) publishVideoChanged(video repo.Video) {
	if h.publisher == nil {
		return
	}
	at := h.now()
	if !video.UpdatedAt.IsZero() {
		at = video.UpdatedAt
	}
	platform := video.Platform
	if platform == "" {
		platform = "bilibili"
	}

	h.publisher.Publish(live.Event{
		ID:   fmt.Sprintf("video-%d-%d", video.ID, at.UnixNano()),
		Type: "video.changed",
		At:   at,
		Payload: map[string]any{
			"id":              video.ID,
			"platform":        platform,
			"video_id":        video.VideoID,
			"creator_id":      video.CreatorID,
			"title":           video.Title,
			"description":     video.Description,
			"publish_time":    formatChangedEventTime(video.PublishTime),
			"duration":        video.Duration,
			"cover_url":       video.CoverURL,
			"view_count":      video.ViewCount,
			"favorite_count":  video.FavoriteCount,
			"state":           video.State,
			"out_of_print_at": formatChangedEventTime(video.OutOfPrintAt),
			"stable_at":       formatChangedEventTime(video.StableAt),
			"last_check_at":   formatChangedEventTime(video.LastCheckAt),
		},
	})
}

func (h *DefaultHandler) publishStorageChanged(ctx context.Context, delta storageDelta) {
	if h.publisher == nil || h.storageRoot == "" {
		return
	}

	snapshot, err := h.applyStorageDelta(ctx, delta)
	if err != nil {
		h.logger.Printf("发布 storage.changed 失败: %v", err)
		return
	}

	at := h.now()
	h.publisher.Publish(live.Event{
		ID:   fmt.Sprintf("storage-%d", at.UnixNano()),
		Type: "storage.changed",
		At:   at,
		Payload: map[string]any{
			"root_dir":       h.storageRoot,
			"used_bytes":     snapshot.usedBytes,
			"max_bytes":      h.storageMax,
			"safe_bytes":     h.storageSafe,
			"usage_percent":  percent(snapshot.usedBytes, h.storageMax),
			"file_count":     snapshot.fileCount,
			"rare_videos":    snapshot.rareVideos,
			"cleanup_rule":   "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量",
			"hottest_bucket": "-",
		},
	})
}

func (h *DefaultHandler) applyStorageDelta(ctx context.Context, delta storageDelta) (storageSnapshot, error) {
	h.storageMu.Lock()
	defer h.storageMu.Unlock()

	refreshedRare := false
	if !h.storageSnapshot.initialized {
		usedBytes, fileCount, err := scanStorageUsageFn(h.storageRoot)
		if err != nil {
			return storageSnapshot{}, err
		}
		rareVideos, err := h.countRareVideos(ctx)
		if err != nil {
			return storageSnapshot{}, err
		}
		h.storageSnapshot = storageSnapshot{
			initialized: true,
			usedBytes:   usedBytes,
			fileCount:   fileCount,
			rareVideos:  rareVideos,
		}
		refreshedRare = true
	} else if h.storageSnapshot.dirtyRare {
		rareVideos, err := h.countRareVideos(ctx)
		if err != nil {
			return storageSnapshot{}, err
		}
		h.storageSnapshot.rareVideos = rareVideos
		h.storageSnapshot.dirtyRare = false
		refreshedRare = true
	}

	h.storageSnapshot.usedBytes += delta.usedBytes
	h.storageSnapshot.fileCount += delta.fileCount
	if !refreshedRare {
		h.storageSnapshot.rareVideos += delta.rareVideos
	}
	if h.storageSnapshot.usedBytes < 0 {
		h.storageSnapshot.usedBytes = 0
	}
	if h.storageSnapshot.fileCount < 0 {
		h.storageSnapshot.fileCount = 0
	}
	if h.storageSnapshot.rareVideos < 0 {
		h.storageSnapshot.rareVideos = 0
	}
	return h.storageSnapshot, nil
}

func (h *DefaultHandler) ensureStorageSnapshotInitialized(ctx context.Context) error {
	if h.publisher == nil || h.storageRoot == "" {
		return nil
	}

	h.storageMu.Lock()
	initialized := h.storageSnapshot.initialized
	h.storageMu.Unlock()
	if initialized {
		return nil
	}

	usedBytes, fileCount, err := scanStorageUsageFn(h.storageRoot)
	if err != nil {
		return err
	}
	h.seedStorageSnapshot(ctx, usedBytes, fileCount)
	return nil
}

func (h *DefaultHandler) seedStorageSnapshot(ctx context.Context, usedBytes, fileCount int64) {
	if h.publisher == nil || h.storageRoot == "" {
		return
	}

	h.storageMu.Lock()
	if h.storageSnapshot.initialized {
		h.storageMu.Unlock()
		return
	}
	h.storageMu.Unlock()

	rareVideos, err := h.countRareVideos(ctx)
	if err != nil {
		h.logger.Printf("统计绝版视频失败: %v", err)
		return
	}

	h.storageMu.Lock()
	if !h.storageSnapshot.initialized {
		dirtyRare := h.storageSnapshot.dirtyRare
		h.storageSnapshot = storageSnapshot{
			initialized: true,
			usedBytes:   usedBytes,
			fileCount:   fileCount,
			rareVideos:  rareVideos,
			dirtyRare:   dirtyRare,
		}
	}
	h.storageMu.Unlock()
}

func (h *DefaultHandler) countRareVideos(ctx context.Context) (int64, error) {
	if h.videos == nil {
		return 0, nil
	}
	return h.videos.CountByState(ctx, "OUT_OF_PRINT")
}

func shouldPublishVideoChanged(prev, next string) bool {
	if prev == next {
		return false
	}
	return next == "OUT_OF_PRINT" || next == "STABLE" || next == "DELETED"
}

func percent(used, max int64) int {
	if max <= 0 {
		return 0
	}
	p := int((used * 100) / max)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func formatChangedEventTime(v time.Time) string {
	if v.IsZero() {
		return ""
	}
	return v.Format(time.RFC3339)
}
