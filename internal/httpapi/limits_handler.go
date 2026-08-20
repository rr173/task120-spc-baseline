package httpapi

import (
	"errors"
	"net/http"

	"task120-spc/internal/chart"
	"task120-spc/internal/domain"
)

type limitJSON struct {
	ChartID       string  `json:"chart_id"`
	BaselineCount int     `json:"baseline_count"`
	CL            float64 `json:"cl"`
	UCL           float64 `json:"ucl"`
	LCL           float64 `json:"lcl"`
	SigmaWithin   float64 `json:"sigma_within"`
	SigmaOverall  float64 `json:"sigma_overall"`
	ComputedSeq   int64   `json:"computed_seq"`
}

func handleGetLimits(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lim, err := svc.GetLimit(r.Context(), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, limitJSON{
			ChartID: lim.ChartID, BaselineCount: lim.BaselineCount,
			CL: lim.CL, UCL: lim.UCL, LCL: lim.LCL,
			SigmaWithin: lim.SigmaWithin, SigmaOverall: lim.SigmaOverall,
			ComputedSeq: lim.ComputedSeq,
		})
	}
}

// zoneJSON reports the +-1s/+-2s/+-3s boundaries for the controlled quantity.
// The frontend draws the control chart against these. For p_chart the
// per-point sigma varies, so the zones use the cache's within sigma as a
// representative width.
func handleGetZones(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lim, err := svc.GetLimit(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				// No baseline yet: report empty zones so the frontend can render.
				writeJSON(w, http.StatusOK, map[string]any{"chart_id": r.PathValue("id"), "zones": map[string]float64{}})
				return
			}
			writeServiceError(w, err)
			return
		}
		s := lim.SigmaWithin
		writeJSON(w, http.StatusOK, map[string]any{
			"chart_id":       lim.ChartID,
			"cl":             lim.CL,
			"ucl":            lim.UCL,
			"lcl":            lim.LCL,
			"plus1s":         lim.CL + s,
			"minus1s":        lim.CL - s,
			"plus2s":         lim.CL + 2*s,
			"minus2s":        lim.CL - 2*s,
			"plus3s":         lim.CL + 3*s,
			"minus3s":        lim.CL - 3*s,
			"baseline_count": lim.BaselineCount,
		})
	}
}

type violationJSON struct {
	ViolationID    string `json:"violation_id"`
	ChartID        string `json:"chart_id"`
	MeasurementSeq int    `json:"measurement_seq"`
	Rule           string `json:"rule"`
	Severity       string `json:"severity"`
	DetectedSeq    int64  `json:"detected_seq"`
}

func handleViolations(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vs, err := svc.ListViolations(r.Context(), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := make([]violationJSON, 0, len(vs))
		for _, v := range vs {
			out = append(out, violationJSON{
				ViolationID: v.ViolationID, ChartID: v.ChartID,
				MeasurementSeq: v.MeasurementSeq, Rule: string(v.Rule),
				Severity: string(v.Severity), DetectedSeq: v.DetectedSeq,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"violations": out})
	}
}

// capabilityJSON reports indices as float pointers so absent ones serialize
// away and present ones serialize as numbers. Estimable=false surfaces the
// not_estimable status to the client.
type capabilityJSON struct {
	Estimable   bool     `json:"estimable"`
	Status      string   `json:"status"`
	Cp          *float64 `json:"cp,omitempty"`
	Pp          *float64 `json:"pp,omitempty"`
	Cpm         *float64 `json:"cpm,omitempty"`
	CPU         *float64 `json:"cpu,omitempty"`
	CPL         *float64 `json:"cpl,omitempty"`
	Cpk         *float64 `json:"cpk,omitempty"`
	PPU         *float64 `json:"ppu,omitempty"`
	PPL         *float64 `json:"ppl,omitempty"`
	Ppk         *float64 `json:"ppk,omitempty"`
	DPMO        *float64 `json:"dpmo,omitempty"`
	Yield       *float64 `json:"yield,omitempty"`
	SigmaLevel  *float64 `json:"sigma_level,omitempty"`
}

func handleCapability(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := svc.GetCapability(r.Context(), r.PathValue("id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		status := "estimable"
		if !c.Estimable {
			status = "not_estimable"
		}
		writeJSON(w, http.StatusOK, capabilityJSON{
			Estimable: c.Estimable, Status: status,
			Cp: c.Cp, Pp: c.Pp, Cpm: c.Cpm,
			CPU: c.CPU, CPL: c.CPL, Cpk: c.Cpk,
			PPU: c.PPU, PPL: c.PPL, Ppk: c.Ppk,
			DPMO: c.DPMO, Yield: c.Yield, SigmaLevel: c.SigmaLevel,
		})
	}
}
