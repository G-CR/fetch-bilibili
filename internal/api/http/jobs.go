package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fetch-bilibili/internal/repo"
)

type jobHandler struct {
	service   JobService
	dashboard DashboardService
}

type jobRequest struct {
	Type string `json:"type"`
}

type jobResponse struct {
	Status string `json:"status"`
	Type   string `json:"type"`
}

func newJobHandler(service JobService, dashboard DashboardService) http.Handler {
	return &jobHandler{service: service, dashboard: dashboard}
}

func (h *jobHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleList(w, r)
	case http.MethodPost:
		h.handleCreate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *jobHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "任务服务未就绪"})
		return
	}

	var req jobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求体解析失败"})
		return
	}

	jobType := strings.TrimSpace(req.Type)
	if jobType == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "type 不能为空"})
		return
	}

	var err error
	switch jobType {
	case "fetch":
		err = h.service.EnqueueFetch(r.Context())
	case "check":
		err = h.service.EnqueueCheck(r.Context())
	case "cleanup":
		err = h.service.EnqueueCleanup(r.Context())
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "type 无效"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, jobResponse{Status: "queued", Type: jobType})
}

type listJobsResponse struct {
	Items []jobListItem `json:"items"`
}

type jobListItem struct {
	ID         int64                  `json:"id"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"`
	Payload    map[string]any         `json:"payload,omitempty"`
	ErrorMsg   string                 `json:"error_msg,omitempty"`
	NotBefore  string                 `json:"not_before,omitempty"`
	StartedAt  string                 `json:"started_at,omitempty"`
	FinishedAt string                 `json:"finished_at,omitempty"`
	CreatedAt  string                 `json:"created_at,omitempty"`
	UpdatedAt  string                 `json:"updated_at,omitempty"`
}

func (h *jobHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if h.dashboard == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "任务查询服务未就绪"})
		return
	}

	filter := repo.JobListFilter{
		Limit:  20,
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Type:   strings.TrimSpace(r.URL.Query().Get("type")),
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "limit 参数无效"})
			return
		}
		filter.Limit = val
	}

	jobsList, err := h.dashboard.ListJobs(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	items := make([]jobListItem, 0, len(jobsList))
	for _, job := range jobsList {
		items = append(items, jobListItem{
			ID:         job.ID,
			Type:       job.Type,
			Status:     job.Status,
			Payload:    job.Payload,
			ErrorMsg:   job.ErrorMsg,
			NotBefore:  formatTime(job.NotBefore),
			StartedAt:  formatTime(job.StartedAt),
			FinishedAt: formatTime(job.FinishedAt),
			CreatedAt:  formatTime(job.CreatedAt),
			UpdatedAt:  formatTime(job.UpdatedAt),
		})
	}
	writeJSON(w, http.StatusOK, listJobsResponse{Items: items})
}

func formatTime(v time.Time) string {
	if v.IsZero() {
		return ""
	}
	return v.Format(time.RFC3339)
}
