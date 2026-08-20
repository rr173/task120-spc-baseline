package httpapi

import (
	"net/http"

	"task120-spc/internal/chart"
	"task120-spc/internal/domain"
)

// chartJSON is the response shape for a chart.
type chartJSON struct {
	ChartID        string             `json:"chart_id"`
	Name           string             `json:"name"`
	Characteristic string             `json:"characteristic"`
	ChartType      string             `json:"chart_type"`
	Unit           string             `json:"unit"`
	SubgroupSize   int                `json:"subgroup_size"`
	USL            *float64           `json:"usl,omitempty"`
	LSL            *float64           `json:"lsl,omitempty"`
	Target         *float64           `json:"target,omitempty"`
	CreatedSeq     int64              `json:"created_seq"`
	Archived       bool               `json:"archived"`
}

func toChartJSON(c domain.Chart) chartJSON {
	return chartJSON{
		ChartID: c.ChartID, Name: c.Name, Characteristic: c.Characteristic,
		ChartType: string(c.ChartType), Unit: c.Unit, SubgroupSize: c.SubgroupSize,
		USL: c.USL, LSL: c.LSL, Target: c.Target, CreatedSeq: c.CreatedSeq, Archived: c.Archived,
	}
}

type createChartReq struct {
	Name           string   `json:"name"`
	Characteristic string   `json:"characteristic"`
	ChartType      string   `json:"chart_type"`
	Unit           string   `json:"unit"`
	SubgroupSize   int      `json:"subgroup_size"`
	USL            *float64 `json:"usl,omitempty"`
	LSL            *float64 `json:"lsl,omitempty"`
	Target         *float64 `json:"target,omitempty"`
}

func handleCreateChart(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createChartReq
		if err := decodeChartRequest(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
			return
		}
		c, err := svc.CreateChart(r.Context(), req.Name, req.Characteristic,
			domain.ChartType(req.ChartType), req.Unit, req.SubgroupSize,
			ptrFloat(req.USL), ptrFloat(req.LSL), ptrFloat(req.Target))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toChartJSON(c))
	}
}

func handleListCharts(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		includeArchived := r.URL.Query().Get("archived") == "1"
		charts, err := svc.ListCharts(r.Context(), includeArchived)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := make([]chartJSON, 0, len(charts))
		for _, c := range charts {
			out = append(out, toChartJSON(c))
		}
		writeJSON(w, http.StatusOK, map[string]any{"charts": out})
	}
}

func handleGetChart(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := svc.GetChart(r.Context(), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toChartJSON(c))
	}
}

type updateChartReq struct {
	Name           *string   `json:"name,omitempty"`
	Characteristic *string   `json:"characteristic,omitempty"`
	USL            *float64  `json:"usl,omitempty"`
	LSL            *float64  `json:"lsl,omitempty"`
	Target         *float64  `json:"target,omitempty"`
	Archived       *bool     `json:"archived,omitempty"`
	// ClearSpec=true with a nil USL/LSL means "clear that spec". Without these
	// flags a nil pointer is ambiguous (absent vs clear). The handler turns
	// the presence of a key into a **float64.
	USLSet    *bool `json:"usl_set,omitempty"`
	LSLSet    *bool `json:"lsl_set,omitempty"`
	TargetSet *bool `json:"target_set,omitempty"`
}

func handleUpdateChart(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateChartReq
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
			return
		}
		id := r.PathValue("id")
		// Build **float64: nil when the key was absent, non-nil (pointing to a
		// possibly-nil *float64) when present so the service can clear.
		var uslP, lslP, targetP **float64
		if req.USLSet != nil && *req.USLSet {
			uslP = ptrToPtr(ptrFloat(req.USL))
		}
		if req.LSLSet != nil && *req.LSLSet {
			lslP = ptrToPtr(ptrFloat(req.LSL))
		}
		if req.TargetSet != nil && *req.TargetSet {
			targetP = ptrToPtr(ptrFloat(req.Target))
		}
		c, err := svc.UpdateChart(r.Context(), id, req.Name, req.Characteristic, uslP, lslP, targetP, req.Archived)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toChartJSON(c))
	}
}

// ptrToPtr returns a pointer to the given pointer, so the caller can express
// "set to nil" vs "leave unchanged".
func ptrToPtr(p *float64) **float64 {
	return &p
}

func handleDeleteChart(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.ArchiveChart(r.Context(), r.PathValue("id")); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
	}
}
