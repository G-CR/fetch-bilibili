package httpapi

import "net/http"

func newStorageStatsHandler(dashboard DashboardService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if dashboard == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "存储统计服务未就绪"})
			return
		}

		stats, err := dashboard.GetStorageStats(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, stats)
	})
}

func newStorageCleanupHandler(service JobService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "清理任务服务未就绪"})
			return
		}
		if err := service.EnqueueCleanup(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "queued",
			"type":   "cleanup",
		})
	})
}
