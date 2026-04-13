package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"fetch-bilibili/internal/repo"
)

type videoHandler struct {
	dashboard DashboardService
}

type videoItemHandler struct {
	service   JobService
	dashboard DashboardService
}

type listVideosResponse struct {
	Items []videoListItem `json:"items"`
}

type videoListItem struct {
	ID            int64  `json:"id"`
	Platform      string `json:"platform"`
	VideoID       string `json:"video_id"`
	CreatorID     int64  `json:"creator_id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	PublishTime   string `json:"publish_time"`
	Duration      int    `json:"duration"`
	CoverURL      string `json:"cover_url"`
	ViewCount     int64  `json:"view_count"`
	FavoriteCount int64  `json:"favorite_count"`
	State         string `json:"state"`
	OutOfPrintAt  string `json:"out_of_print_at"`
	StableAt      string `json:"stable_at"`
	LastCheckAt   string `json:"last_check_at"`
}

func newVideoHandler(dashboard DashboardService) http.Handler {
	return &videoHandler{dashboard: dashboard}
}

func newVideoItemHandler(service JobService, dashboard DashboardService) http.Handler {
	return &videoItemHandler{service: service, dashboard: dashboard}
}

func (h *videoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.dashboard == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "视频查询服务未就绪"})
		return
	}

	filter := repo.VideoListFilter{
		Limit: 20,
		State: strings.TrimSpace(r.URL.Query().Get("state")),
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "limit 参数无效"})
			return
		}
		filter.Limit = val
	}
	if raw := r.URL.Query().Get("creator_id"); raw != "" {
		val, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || val <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "creator_id 参数无效"})
			return
		}
		filter.CreatorID = val
	}

	videos, err := h.dashboard.ListVideos(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	items := make([]videoListItem, 0, len(videos))
	for _, video := range videos {
		items = append(items, videoListItem{
			ID:            video.ID,
			Platform:      video.Platform,
			VideoID:       video.VideoID,
			CreatorID:     video.CreatorID,
			Title:         video.Title,
			Description:   video.Description,
			PublishTime:   formatTime(video.PublishTime),
			Duration:      video.Duration,
			CoverURL:      video.CoverURL,
			ViewCount:     video.ViewCount,
			FavoriteCount: video.FavoriteCount,
			State:         video.State,
			OutOfPrintAt:  formatTime(video.OutOfPrintAt),
			StableAt:      formatTime(video.StableAt),
			LastCheckAt:   formatTime(video.LastCheckAt),
		})
	}

	writeJSON(w, http.StatusOK, listVideosResponse{Items: items})
}

func (h *videoItemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, tail, err := parsePathID(r.URL.Path, "/videos/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "视频 ID 无效"})
		return
	}

	switch {
	case r.Method == http.MethodGet && tail == "":
		h.handleGet(w, r, id)
	case r.Method == http.MethodPost && tail == "download":
		h.handleDownload(w, r, id)
	case r.Method == http.MethodPost && tail == "check":
		h.handleCheck(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *videoItemHandler) handleGet(w http.ResponseWriter, r *http.Request, id int64) {
	if h.dashboard == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "视频查询服务未就绪"})
		return
	}
	video, err := h.dashboard.GetVideo(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "视频不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, videoListItem{
		ID:            video.ID,
		Platform:      video.Platform,
		VideoID:       video.VideoID,
		CreatorID:     video.CreatorID,
		Title:         video.Title,
		Description:   video.Description,
		PublishTime:   formatTime(video.PublishTime),
		Duration:      video.Duration,
		CoverURL:      video.CoverURL,
		ViewCount:     video.ViewCount,
		FavoriteCount: video.FavoriteCount,
		State:         video.State,
		OutOfPrintAt:  formatTime(video.OutOfPrintAt),
		StableAt:      formatTime(video.StableAt),
		LastCheckAt:   formatTime(video.LastCheckAt),
	})
}

func (h *videoItemHandler) handleDownload(w http.ResponseWriter, r *http.Request, id int64) {
	if h.service == nil || h.dashboard == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "视频运维服务未就绪"})
		return
	}
	if _, err := h.dashboard.GetVideo(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "视频不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if err := h.service.EnqueueDownload(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "queued",
		"type":     "download",
		"video_id": id,
	})
}

func (h *videoItemHandler) handleCheck(w http.ResponseWriter, r *http.Request, id int64) {
	if h.service == nil || h.dashboard == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "视频运维服务未就绪"})
		return
	}
	if _, err := h.dashboard.GetVideo(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "视频不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if err := h.service.EnqueueCheckVideo(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "queued",
		"type":     "check",
		"video_id": id,
	})
}
