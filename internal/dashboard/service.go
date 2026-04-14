package dashboard

import (
	"context"
	"database/sql"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fetch-bilibili/internal/config"
	"fetch-bilibili/internal/library"
	"fetch-bilibili/internal/platform/bilibili"
	"fetch-bilibili/internal/repo"
)

type AuthChecker interface {
	CheckAuth(ctx context.Context) (bilibili.AuthInfo, error)
}

type runtimeStatusProvider interface {
	RuntimeStatus() bilibili.RuntimeStatus
}

type Service struct {
	db       *sql.DB
	creators repo.CreatorRepository
	videos   repo.VideoRepository
	jobs     repo.JobRepository
	auth     AuthChecker
	cfg      config.Config
}

type Overview struct {
	ActiveCreators int64 `json:"active_creators"`
	PendingJobs    int64 `json:"pending_jobs"`
	RareVideos     int64 `json:"rare_videos"`
}

type CookieStatus struct {
	Configured       bool      `json:"configured"`
	IsLogin          bool      `json:"is_login"`
	Mid              int64     `json:"mid"`
	Uname            string    `json:"uname"`
	Status           string    `json:"status"`
	Error            string    `json:"error,omitempty"`
	Source           string    `json:"source,omitempty"`
	LastCheckAt      time.Time `json:"last_check_at,omitempty"`
	LastCheckResult  string    `json:"last_check_result,omitempty"`
	LastReloadAt     time.Time `json:"last_reload_at,omitempty"`
	LastReloadResult string    `json:"last_reload_result,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
}

type RiskStatus struct {
	Level          string    `json:"level"`
	Active         bool      `json:"active"`
	BackoffUntil   time.Time `json:"backoff_until,omitempty"`
	BackoffSeconds int64     `json:"backoff_seconds"`
	LastHitAt      time.Time `json:"last_hit_at,omitempty"`
	LastReason     string    `json:"last_reason,omitempty"`
}

type SystemStatus struct {
	Health      string                 `json:"health"`
	MySQLOK     bool                   `json:"mysql_ok"`
	AuthEnabled bool                   `json:"auth_enabled"`
	Cookie      CookieStatus           `json:"cookie"`
	Risk        RiskStatus             `json:"risk"`
	Overview    Overview               `json:"overview"`
	ActiveJobs  int64                  `json:"active_jobs"`
	LastJobAt   time.Time              `json:"last_job_at"`
	RiskLevel   string                 `json:"risk_level"`
	Limits      config.LimitsConfig    `json:"limits"`
	Scheduler   config.SchedulerConfig `json:"scheduler"`
	StorageRoot string                 `json:"storage_root"`
}

type StorageStats struct {
	RootDir       string `json:"root_dir"`
	UsedBytes     int64  `json:"used_bytes"`
	MaxBytes      int64  `json:"max_bytes"`
	SafeBytes     int64  `json:"safe_bytes"`
	UsagePercent  int    `json:"usage_percent"`
	FileCount     int64  `json:"file_count"`
	HottestBucket string `json:"hottest_bucket"`
	RareVideos    int64  `json:"rare_videos"`
	CleanupRule   string `json:"cleanup_rule"`
}

func New(db *sql.DB, creators repo.CreatorRepository, videos repo.VideoRepository, jobs repo.JobRepository, auth AuthChecker, cfg config.Config) *Service {
	return &Service{
		db:       db,
		creators: creators,
		videos:   videos,
		jobs:     jobs,
		auth:     auth,
		cfg:      cfg,
	}
}

func (s *Service) ListJobs(ctx context.Context, filter repo.JobListFilter) ([]repo.Job, error) {
	if s.jobs == nil {
		return nil, nil
	}
	return s.jobs.ListRecent(ctx, filter)
}

func (s *Service) ListVideos(ctx context.Context, filter repo.VideoListFilter) ([]repo.Video, error) {
	if s.videos == nil {
		return nil, nil
	}
	return s.videos.ListRecent(ctx, filter)
}

func (s *Service) GetVideo(ctx context.Context, id int64) (repo.Video, error) {
	if s.videos == nil {
		return repo.Video{}, repo.ErrNotImplemented
	}
	return s.videos.FindByID(ctx, id)
}

func (s *Service) GetSystemStatus(ctx context.Context) (SystemStatus, error) {
	status := SystemStatus{
		Health:      "online",
		MySQLOK:     true,
		RiskLevel:   "低",
		Risk:        RiskStatus{Level: "低"},
		Limits:      s.cfg.Limits,
		Scheduler:   s.cfg.Scheduler,
		StorageRoot: s.cfg.Storage.RootDir,
	}
	status.AuthEnabled = isCookieConfigured(s.cfg) && s.auth != nil

	if s.creators != nil {
		count, err := s.creators.CountActive(ctx)
		if err != nil {
			status.Health = "degraded"
		} else {
			status.Overview.ActiveCreators = count
		}
	}

	if s.jobs != nil {
		pending, err := s.jobs.CountByStatuses(ctx, []string{"queued", "running"})
		if err != nil {
			status.Health = "degraded"
		} else {
			status.Overview.PendingJobs = pending
			status.ActiveJobs = pending
		}
		jobsList, err := s.jobs.ListRecent(ctx, repo.JobListFilter{Limit: 1})
		if err != nil {
			status.Health = "degraded"
		} else if len(jobsList) > 0 {
			status.LastJobAt = jobsList[0].CreatedAt
		}
	}

	if s.videos != nil {
		rareCount, err := s.videos.CountByState(ctx, "OUT_OF_PRINT")
		if err != nil {
			status.Health = "degraded"
		} else {
			status.Overview.RareVideos = rareCount
		}
	}

	if s.db != nil {
		if err := s.db.PingContext(ctx); err != nil {
			status.Health = "degraded"
			status.MySQLOK = false
		}
	}

	runtime := s.runtimeStatus()
	status.Cookie = s.checkCookie(ctx, runtime)
	status.Risk = s.buildRiskStatus(runtime)
	status.RiskLevel = status.Risk.Level
	if status.Cookie.Status == "error" {
		status.Health = "degraded"
		if !status.Risk.Active {
			status.Risk.Level = "中"
			status.RiskLevel = "中"
		}
	}
	if status.Cookie.Configured && !status.Cookie.IsLogin && status.Cookie.Status == "invalid" {
		if !status.Risk.Active {
			status.Risk.Level = "中"
			status.RiskLevel = "中"
		}
	}

	return status, nil
}

func (s *Service) GetStorageStats(ctx context.Context) (StorageStats, error) {
	usedBytes, fileCount, hottestBucket, err := scanStorage(s.cfg.Storage.RootDir)
	if err != nil {
		return StorageStats{}, err
	}

	var rareVideos int64
	if s.videos != nil {
		rareVideos, err = s.videos.CountByState(ctx, "OUT_OF_PRINT")
		if err != nil {
			return StorageStats{}, err
		}
	}

	return StorageStats{
		RootDir:       s.cfg.Storage.RootDir,
		UsedBytes:     usedBytes,
		MaxBytes:      s.cfg.Storage.MaxBytes,
		SafeBytes:     s.cfg.Storage.SafeBytes,
		UsagePercent:  percent(usedBytes, s.cfg.Storage.MaxBytes),
		FileCount:     fileCount,
		HottestBucket: hottestBucket,
		RareVideos:    rareVideos,
		CleanupRule:   "绝版优先 -> 粉丝量 -> 播放量 -> 收藏量",
	}, nil
}

func (s *Service) checkCookie(ctx context.Context, runtime bilibili.RuntimeStatus) CookieStatus {
	configured := isCookieConfigured(s.cfg)
	base := CookieStatus{
		Configured:       configured,
		Source:           runtime.CookieSource,
		LastCheckAt:      runtime.LastCheckAt,
		LastCheckResult:  runtime.LastCheckResult,
		LastReloadAt:     runtime.LastReloadAt,
		LastReloadResult: runtime.LastReloadResult,
		LastError:        runtime.LastError,
	}

	if !configured {
		base.Status = "not_configured"
		return base
	}
	if s.auth == nil {
		base.Status = "unknown"
		return base
	}

	info, err := s.auth.CheckAuth(ctx)
	if err != nil {
		base.Status = "error"
		base.Error = err.Error()
		return base
	}
	if !info.IsLogin {
		base.Status = "invalid"
		return base
	}
	base.IsLogin = true
	base.Mid = info.Mid
	base.Uname = info.Uname
	base.Status = "valid"
	return base
}

func (s *Service) runtimeStatus() bilibili.RuntimeStatus {
	if provider, ok := s.auth.(runtimeStatusProvider); ok && provider != nil {
		return provider.RuntimeStatus()
	}
	return bilibili.RuntimeStatus{}
}

func (s *Service) buildRiskStatus(runtime bilibili.RuntimeStatus) RiskStatus {
	now := time.Now()
	status := RiskStatus{
		Level:      "低",
		LastHitAt:  runtime.LastRiskAt,
		LastReason: runtime.LastRiskReason,
	}
	if !runtime.RiskUntil.IsZero() && runtime.RiskUntil.After(now) {
		status.Active = true
		status.Level = "高"
		status.BackoffUntil = runtime.RiskUntil
		status.BackoffSeconds = int64(math.Ceil(runtime.RiskUntil.Sub(now).Seconds()))
	}
	return status
}

func isCookieConfigured(cfg config.Config) bool {
	return cfg.Bilibili.Cookie != "" ||
		cfg.Bilibili.SESSDATA != ""
}

func scanStorage(root string) (int64, int64, string, error) {
	if strings.TrimSpace(root) == "" {
		return 0, 0, "-", nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, "-", nil
		}
		return 0, 0, "", err
	}
	if !info.IsDir() {
		return 0, 0, "-", nil
	}
	root = library.StoreRootPath(root)
	info, err = os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, "-", nil
		}
		return 0, 0, "", err
	}
	if !info.IsDir() {
		return 0, 0, "-", nil
	}

	var (
		usedBytes int64
		fileCount int64
	)
	buckets := make(map[string]int64)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		stat, err := d.Info()
		if err != nil {
			return err
		}
		size := stat.Size()
		usedBytes += size
		fileCount++

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 0 && parts[0] != "." && parts[0] != "" {
			buckets[parts[0]] += size
		}
		return nil
	})
	if err != nil {
		return 0, 0, "", err
	}

	hottestBucket := "-"
	var maxSize int64
	for bucket, size := range buckets {
		if size > maxSize {
			maxSize = size
			hottestBucket = bucket
		}
	}

	return usedBytes, fileCount, hottestBucket, nil
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
