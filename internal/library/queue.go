package library

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fetch-bilibili/internal/live"
)

const (
	defaultQueueSize         = 256
	defaultReconcileInterval = 6 * time.Hour
	defaultRebuildPageSize   = 200
)

type Syncer struct {
	exporter          *Exporter
	projector         *Projector
	broker            *live.Broker
	events            <-chan live.Event
	cancelEvents      context.CancelFunc
	logger            *log.Logger
	reconcileInterval time.Duration
	rebuildPageSize   int

	queue chan int64

	mu         sync.Mutex
	pending    map[int64]struct{}
	processing map[int64]struct{}
	dirty      map[int64]struct{}
}

type SyncerOption func(*Syncer)

func WithReconcileInterval(interval time.Duration) SyncerOption {
	return func(s *Syncer) {
		s.reconcileInterval = interval
	}
}

func NewSyncer(root string, exporter *Exporter, broker *live.Broker, opts ...SyncerOption) *Syncer {
	s := &Syncer{
		exporter:          exporter,
		projector:         NewProjector(root),
		broker:            broker,
		logger:            log.New(os.Stderr, "", log.LstdFlags),
		reconcileInterval: defaultReconcileInterval,
		rebuildPageSize:   defaultRebuildPageSize,
		queue:             make(chan int64, defaultQueueSize),
		pending:           make(map[int64]struct{}),
		processing:        make(map[int64]struct{}),
		dirty:             make(map[int64]struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if broker != nil {
		subCtx, cancel := context.WithCancel(context.Background())
		s.events = broker.Subscribe(subCtx, defaultQueueSize)
		s.cancelEvents = cancel
	}
	return s
}

func (s *Syncer) Start(ctx context.Context) {
	if s == nil {
		return
	}

	go s.runQueue(ctx)

	if s.cancelEvents != nil {
		go func() {
			<-ctx.Done()
			s.cancelEvents()
		}()
	}
	var ticker *time.Ticker
	if s.reconcileInterval > 0 {
		ticker = time.NewTicker(s.reconcileInterval)
		defer ticker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-s.events:
			if !ok {
				s.events = nil
				continue
			}
			s.handleEvent(ctx, evt)
		case <-tickerChan(ticker):
			if err := s.RebuildAll(ctx); err != nil && ctx.Err() == nil {
				s.logger.Printf("浏览目录全量对账失败: %v", err)
			}
		}
	}
}

func (s *Syncer) RebuildAll(ctx context.Context) error {
	if s == nil || s.exporter == nil || s.projector == nil {
		return nil
	}

	creators, err := s.exporter.ListCreatorsForRebuild(ctx, s.rebuildPageSize)
	if err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(creators))
	for _, creator := range creators {
		if err := ctx.Err(); err != nil {
			return err
		}
		snapshot, err := s.exporter.ExportCreator(ctx, creator.ID)
		if err != nil {
			return err
		}
		seen[CreatorDirectoryPath(s.projector.root, snapshot)] = struct{}{}
		if err := s.projector.RebuildCreator(ctx, snapshot); err != nil {
			return err
		}
	}
	return s.removeStaleCreatorDirectories(seen)
}

func (s *Syncer) runQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case creatorID := <-s.queue:
			s.processCreator(ctx, creatorID)
		}
	}
}

func (s *Syncer) processCreator(ctx context.Context, creatorID int64) {
	for {
		s.mu.Lock()
		delete(s.pending, creatorID)
		s.processing[creatorID] = struct{}{}
		s.mu.Unlock()

		err := s.rebuildCreator(ctx, creatorID)

		s.mu.Lock()
		delete(s.processing, creatorID)
		_, dirty := s.dirty[creatorID]
		if dirty {
			delete(s.dirty, creatorID)
			s.pending[creatorID] = struct{}{}
		}
		s.mu.Unlock()

		if err != nil && ctx.Err() == nil {
			s.logger.Printf("重建浏览目录失败 creator_id=%d: %v", creatorID, err)
		}
		if !dirty {
			return
		}

		select {
		case <-ctx.Done():
			return
		case s.queue <- creatorID:
			return
		}
	}
}

func (s *Syncer) rebuildCreator(ctx context.Context, creatorID int64) error {
	if s.exporter == nil || s.projector == nil {
		return nil
	}
	snapshot, err := s.exporter.ExportCreator(ctx, creatorID)
	if err != nil {
		return err
	}
	return s.projector.RebuildCreator(ctx, snapshot)
}

func (s *Syncer) handleEvent(ctx context.Context, evt live.Event) {
	switch evt.Type {
	case "creator.changed":
		s.enqueueCreator(payloadInt64(evt.Payload, "id"))
	case "video.changed":
		if creatorID := payloadInt64(evt.Payload, "creator_id"); creatorID > 0 {
			s.enqueueCreator(creatorID)
			return
		}
		videoID := payloadInt64(evt.Payload, "id")
		if videoID <= 0 {
			return
		}
		creatorID, err := s.exporter.CreatorIDForVideo(ctx, videoID)
		if err != nil {
			if ctx.Err() == nil {
				s.logger.Printf("解析视频归属博主失败 video_id=%d: %v", videoID, err)
			}
			return
		}
		s.enqueueCreator(creatorID)
	}
}

func (s *Syncer) enqueueCreator(creatorID int64) {
	if creatorID <= 0 {
		return
	}

	s.mu.Lock()
	if _, ok := s.processing[creatorID]; ok {
		s.dirty[creatorID] = struct{}{}
		s.mu.Unlock()
		return
	}
	if _, ok := s.pending[creatorID]; ok {
		s.mu.Unlock()
		return
	}
	s.pending[creatorID] = struct{}{}
	s.mu.Unlock()

	s.queue <- creatorID
}

func (s *Syncer) removeStaleCreatorDirectories(seen map[string]struct{}) error {
	root := filepath.Join(s.projector.root, "library")
	platforms, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, platform := range platforms {
		if !platform.IsDir() {
			continue
		}
		creatorsDir := filepath.Join(root, platform.Name(), "creators")
		creators, err := os.ReadDir(creatorsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		for _, creator := range creators {
			if !creator.IsDir() {
				continue
			}
			path := filepath.Join(creatorsDir, creator.Name())
			if _, ok := seen[path]; ok {
				continue
			}
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func payloadInt64(payload any, key string) int64 {
	data, ok := payload.(map[string]any)
	if !ok {
		return 0
	}
	switch value := data[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func tickerChan(ticker *time.Ticker) <-chan time.Time {
	if ticker == nil {
		return nil
	}
	return ticker.C
}
