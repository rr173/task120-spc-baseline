// Package httpapi wires the chart service to HTTP/JSON endpoints. Handlers
// are plain functions over the service so the same mux is shared by the real
// server, the smoke test and the handler tests (httptest).
//
// Routes are registered as Go 1.22 ServeMux patterns ("METHOD /path"). Every
// mutating endpoint validates its JSON body via decode; business errors are
// mapped to status codes by writeServiceError.
package httpapi

import (
	"net/http"
	"strconv"

	"task120-spc/internal/chart"
)

// Version is the service identifier reported by /healthz and /version.
const Version = "task120-spc/v1"

// AdminTokenHeader is the header consulted by admin-only endpoints.
const AdminTokenHeader = "X-Admin-Token"

// Services bundles the service the mux wires up.
type Services struct {
	Chart *chart.Service
}

// NewMux builds the HTTP handler tree over the given service. adminToken is a
// shared secret required by admin endpoints (POST /admin/recompute); empty
// disables admin protection (used by the smoke test). webFS serves the embedded
// frontend (nil disables the static routes).
func NewMux(svc Services, adminToken string, webFS http.FileSystem) http.Handler {
	mux := http.NewServeMux()
	requireAdmin := func(w http.ResponseWriter, r *http.Request) bool {
		if adminToken != "" && r.Header.Get(AdminTokenHeader) != adminToken {
			writeError(w, http.StatusUnauthorized, "invalid admin token")
			return false
		}
		return true
	}

	// --- health / version ---
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /version", handleVersion)

	// --- charts ---
	mux.HandleFunc("POST /charts", handleCreateChart(svc.Chart))
	mux.HandleFunc("GET /charts", handleListCharts(svc.Chart))
	mux.HandleFunc("GET /charts/{id}", handleGetChart(svc.Chart))
	mux.HandleFunc("PATCH /charts/{id}", handleUpdateChart(svc.Chart))
	mux.HandleFunc("DELETE /charts/{id}", handleDeleteChart(svc.Chart))

	// --- measurements ---
	mux.HandleFunc("POST /charts/{id}/measurements", handleAddMeasurement(svc.Chart))
	mux.HandleFunc("GET /charts/{id}/measurements", handleListMeasurements(svc.Chart))
	mux.HandleFunc("GET /charts/{id}/measurements/{mid}", handleGetMeasurement(svc.Chart))
	mux.HandleFunc("DELETE /charts/{id}/measurements/{mid}", handleExcludeMeasurement(svc.Chart))
	mux.HandleFunc("POST /charts/{id}/measurements/{mid}/restore", handleRestoreMeasurement(svc.Chart))

	// --- limits / zones / violations / capability ---
	mux.HandleFunc("GET /charts/{id}/limits", handleGetLimits(svc.Chart))
	mux.HandleFunc("GET /charts/{id}/zones", handleGetZones(svc.Chart))
	mux.HandleFunc("GET /charts/{id}/violations", handleViolations(svc.Chart))
	mux.HandleFunc("GET /charts/{id}/capability", handleCapability(svc.Chart))

	// --- rules ---
	mux.HandleFunc("GET /charts/{id}/rules", handleGetRules(svc.Chart))
	mux.HandleFunc("PUT /charts/{id}/rules", handleSetRules(svc.Chart))

	// --- per-chart recompute + stats + admin ---
	mux.HandleFunc("POST /charts/{id}/recompute", handleRecompute(svc.Chart))
	mux.HandleFunc("GET /stats", handleStats(svc.Chart))
	mux.HandleFunc("POST /admin/recompute", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		handleAdminRecompute(svc.Chart)(w, r)
	})

	// --- frontend (embedded static) ---
	if webFS != nil {
		registerFrontend(mux, webFS)
	}

	return mux
}

// --- health / version ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": Version})
}

// atoiOr parses s as int, returning def on empty/parse error.
func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
