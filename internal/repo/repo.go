package repo

import (
	"context"
	"time"
)

type CreatorRepository interface {
	Upsert(ctx context.Context, c Creator) (int64, error)
	Create(ctx context.Context, c Creator) (int64, error)
	Update(ctx context.Context, c Creator) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	FindByID(ctx context.Context, id int64) (Creator, error)
	ListActive(ctx context.Context, limit int) ([]Creator, error)
	ListActiveAfter(ctx context.Context, lastID int64, limit int) ([]Creator, error)
}

type VideoRepository interface {
	Upsert(ctx context.Context, v Video) (int64, bool, error)
	UpdateState(ctx context.Context, id int64, state string) error
	FindByID(ctx context.Context, id int64) (Video, error)
	ListForCheck(ctx context.Context, limit int) ([]Video, error)
	UpdateCheckStatus(ctx context.Context, id int64, state string, outOfPrintAt *time.Time, stableAt *time.Time, lastCheckAt time.Time) error
}

type VideoFileRepository interface {
	Create(ctx context.Context, f VideoFile) (int64, error)
}

type JobRepository interface {
	Enqueue(ctx context.Context, job Job) (int64, error)
	FetchQueued(ctx context.Context, limit int) ([]Job, error)
	UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error
}

type Repositories struct {
	Creators   CreatorRepository
	Videos     VideoRepository
	VideoFiles VideoFileRepository
	Jobs       JobRepository
}
