package httpapi

import (
	"encoding/json"
	"net/http"
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

func (h *creatorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
