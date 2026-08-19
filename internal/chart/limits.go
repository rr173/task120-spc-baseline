package chart

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"task120-spc/internal/domain"
	"task120-spc/internal/stats"
	"task120-spc/internal/westgard"
)

// recompute is the engine's spine. It reloads the non-excluded baseline of the
// chart, recomputes the center line and within/overall sigma, writes the
// control_limits cache, then wipes and re-runs Westgard so the stored
// violations are exactly the recomputed set. Runs inside the caller's tx.
func (s *Service) recompute(ctx context.Context, tx *sql.Tx, chartID string, ct domain.ChartType, n int) error {
	// Load every measurement of the chart in subgroup order, including
	// excluded ones (we need the full history to know which are excluded).
	// Run on the tx's connection — with SetMaxOpenConns(1) using the pooled
	// store here would deadlock against the tx holding the single connection.
	all, err := s.me.ListMeasurementsByChart(ctx, tx, chartID, false)
	if err != nil {
		return fmt.Errorf("recompute load: %w", err)
	}
	// Filter to the non-excluded baseline preserving subgroup order.
	var baseline []domain.Measurement
	for _, m := range all {
		if !m.Excluded {
			baseline = append(baseline, m)
		}
	}

	lim, ok := s.computeLimits(ct, n, baseline)
	// Always upsert the cache; when not estimable we store a zero-ish limit so
	// callers see baseline_count=0 and the consistency check detects the empty
	// baseline deterministically.
	if !ok {
		lim = domain.ControlLimit{ChartID: chartID, BaselineCount: len(baseline)}
	}
	seq, err := s.st.NextSeq(ctx, tx)
	if err != nil {
		return err
	}
	lim.ChartID = chartID
	lim.ComputedSeq = seq
	if err := s.vi.PutLimit(ctx, tx, lim); err != nil {
		return err
	}

	// Back-fill individuals moving range now that the baseline is known.
	if ct == domain.ChartIndividuals {
		if err := s.backfillMR(ctx, tx, baseline); err != nil {
			return err
		}
	}

	// Wipe and re-run Westgard over the recomputed baseline.
	if err := s.vi.DeleteViolationsByChart(ctx, tx, chartID); err != nil {
		return err
	}
	if !ok {
		// No usable sigma: no violations can be derived.
		return nil
	}
	rules, err := s.ch.ListRules(ctx, tx, chartID)
	if err != nil {
		return err
	}
	points := buildPoints(ct, baseline, lim)
	violations := westgard.Detect(points, lim.CL, lim.SigmaWithin, rules)
	for _, v := range violations {
		v.ChartID = chartID
		seq, err := s.st.NextSeq(ctx, tx)
		if err != nil {
			return err
		}
		v.ViolationID = domain.ViolationID(seq)
		v.DetectedSeq = seq
		if err := s.vi.InsertViolation(ctx, tx, v); err != nil {
			return err
		}
	}
	return nil
}

// computeLimits derives the control-limit cache from the baseline. Returns the
// limit and an ok flag (false when the baseline is empty or sigma is not
// estimable). The formulas are locked in the design doc.
func (s *Service) computeLimits(ct domain.ChartType, n int, baseline []domain.Measurement) (domain.ControlLimit, bool) {
	if len(baseline) == 0 {
		return domain.ControlLimit{}, false
	}
	switch ct {
	case domain.ChartIndividuals:
		return computeIndividuals(baseline)
	case domain.ChartXbarR:
		return computeXbarR(baseline, n)
	case domain.ChartP:
		return computeP(baseline, n)
	}
	return domain.ControlLimit{}, false
}

// computeIndividuals derives the I-MR limits. sigma_within = MRbar / d2(2);
// CL = mean of values; UCL/LCL = CL +/- 3*sigma_within. sigma_overall is the
// Bessel std of all single values.
func computeIndividuals(baseline []domain.Measurement) (domain.ControlLimit, bool) {
	xs := valuesOf(baseline)
	mean := stats.Mean(xs)
	// Moving range over consecutive baseline points (excluded points already
	// filtered out, so MR is between truly adjacent in-control points).
	var mrs []float64
	for i := 1; i < len(baseline); i++ {
		mrs = append(mrs, math.Abs(baseline[i].ValueAvg-baseline[i-1].ValueAvg))
	}
	mrbar := stats.Mean(mrs)
	sigmaWithin := mrbar / stats.D2(2)
	sigmaOverall := stats.SampleStdDev(xs)
	cl := domain.ControlLimit{
		BaselineCount: len(baseline),
		CL:            mean,
		SigmaWithin:   sigmaWithin,
		SigmaOverall:  sigmaOverall,
	}
	if sigmaWithin > 0 {
		cl.UCL = mean + 3*sigmaWithin
		cl.LCL = mean - 3*sigmaWithin
	}
	// estimable when we have at least one MR (>=2 points) -> sigma>0.
	return cl, sigmaWithin > 0
}

// computeXbarR derives the X-bar/R limits. sigma_within = Rbar / d2(n);
// CL = xbarbar (mean of subgroup means); UCL/LCL = CL +/- A2(n)*Rbar.
func computeXbarR(baseline []domain.Measurement, n int) (domain.ControlLimit, bool) {
	means := valuesOf(baseline) // ValueAvg holds the subgroup mean
	xbarbar := stats.Mean(means)
	var ranges []float64
	for _, m := range baseline {
		ranges = append(ranges, m.RangeValue)
	}
	rbar := stats.Mean(ranges)
	sigmaWithin := rbar / stats.D2(n)
	// Overall sigma uses every individual value across subgroups.
	var all []float64
	for _, m := range baseline {
		all = append(all, m.Values...)
	}
	sigmaOverall := stats.SampleStdDev(all)
	cl := domain.ControlLimit{
		BaselineCount: len(baseline),
		CL:            xbarbar,
		SigmaWithin:   sigmaWithin,
		SigmaOverall:  sigmaOverall,
	}
	if sigmaWithin > 0 {
		cl.UCL = xbarbar + stats.A2(n)*rbar
		cl.LCL = xbarbar - stats.A2(n)*rbar
	}
	return cl, sigmaWithin > 0
}

// computeP derives the p-chart limits. CL = pbar; the per-point sigma varies,
// so the cache stores CL and the most recent point's UCL/LCL for display. The
// westgard detector receives per-point sigma via buildPoints.
func computeP(baseline []domain.Measurement, n int) (domain.ControlLimit, bool) {
	var totalDef, totalN int
	for _, m := range baseline {
		totalDef += m.Defectives
		totalN += m.NObserved
	}
	if totalN == 0 {
		return domain.ControlLimit{}, false
	}
	pbar := float64(totalDef) / float64(totalN)
	// sigma_overall for p_chart: std of the individual p_i values.
	var ps []float64
	for _, m := range baseline {
		ps = append(ps, m.ValueAvg)
	}
	sigmaOverall := stats.SampleStdDev(ps)
	// Use the last point's sigma for the displayed UCL/LCL.
	last := baseline[len(baseline)-1]
	sigmaP := math.Sqrt(pbar * (1 - pbar) / float64(last.NObserved))
	cl := domain.ControlLimit{
		BaselineCount: len(baseline),
		CL:            pbar,
		SigmaWithin:   sigmaP,
		SigmaOverall:  sigmaOverall,
	}
	if sigmaP > 0 {
		cl.UCL = pbar + 3*sigmaP
		cl.LCL = stats.ClampLower(pbar - 3*sigmaP)
	}
	return cl, sigmaP > 0
}

// buildPoints projects the baseline into the detector's input. For variables
// charts the sigma is the within-sigma (constant); for p_chart each point
// carries its own sigma derived from its n.
func buildPoints(ct domain.ChartType, baseline []domain.Measurement, lim domain.ControlLimit) []westgard.Point {
	pts := make([]westgard.Point, 0, len(baseline))
	for _, m := range baseline {
		p := westgard.Point{Seq: m.SubgroupSeq, Value: m.ValueAvg, CL: lim.CL}
		if ct == domain.ChartP {
			// per-point sigma; pbar==0 or ==1 -> sigma 0, but then the detector
			// skips (sigma<=0 guard).
			p.Sigma = math.Sqrt(lim.CL * (1 - lim.CL) / float64(m.NObserved))
		} else {
			p.Sigma = lim.SigmaWithin
		}
		pts = append(pts, p)
	}
	return pts
}

// backfillMR writes the moving range of each individuals point back into its
// row so GET /measurements shows the MR the limit was derived from. The first
// baseline point has MR=0 (no predecessor).
func (s *Service) backfillMR(ctx context.Context, tx *sql.Tx, baseline []domain.Measurement) error {
	for i, m := range baseline {
		var mr float64
		if i > 0 {
			mr = math.Abs(m.ValueAvg - baseline[i-1].ValueAvg)
		}
		if mr != m.RangeValue {
			if err := s.me.UpdateRange(ctx, tx, m.MeasurementID, mr); err != nil {
				return err
			}
		}
	}
	return nil
}

// valuesOf returns the ValueAvg slice of the measurements.
func valuesOf(ms []domain.Measurement) []float64 {
	out := make([]float64, len(ms))
	for i, m := range ms {
		out[i] = m.ValueAvg
	}
	return out
}
