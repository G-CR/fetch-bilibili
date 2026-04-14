package creator

import (
	"context"
	"log"
	"os"
	"time"
)

type FileSyncer struct {
	service       *Service
	filePath      string
	interval      time.Duration
	logger        *log.Logger
	lastModTime   time.Time
	lastSize      int64
	missingLogged bool
}

func NewFileSyncer(service *Service, filePath string, interval time.Duration, logger *log.Logger) *FileSyncer {
	if logger == nil {
		logger = log.Default()
	}
	return &FileSyncer{
		service:  service,
		filePath: filePath,
		interval: interval,
		logger:   logger,
	}
}

func (s *FileSyncer) Start(ctx context.Context) {
	if s.service == nil || s.filePath == "" {
		s.logger.Print("博主文件同步未启用：未配置文件或服务")
		return
	}

	s.syncOnce(ctx, true)
	if s.interval <= 0 {
		return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx, false)
		}
	}
}

func (s *FileSyncer) syncOnce(ctx context.Context, force bool) {
	info, err := os.Stat(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			if !s.missingLogged {
				s.logger.Printf("博主文件不存在: %s", s.filePath)
				s.missingLogged = true
			}
			return
		}
		s.logger.Printf("读取博主文件失败: %v", err)
		return
	}

	s.missingLogged = false
	if !force && !info.ModTime().After(s.lastModTime) && info.Size() == s.lastSize {
		return
	}

	entries, err := LoadEntries(s.filePath)
	if err != nil {
		s.logger.Printf("解析博主文件失败: %v", err)
		return
	}

	success := 0
	failed := 0
	skipped := 0
	activeIDs := make(map[int64]struct{}, len(entries))
	for _, entry := range entries {
		created, removed, err := s.service.upsertFromFile(ctx, entry)
		if err != nil {
			s.logger.Printf("同步博主失败 uid=%s name=%s: %v", entry.UID, entry.Name, err)
			failed++
			continue
		}
		if removed {
			s.logger.Printf("跳过已移除博主 uid=%s name=%s", created.UID, created.Name)
			skipped++
			continue
		}
		if created.ID > 0 {
			activeIDs[created.ID] = struct{}{}
		}
		success++
	}

	s.lastModTime = info.ModTime()
	s.lastSize = info.Size()
	s.logger.Printf("同步博主完成: 成功=%d 跳过=%d 失败=%d", success, skipped, failed)

	s.pauseMissing(ctx, activeIDs)
}

func (s *FileSyncer) pauseMissing(ctx context.Context, activeIDs map[int64]struct{}) {
	if s.service == nil || s.service.repo == nil {
		return
	}

	paused := 0
	lastID := int64(0)
	for {
		list, err := s.service.repo.ListActiveAfter(ctx, lastID, 200)
		if err != nil {
			s.logger.Printf("加载活跃博主失败: %v", err)
			return
		}
		if len(list) == 0 {
			break
		}
		for _, c := range list {
			if c.ID > lastID {
				lastID = c.ID
			}
			if _, ok := activeIDs[c.ID]; ok {
				continue
			}
			if err := s.service.repo.UpdateStatus(ctx, c.ID, "paused"); err != nil {
				s.logger.Printf("停用博主失败 id=%d uid=%s: %v", c.ID, c.UID, err)
				continue
			}
			paused++
		}
	}
	if paused > 0 {
		s.logger.Printf("自动停用博主: %d", paused)
	}
}
