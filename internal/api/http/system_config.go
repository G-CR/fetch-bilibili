package httpapi

import (
	"encoding/json"
	"net/http"
)

type systemConfigResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type updateSystemConfigRequest struct {
	Content string `json:"content"`
}

type updateSystemConfigResponse struct {
	Changed          bool   `json:"changed"`
	RestartScheduled bool   `json:"restart_scheduled"`
	Path             string `json:"path"`
}

func newSystemConfigHandler(service ConfigService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "配置服务未就绪"})
			return
		}

		switch r.Method {
		case http.MethodGet:
			doc, err := service.Load(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, systemConfigResponse{
				Path:    doc.Path,
				Content: doc.Content,
			})
		case http.MethodPut:
			var req updateSystemConfigRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "请求体解析失败"})
				return
			}
			result, err := service.Save(r.Context(), req.Content)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, updateSystemConfigResponse{
				Changed:          result.Changed,
				RestartScheduled: result.RestartScheduled,
				Path:             result.Path,
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
