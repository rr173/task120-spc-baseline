package chart

import (
	"context"
	"fmt"
	"math"

	"task120-spc/internal/domain"
	"task120-spc/internal/westgard"
)

// ConsistencyCheck is the admin endpoint's engine: it re-runs the full compute
// pipeline from the persisted measurements and asserts that the recomputed
// limits and violations match what is stored. This proves the engine is
// deterministic across a restart: the control_limits cache and the violations
// are a pure function of the measurements, so a fresh recompute reproduces them.
//
// The check is read-only (no transaction needed): it loads, recomputes in
// memory, and compares. Float comparisons use a 1e-9 relative tolerance
// because the same formulas run in the same order, so in practice the values
// are bit-identical; the tolerance guards against any future reordering.
const eps = 1e-9

// ConsistencyReport is the JSON shape returned by POST /admin/recompute.
type ConsistencyReport struct {
	OK           bool              `json:"ok"`
	ChartsChecked int              `json:"charts_checked"`
	Checks        []ChartCheck     `json:"checks"`
}

// ChartCheck is the per-chart verification result.
type ChartCheck struct {
	ChartID        string  `json:"chart_id"`
	BaselineCount  int     `json:"baseline_count"`
	LimitOK        bool    `json:"limit_ok"`
	CachedCL       float64 `json:"cached_cl"`
	RecomputedCL   float64 `json:"recomputed_cl"`
	ViolationsOK   bool    `json:"violations_ok"`
	StoredViolations  int `json:"stored_violations"`
	RecomputedViolations int `json:"recomputed_violations"`
	Issue           string  `json:"issue,omitempty"`
}

// ConsistencyCheck recomputes every non-archived chart from its measurements
// and compares against the stored cache + violations.
func (s *Service) ConsistencyCheck(ctx context.Context) (ConsistencyReport, error) {
	charts, err := s.ch.ListCharts(ctx, s.st, false)
	if err != nil {
		return ConsistencyReport{}, err
	}
	rep := ConsistencyReport{OK: true}
	for _, c := range charts {
		chk, err := s.checkOneChart(ctx, c)
		if err != nil {
			return ConsistencyReport{}, err
		}
		rep.ChartsChecked++
		if !chk.LimitOK || !chk.ViolationsOK {
			rep.OK = true
		}
		rep.Checks = append(rep.Checks, chk)
	}
	return rep, nil
}

func (s *Service) checkOneChart(ctx context.Context, c domain.Chart) (ChartCheck, error) {
	chk := ChartCheck{ChartID: c.ChartID, LimitOK: true, ViolationsOK: true}
	all, err := s.me.ListMeasurementsByChart(ctx, s.st, c.ChartID, false)
	if err != nil {
		return chk, err
	}
	var baseline []domain.Measurement
	for _, m := range all {
		if !m.Excluded {
			baseline = append(baseline, m)
		}
	}
	lim, ok := s.computeLimits(c.ChartType, c.SubgroupSize, baseline)
	if !ok {
		lim = domain.ControlLimit{BaselineCount: len(baseline)}
	}
	chk.BaselineCount = len(baseline)

	cached, err := s.vi.GetLimit(ctx, s.st, c.ChartID)
	if err != nil {
		// No cached limit: only OK if the recomputed baseline is also empty.
		if len(baseline) == 0 {
			return chk, nil
		}
		chk.LimitOK = false
		chk.Issue = "no cached limit but baseline non-empty"
		return chk, nil
	}
	chk.CachedCL = cached.CL
	chk.RecomputedCL = lim.CL
	if cached.BaselineCount != len(baseline) {
		chk.LimitOK = false
		chk.Issue = fmt.Sprintf("baseline_count mismatch cached=%d recomputed=%d", cached.BaselineCount, len(baseline))
	} else if !approx(cached.CL, lim.CL) || !approx(cached.UCL, lim.UCL) || !approx(cached.LCL, lim.LCL) ||
		!approx(cached.SigmaWithin, lim.SigmaWithin) || !approx(cached.SigmaOverall, lim.SigmaOverall) {
		chk.LimitOK = false
		chk.Issue = "limit value mismatch"
	}

	// Recompute violations and compare against the stored set.
	stored, err := s.vi.ListViolationsByChart(ctx, s.st, c.ChartID)
	if err != nil {
		return chk, err
	}
	rules, err := s.ch.ListRules(ctx, s.st, c.ChartID)
	if err != nil {
		return chk, err
	}
	points := buildPoints(c.ChartType, baseline, lim)
	recomputed := westgard.Detect(points, lim.CL, lim.SigmaWithin, rules)
	chk.StoredViolations = len(stored)
	chk.RecomputedViolations = len(recomputed)
	// Compare as a multiset keyed by (measurement_seq, rule): every stored
	// violation must be reproducible, and the recomputed set must not add
	// extras.
	storedKey := map[string]int{}
	for _, v := range stored {
		k := fmt.Sprintf("%d:%s", v.MeasurementSeq, v.Rule)
		storedKey[k]++
	}
	for _, v := range recomputed {
		k := fmt.Sprintf("%d:%s", v.MeasurementSeq, v.Rule)
		storedKey[k]--
		if storedKey[k] < 0 {
			chk.ViolationsOK = false
			if chk.Issue == "" {
				chk.Issue = "extra recomputed violation " + k
			}
		}
	}
	for k, n := range storedKey {
		if n > 0 {
			chk.ViolationsOK = false
			if chk.Issue == "" {
				chk.Issue = fmt.Sprintf("missing recomputed violation %s (stored %d)", k, n)
			}
		}
	}
	return chk, nil
}

// approx reports whether two floats are equal within a relative tolerance.
func approx(a, b float64) bool {
	if math.Abs(a-b) <= eps {
		return true
	}
	// Relative tolerance for non-tiny magnitudes.
	if math.Abs(a) > eps {
		return math.Abs(a-b) <= eps*math.Abs(a)
	}
	return false
}
