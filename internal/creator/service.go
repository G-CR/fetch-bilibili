package creator

import (
	"context"
	"errors"
	"log"
	"strings"

	"fetch-bilibili/internal/repo"
)

type Resolver interface {
	ResolveUID(ctx context.Context, keyword string) (string, string, error)
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

	creator := repo.Creator{
		Platform: platform,
		UID:      uid,
		Name:     name,
		Status:   status,
	}
	id, err := s.repo.Upsert(ctx, creator)
	if err != nil {
		return repo.Creator{}, err
	}
	creator.ID = id
	return creator, nil
}
