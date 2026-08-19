package httpapi

import (
	"net/http"

	"task120-spc/internal/chart"
	"task120-spc/internal/domain"
)

type ruleConfigJSON struct {
	ChartID string          `json:"chart_id"`
	Rules   map[string]bool `json:"rules"`
}

func handleGetRules(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rules, err := svc.GetRules(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		out := make(map[string]bool, len(rules))
		for r, en := range rules {
			out[string(r)] = en
		}
		writeJSON(w, http.StatusOK, ruleConfigJSON{ChartID: id, Rules: out})
	}
}

type setRulesReq struct {
	Rules map[string]bool `json:"rules"`
}

func handleSetRules(svc *chart.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setRulesReq
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
			return
		}
		toggles := make(map[domain.RuleName]bool, len(req.Rules))
		for k, v := range req.Rules {
			toggles[domain.RuleName(k)] = v
		}
		if err := svc.SetRules(r.Context(), r.PathValue("id"), toggles); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}
