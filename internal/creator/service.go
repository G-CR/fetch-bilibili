package creator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"fetch-bilibili/internal/live"
	"fetch-bilibili/internal/repo"
)

type Resolver interface {
	ResolveUID(ctx context.Context, keyword string) (string, string, error)
	ResolveName(ctx context.Context, uid string) (string, error)
}

var ErrInvalidPatch = errors.New("博主 patch 参数无效")
var ErrInvalidDelete = errors.New("博主 delete 参数无效")

type Patch struct {
	Name   *string
	Status *string
}

type EventPublisher interface {
	Publish(evt live.Event)
}

type Service struct {
	repo      repo.CreatorRepository
	resolver  Resolver
	logger    *log.Logger
	publisher EventPublisher
	now       func() time.Time
}

func NewService(repo repo.CreatorRepository, resolver Resolver, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{
		repo:     repo,
		resolver: resolver,
		logger:   logger,
		now:      time.Now,
	}
}

func (s *Service) SetPublisher(publisher EventPublisher) {
	s.publisher = publisher
}

func (s *Service) Upsert(ctx context.Context, entry Entry) (repo.Creator, error) {
	creator, err := s.prepareCreator(ctx, entry)
	if err != nil {
		return repo.Creator{}, err
	}

	id, err := s.repo.Upsert(ctx, creator)
	if err != nil {
		return repo.Creator{}, err
	}
	creator.ID = id
	s.publishCreatorChanged(creator)
	return creator, nil
}

func (s *Service) prepareCreator(ctx context.Context, entry Entry) (repo.Creator, error) {
	if s.repo == nil {
		return repo.Creator{}, errors.New("博主服务未初始化")
	}

	uid := strings.TrimSpace(entry.UID)
	name := strings.TrimSpace(entry.Name)
	platform := strings.TrimSpace(entry.Platform)
	status := strings.TrimSpace(entry.Status)

	if platform == "" {
		platform = "bilibili"
	}
	if status == "" {
		status = "active"
	}
	if uid == "" && name == "" {
		return repo.Creator{}, errors.New("uid 或 name 必须提供")
	}

	if uid == "" {
		if s.resolver == nil {
			return repo.Creator{}, errors.New("缺少 UID 且未配置名称解析器")
		}
		resolvedUID, resolvedName, err := s.resolver.ResolveUID(ctx, name)
		if err != nil {
			return repo.Creator{}, err
		}
		uid = resolvedUID
		if resolvedName != "" {
			name = resolvedName
		}
		s.logger.Printf("名称解析成功 name=%s uid=%s", entry.Name, uid)
	}
	if name == "" {
		name = s.resolveNameByUID(ctx, uid)
	}

	return repo.Creator{
		Platform: platform,
		UID:      uid,
		Name:     name,
		Status:   status,
	}, nil
}

func (s *Service) ListActive(ctx context.Context, limit int) ([]repo.Creator, error) {
	if s.repo == nil {
		return nil, errors.New("博主服务未初始化")
	}
	creators, err := s.repo.ListActive(ctx, limit)
	if err != nil {
		return nil, err
	}
	s.backfillCreatorNames(ctx, creators)
	return creators, nil
}

func (s *Service) Patch(ctx context.Context, id int64, patch Patch) (repo.Creator, error) {
	if s.repo == nil {
		return repo.Creator{}, errors.New("博主服务未初始化")
	}
	if id <= 0 {
		return repo.Creator{}, fmt.Errorf("%w: id 必须大于 0", ErrInvalidPatch)
	}
	if patch.Name == nil && patch.Status == nil {
		return repo.Creator{}, fmt.Errorf("%w: 至少提供一个更新字段", ErrInvalidPatch)
	}

	current, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return repo.Creator{}, err
	}

	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return repo.Creator{}, fmt.Errorf("%w: name 不能为空", ErrInvalidPatch)
		}
		current.Name = name
	}
	if patch.Status != nil {
		status := strings.TrimSpace(*patch.Status)
		if status == "" {
			return repo.Creator{}, fmt.Errorf("%w: status 不能为空", ErrInvalidPatch)
		}
		current.Status = status
	}

	if err := s.repo.Update(ctx, current); err != nil {
		return repo.Creator{}, err
	}
	s.publishCreatorChanged(current)
	return current, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if s.repo == nil {
		return errors.New("博主服务未初始化")
	}
	if id <= 0 {
		return fmt.Errorf("%w: id 必须大于 0", ErrInvalidDelete)
	}

	current, err := s.repo.FindByID(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return repo.ErrNotFound
	case err != nil:
		return err
	}
	if current.Status == "removed" {
		return nil
	}
	if err := s.repo.UpdateStatus(ctx, id, "removed"); err != nil {
		return err
	}
	current.Status = "removed"
	s.publishCreatorChanged(current)
	return nil
}

func (s *Service) upsertFromFile(ctx context.Context, entry Entry) (repo.Creator, bool, error) {
	creator, err := s.prepareCreator(ctx, entry)
	if err != nil {
		return repo.Creator{}, false, err
	}

	current, err := s.repo.FindByPlatformUID(ctx, creator.Platform, creator.UID)
	switch {
	case err == nil && current.Status == "removed":
		return current, true, nil
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
	default:
		return repo.Creator{}, false, err
	}

	id, err := s.repo.Upsert(ctx, creator)
	if err != nil {
		return repo.Creator{}, false, err
	}
	creator.ID = id
	s.publishCreatorChanged(creator)
	return creator, false, nil
}

func (s *Service) resolveNameByUID(ctx context.Context, uid string) string {
	if s.resolver == nil {
		return ""
	}
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return ""
	}
	name, err := s.resolver.ResolveName(ctx, uid)
	if err != nil {
		s.logger.Printf("根据 UID 补齐博主名称失败 uid=%s: %v", uid, err)
		return ""
	}
	name = strings.TrimSpace(name)
	if name != "" {
		s.logger.Printf("根据 UID 补齐博主名称成功 uid=%s name=%s", uid, name)
	}
	return name
}

func (s *Service) backfillCreatorNames(ctx context.Context, creators []repo.Creator) {
	if s.repo == nil || s.resolver == nil {
		return
	}
	for idx := range creators {
		if strings.TrimSpace(creators[idx].Name) != "" {
			continue
		}
		name := s.resolveNameByUID(ctx, creators[idx].UID)
		if name == "" {
			continue
		}
		creators[idx].Name = name
		if creators[idx].Platform == "" {
			creators[idx].Platform = "bilibili"
		}
		if creators[idx].Status == "" {
			creators[idx].Status = "active"
		}
		if creators[idx].ID <= 0 {
			continue
		}
		if err := s.repo.Update(ctx, creators[idx]); err != nil {
			s.logger.Printf("回填博主名称入库失败 id=%d uid=%s: %v", creators[idx].ID, creators[idx].UID, err)
			continue
		}
		s.publishCreatorChanged(creators[idx])
	}
}

func (s *Service) publishCreatorChanged(creator repo.Creator) {
	if s.publisher == nil {
		return
	}
	creator = normalizeCreatorForEvent(creator)
	at := s.now()
	if !creator.UpdatedAt.IsZero() {
		at = creator.UpdatedAt
	}
	s.publisher.Publish(live.Event{
		ID:   fmt.Sprintf("creator-%d-%d", creator.ID, at.UnixNano()),
		Type: "creator.changed",
		At:   at,
		Payload: map[string]any{
			"id":       creator.ID,
			"uid":      creator.UID,
			"name":     creator.Name,
			"platform": creator.Platform,
			"status":   creator.Status,
		},
	})
}

func normalizeCreatorForEvent(creator repo.Creator) repo.Creator {
	if creator.Platform == "" {
		creator.Platform = "bilibili"
	}
	if creator.Status == "" {
		creator.Status = "active"
	}
	return creator
}
