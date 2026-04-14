package creator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"fetch-bilibili/internal/repo"
)

type Resolver interface {
	ResolveUID(ctx context.Context, keyword string) (string, string, error)
}

var ErrInvalidPatch = errors.New("博主 patch 参数无效")
var ErrInvalidDelete = errors.New("博主 delete 参数无效")

type Patch struct {
	Name   *string
	Status *string
}

type Service struct {
	repo     repo.CreatorRepository
	resolver Resolver
	logger   *log.Logger
}

func NewService(repo repo.CreatorRepository, resolver Resolver, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{
		repo:     repo,
		resolver: resolver,
		logger:   logger,
	}
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
	return s.repo.ListActive(ctx, limit)
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
	return s.repo.UpdateStatus(ctx, id, "removed")
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
	return creator, false, nil
}
