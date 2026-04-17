package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"fetch-bilibili/internal/repo"
)

type candidateHandler struct {
	service CandidateService
}

type candidateItemHandler struct {
	service CandidateService
}

type candidateDiscoverHandler struct {
	service CandidateService
}

type listCandidatesResponse struct {
	Items    []candidateListItem `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

type candidateListItem struct {
	ID               int64                 `json:"id"`
	Platform         string                `json:"platform"`
	UID              string                `json:"uid"`
	Name             string                `json:"name"`
	AvatarURL        string                `json:"avatar_url"`
	ProfileURL       string                `json:"profile_url"`
	FollowerCount    int64                 `json:"follower_count"`
	Status           string                `json:"status"`
	Score            int                   `json:"score"`
	ScoreVersion     string                `json:"score_version"`
	LastDiscoveredAt string                `json:"last_discovered_at"`
	LastScoredAt     string                `json:"last_scored_at"`
	ApprovedAt       string                `json:"approved_at"`
	IgnoredAt        string                `json:"ignored_at"`
	BlockedAt        string                `json:"blocked_at"`
	CreatedAt        string                `json:"created_at"`
	UpdatedAt        string                `json:"updated_at"`
	Sources          []candidateSourceItem `json:"sources"`
}

type candidateDetailResponse struct {
	Candidate    candidateCoreResponse      `json:"candidate"`
	Sources      []candidateSourceItem      `json:"sources"`
	ScoreDetails []candidateScoreDetailItem `json:"score_details"`
}

type candidateCoreResponse struct {
	ID               int64  `json:"id"`
	Platform         string `json:"platform"`
	UID              string `json:"uid"`
	Name             string `json:"name"`
	AvatarURL        string `json:"avatar_url"`
	ProfileURL       string `json:"profile_url"`
	FollowerCount    int64  `json:"follower_count"`
	Status           string `json:"status"`
	Score            int    `json:"score"`
	ScoreVersion     string `json:"score_version"`
	LastDiscoveredAt string `json:"last_discovered_at"`
	LastScoredAt     string `json:"last_scored_at"`
	ApprovedAt       string `json:"approved_at"`
	IgnoredAt        string `json:"ignored_at"`
	BlockedAt        string `json:"blocked_at"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type candidateSourceItem struct {
	ID          int64           `json:"id"`
	SourceType  string          `json:"source_type"`
	SourceValue string          `json:"source_value"`
	SourceLabel string          `json:"source_label"`
	Weight      int             `json:"weight"`
	DetailJSON  json.RawMessage `json:"detail_json,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

type candidateScoreDetailItem struct {
	ID          int64           `json:"id"`
	FactorKey   string          `json:"factor_key"`
	FactorLabel string          `json:"factor_label"`
	ScoreDelta  int             `json:"score_delta"`
	DetailJSON  json.RawMessage `json:"detail_json,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

func newCandidateHandler(service CandidateService) http.Handler {
	return &candidateHandler{service: service}
}

func newCandidateItemHandler(service CandidateService) http.Handler {
	return &candidateItemHandler{service: service}
}

func newCandidateDiscoverHandler(service CandidateService) http.Handler {
	return &candidateDiscoverHandler{service: service}
}

func (h *candidateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "请求方法不支持"})
		return
	}
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "候选池服务未就绪"})
		return
	}

	filter := repo.CandidateListFilter{
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Keyword:  strings.TrimSpace(r.URL.Query().Get("keyword")),
		Page:     1,
		PageSize: 20,
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("min_score")); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val < 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "min_score 参数无效"})
			return
		}
		filter.MinScore = val
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "page 参数无效"})
			return
		}
		filter.Page = val
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil || val <= 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "page_size 参数无效"})
			return
		}
		filter.PageSize = val
	}

	items, total, err := h.service.ListCandidates(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	responseItems := make([]candidateListItem, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, candidateListItem{
			ID:               item.Candidate.ID,
			Platform:         item.Candidate.Platform,
			UID:              item.Candidate.UID,
			Name:             item.Candidate.Name,
			AvatarURL:        item.Candidate.AvatarURL,
			ProfileURL:       item.Candidate.ProfileURL,
			FollowerCount:    item.Candidate.FollowerCount,
			Status:           item.Candidate.Status,
			Score:            item.Candidate.Score,
			ScoreVersion:     item.Candidate.ScoreVersion,
			LastDiscoveredAt: formatTime(item.Candidate.LastDiscoveredAt),
			LastScoredAt:     formatTime(item.Candidate.LastScoredAt),
			ApprovedAt:       formatTime(item.Candidate.ApprovedAt),
			IgnoredAt:        formatTime(item.Candidate.IgnoredAt),
			BlockedAt:        formatTime(item.Candidate.BlockedAt),
			CreatedAt:        formatTime(item.Candidate.CreatedAt),
			UpdatedAt:        formatTime(item.Candidate.UpdatedAt),
			Sources:          mapCandidateSources(item.Sources),
		})
	}

	writeJSON(w, http.StatusOK, listCandidatesResponse{
		Items:    responseItems,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	})
}

func (h *candidateDiscoverHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "请求方法不支持"})
		return
	}
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "候选池服务未就绪"})
		return
	}
	if err := h.service.TriggerDiscover(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "queued",
		"type":   "discover",
	})
}

func (h *candidateItemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, tail, err := parsePathID(r.URL.Path, "/candidate-creators/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "候选 ID 无效"})
		return
	}

	switch {
	case r.Method == http.MethodGet && tail == "":
		h.handleGet(w, r, id)
	case r.Method == http.MethodPost && tail == "approve":
		h.handleApprove(w, r, id)
	case r.Method == http.MethodPost && tail == "ignore":
		h.handleAction(w, r, id, "ignore", h.service.Ignore)
	case r.Method == http.MethodPost && tail == "block":
		h.handleAction(w, r, id, "block", h.service.Block)
	case r.Method == http.MethodPost && tail == "review":
		h.handleAction(w, r, id, "review", h.service.Review)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "请求方法不支持"})
	}
}

func (h *candidateItemHandler) handleGet(w http.ResponseWriter, r *http.Request, id int64) {
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "候选池服务未就绪"})
		return
	}
	item, err := h.service.GetCandidate(r.Context(), id)
	if err != nil {
		h.writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, candidateDetailResponse{
		Candidate:    mapCandidateCore(item.Candidate),
		Sources:      mapCandidateSources(item.Sources),
		ScoreDetails: mapCandidateScoreDetails(item.ScoreDetails),
	})
}

func (h *candidateItemHandler) handleApprove(w http.ResponseWriter, r *http.Request, id int64) {
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "候选池服务未就绪"})
		return
	}
	creator, err := h.service.Approve(r.Context(), id)
	if err != nil {
		h.writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, createCreatorResponse{
		ID:       creator.ID,
		UID:      creator.UID,
		Name:     creator.Name,
		Platform: creator.Platform,
		Status:   creator.Status,
	})
}

func (h *candidateItemHandler) handleAction(w http.ResponseWriter, r *http.Request, id int64, action string, fn func(ctx context.Context, id int64) error) {
	if h.service == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "候选池服务未就绪"})
		return
	}
	if err := fn(r.Context(), id); err != nil {
		h.writeCandidateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"action":       action,
		"candidate_id": id,
	})
}

func (h *candidateItemHandler) writeCandidateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repo.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "候选不存在"})
	case isCandidateBadRequest(err):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	}
}

func isCandidateBadRequest(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "非法状态流转") || strings.Contains(message, "候选状态不允许批准")
}

func mapCandidateCore(candidate repo.CandidateCreator) candidateCoreResponse {
	return candidateCoreResponse{
		ID:               candidate.ID,
		Platform:         candidate.Platform,
		UID:              candidate.UID,
		Name:             candidate.Name,
		AvatarURL:        candidate.AvatarURL,
		ProfileURL:       candidate.ProfileURL,
		FollowerCount:    candidate.FollowerCount,
		Status:           candidate.Status,
		Score:            candidate.Score,
		ScoreVersion:     candidate.ScoreVersion,
		LastDiscoveredAt: formatTime(candidate.LastDiscoveredAt),
		LastScoredAt:     formatTime(candidate.LastScoredAt),
		ApprovedAt:       formatTime(candidate.ApprovedAt),
		IgnoredAt:        formatTime(candidate.IgnoredAt),
		BlockedAt:        formatTime(candidate.BlockedAt),
		CreatedAt:        formatTime(candidate.CreatedAt),
		UpdatedAt:        formatTime(candidate.UpdatedAt),
	}
}

func mapCandidateSources(items []repo.CandidateCreatorSource) []candidateSourceItem {
	result := make([]candidateSourceItem, 0, len(items))
	for _, item := range items {
		result = append(result, candidateSourceItem{
			ID:          item.ID,
			SourceType:  item.SourceType,
			SourceValue: item.SourceValue,
			SourceLabel: item.SourceLabel,
			Weight:      item.Weight,
			DetailJSON:  item.DetailJSON,
			CreatedAt:   formatTime(item.CreatedAt),
		})
	}
	return result
}

func mapCandidateScoreDetails(items []repo.CandidateCreatorScoreDetail) []candidateScoreDetailItem {
	result := make([]candidateScoreDetailItem, 0, len(items))
	for _, item := range items {
		result = append(result, candidateScoreDetailItem{
			ID:          item.ID,
			FactorKey:   item.FactorKey,
			FactorLabel: item.FactorLabel,
			ScoreDelta:  item.ScoreDelta,
			DetailJSON:  item.DetailJSON,
			CreatedAt:   formatTime(item.CreatedAt),
		})
	}
	return result
}
