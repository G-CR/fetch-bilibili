package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"fetch-bilibili/internal/creator"
)

type creatorHandler struct {
	service CreatorService
}

type createCreatorRequest struct {
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
}

type createCreatorResponse struct {
	ID       int64  `json:"id"`
	UID      string `json:"uid"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Status   string `json:"status"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func newCreatorHandler(service CreatorService) http.Handler {
	return &creatorHandler{service: service}
}

type patchCreatorRequest struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
}

func (h *creatorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleList(w, r)
	case http.MethodPost:
		h.handleCreate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *creatorHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "博主服务未就绪"})
		return
	}
	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "limit 参数无效"})
			return
		}
		limit = val
	}

	creators, err := h.service.ListActive(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	items := make([]createCreatorResponse, 0, len(creators))
	for _, c := range creators {
		items = append(items, createCreatorResponse{
			ID:       c.ID,
			UID:      c.UID,
			Name:     c.Name,
			Platform: c.Platform,
			Status:   c.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *creatorHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "博主服务未就绪"})
		return
	}

	var req createCreatorRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求体解析失败"})
		return
	}

	entry := creator.Entry{
		UID:      strings.TrimSpace(req.UID),
		Name:     strings.TrimSpace(req.Name),
		Platform: strings.TrimSpace(req.Platform),
		Status:   strings.TrimSpace(req.Status),
	}
	if entry.UID == "" && entry.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "uid 或 name 必须提供"})
		return
	}

	created, err := h.service.Upsert(r.Context(), entry)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	resp := createCreatorResponse{
		ID:       created.ID,
		UID:      created.UID,
		Name:     created.Name,
		Platform: created.Platform,
		Status:   created.Status,
	}
	writeJSON(w, http.StatusOK, resp)
}

func newCreatorItemHandler(service CreatorService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "博主服务未就绪"})
			return
		}

		id, tail, err := parsePathID(r.URL.Path, "/creators/")
		if err != nil || tail != "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "博主 ID 无效"})
			return
		}

		var req patchCreatorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求体解析失败"})
			return
		}

		updated, err := service.Patch(r.Context(), id, creator.Patch{
			Name:   req.Name,
			Status: req.Status,
		})
		if err != nil {
			switch {
			case errors.Is(err, creator.ErrInvalidPatch):
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			case errors.Is(err, sql.ErrNoRows):
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "博主不存在"})
			default:
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			}
			return
		}

		writeJSON(w, http.StatusOK, createCreatorResponse{
			ID:       updated.ID,
			UID:      updated.UID,
			Name:     updated.Name,
			Platform: updated.Platform,
			Status:   updated.Status,
		})
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
