package httpapi

import (
	"net/http"

	"task120-spc/internal/chart"
	"task120-spc/internal/domain"
)

type measurementJSON struct {
	MeasurementID string     `json:"measurement_id"`
	ChartID       string     `json:"chart_id"`
	SubgroupSeq   int        `json:"subgroup_seq"`
	Values        []float64  `json:"values,omitempty"`
	ValueAvg      float64    `json:"value_avg"`
	RangeValue     float64    `json:"range_value"`
	Defectives    int        `json:"defectives,omitempty"`
	NObserved     int        `json:"n_observed,omitempty"`
	Excluded      bool       `json:"excluded"`
	CreatedSeq    int64      `json:"created_seq"`
}

func toMeasurementJSON(m domain.Measurement) measurementJSON {
	return measurementJSON{
		MeasurementID: m.MeasurementID, ChartID: m.ChartID, SubgroupSeq: m.SubgroupSeq,
		Values: m.Values, ValueAvg: m.ValueAvg, RangeValue: m.RangeValue,
		Defectives: m.Defectives, NObserved: m.NObserved, Excluded: m.Excluded, CreatedSeq: m.CreatedSeq,
	}
}

type addMeasurementReq struct {
	Values     []float64 `json:"values,omitempty"`
	Defectives *int      `json:"defectives,omitempty"`
	NObserved  *int      `json:"n_observed,omitempty"`
}

func handleAddMeasurement(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addMeasurementReq
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
			return
		}
		var def, n int
		if req.Defectives != nil {
			def = *req.Defectives
		}
		if req.NObserved != nil {
			n = *req.NObserved
		}
		m, err := svc.AddMeasurement(r.Context(), r.PathValue("id"), req.Values, def, n)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toMeasurementJSON(m))
	}
}

func handleListMeasurements(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		excludeExcluded := r.URL.Query().Get("include_excluded") != "1" // default: hide excluded
		ms, err := svc.ListMeasurements(r.Context(), r.PathValue("id"), excludeExcluded)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := make([]measurementJSON, 0, len(ms))
		for _, m := range ms {
			out = append(out, toMeasurementJSON(m))
		}
		writeJSON(w, http.StatusOK, map[string]any{"measurements": out})
	}
}

func handleGetMeasurement(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m, err := svc.GetMeasurement(r.Context(), r.PathValue("mid"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toMeasurementJSON(m))
	}
}

func handleExcludeMeasurement(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.ExcludeMeasurement(r.Context(), r.PathValue("mid")); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "excluded"})
	}
}

func handleRestoreMeasurement(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.RestoreMeasurement(r.Context(), r.PathValue("mid")); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
	}
}
