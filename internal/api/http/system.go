package httpapi

import (
	"net/http"

	"fetch-bilibili/internal/dashboard"
)

type systemStatusResponse struct {
	Health      string                  `json:"health"`
	MySQLOK     bool                    `json:"mysql_ok"`
	AuthEnabled bool                    `json:"auth_enabled"`
	Cookie      systemCookieResponse    `json:"cookie"`
	Risk        systemRiskResponse      `json:"risk"`
	Overview    dashboard.Overview      `json:"overview"`
	ActiveJobs  int64                   `json:"active_jobs"`
	LastJobAt   string                  `json:"last_job_at,omitempty"`
	RiskLevel   string                  `json:"risk_level"`
	Limits      systemLimitsResponse    `json:"limits"`
	Scheduler   systemSchedulerResponse `json:"scheduler"`
	StorageRoot string                  `json:"storage_root"`
}

type systemCookieResponse struct {
	Configured       bool   `json:"configured"`
	IsLogin          bool   `json:"is_login"`
	Mid              int64  `json:"mid"`
	Uname            string `json:"uname"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	Source           string `json:"source,omitempty"`
	LastCheckAt      string `json:"last_check_at,omitempty"`
	LastCheckResult  string `json:"last_check_result,omitempty"`
	LastReloadAt     string `json:"last_reload_at,omitempty"`
	LastReloadResult string `json:"last_reload_result,omitempty"`
	LastError        string `json:"last_error,omitempty"`
}

type systemRiskResponse struct {
	Level          string `json:"level"`
	Active         bool   `json:"active"`
	BackoffUntil   string `json:"backoff_until,omitempty"`
	BackoffSeconds int64  `json:"backoff_seconds"`
	LastHitAt      string `json:"last_hit_at,omitempty"`
	LastReason     string `json:"last_reason,omitempty"`
}

type systemLimitsResponse struct {
	GlobalQPS           int `json:"global_qps"`
	PerCreatorQPS       int `json:"per_creator_qps"`
	DownloadConcurrency int `json:"download_concurrency"`
	CheckConcurrency    int `json:"check_concurrency"`
}

type systemSchedulerResponse struct {
	FetchInterval   string `json:"fetch_interval"`
	CheckInterval   string `json:"check_interval"`
	CleanupInterval string `json:"cleanup_interval"`
	CheckStableDays int    `json:"check_stable_days"`
}

func newSystemStatusHandler(dashboard DashboardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if dashboard == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "系统状态服务未就绪"})
			return
		}

		status, err := dashboard.GetSystemStatus(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, systemStatusResponse{
			Health:      status.Health,
			MySQLOK:     status.MySQLOK,
			AuthEnabled: status.AuthEnabled,
			Cookie: systemCookieResponse{
				Configured:       status.Cookie.Configured,
				IsLogin:          status.Cookie.IsLogin,
				Mid:              status.Cookie.Mid,
				Uname:            status.Cookie.Uname,
				Status:           status.Cookie.Status,
				Error:            status.Cookie.Error,
				Source:           status.Cookie.Source,
				LastCheckAt:      formatTime(status.Cookie.LastCheckAt),
				LastCheckResult:  status.Cookie.LastCheckResult,
				LastReloadAt:     formatTime(status.Cookie.LastReloadAt),
				LastReloadResult: status.Cookie.LastReloadResult,
				LastError:        status.Cookie.LastError,
			},
			Risk: systemRiskResponse{
				Level:          status.Risk.Level,
				Active:         status.Risk.Active,
				BackoffUntil:   formatTime(status.Risk.BackoffUntil),
				BackoffSeconds: status.Risk.BackoffSeconds,
				LastHitAt:      formatTime(status.Risk.LastHitAt),
				LastReason:     status.Risk.LastReason,
			},
			Overview:    status.Overview,
			ActiveJobs:  status.ActiveJobs,
			LastJobAt:   formatTime(status.LastJobAt),
			RiskLevel:   status.RiskLevel,
			StorageRoot: status.StorageRoot,
			Limits: systemLimitsResponse{
				GlobalQPS:           status.Limits.GlobalQPS,
				PerCreatorQPS:       status.Limits.PerCreatorQPS,
				DownloadConcurrency: status.Limits.DownloadConcurrency,
				CheckConcurrency:    status.Limits.CheckConcurrency,
			},
			Scheduler: systemSchedulerResponse{
				FetchInterval:   status.Scheduler.FetchInterval.String(),
				CheckInterval:   status.Scheduler.CheckInterval.String(),
				CleanupInterval: status.Scheduler.CleanupInterval.String(),
				CheckStableDays: status.Scheduler.CheckStableDays,
			},
		})
	})
}
