package httpapi

import (
	"net/http"

	"task120-spc/internal/chart"
)

func handleRecompute(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Recompute(r.Context(), r.PathValue("id")); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "recomputed"})
	}
}

func handleStats(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := svc.Stats(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, stats)
	}
}

func handleAdminRecompute(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rep, err := svc.ConsistencyCheck(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rep)
	}
}
