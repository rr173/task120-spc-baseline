// Package capability computes process-capability indices from a chart's
// recomputed sigma values and its spec limits. Every formula is locked in the
// design doc; the self-check and consistency check assert against the same
// formulas. Indices that cannot be computed (no spec, <2 points, sigma==0,
// one-sided for the bilateral Cp/Pp/Cpm, or Cpm without a target) come back as
// a not-estimable status rather than a number — the engine never emits NaN.
package capability

import (
	"math"

	"task120-spc/internal/domain"
	"task120-spc/internal/stats"
)

// Input bundles the recomputed statistics and chart spec needed to compute
// indices. Mean is the process mean of the controlled quantity over the
// non-excluded baseline (individuals: mean of values; xbar: x-barbar; p_chart:
// p-bar).
type Input struct {
	USL         *float64
	LSL         *float64
	Target      *float64
	Mean        float64
	SigmaWithin float64
	SigmaOverall float64
}

// Compute returns the capability report. The Estimable flag is false when the
// hard preconditions (spec present, sigma_within > 0) fail; callers surface
// that as not_estimable. Individual indices are nil when their own
// preconditions fail (one-sided, no target).
func Compute(in Input) domain.Capability {
	c := domain.Capability{}
	// Hard preconditions: at least one spec limit and a usable within-sigma.
	if (in.USL == nil && in.LSL == nil) || in.SigmaWithin <= 0 {
		c.Estimable = false
		return c
	}
	c.Estimable = true

	if in.USL != nil {
		cpu := (*in.USL - in.Mean) / (3 * in.SigmaWithin)
		c.CPU = &cpu
	}
	if in.LSL != nil {
		cpl := (in.Mean - *in.LSL) / (3 * in.SigmaWithin)
		c.CPL = &cpl
	}
	c.Cpk = minPtr(c.CPU, c.CPL)

	// Ppk mirrors Cpk but on the overall sigma; needs sigma_overall > 0.
	if in.SigmaOverall > 0 {
		if in.USL != nil {
			ppu := (*in.USL - in.Mean) / (3 * in.SigmaOverall)
			c.PPU = &ppu
		}
		if in.LSL != nil {
			ppl := (in.Mean - *in.LSL) / (3 * in.SigmaOverall)
			c.PPL = &ppl
		}
		c.Ppk = minPtr(c.PPU, c.PPL)
	}

	// Bilateral indices require both spec limits.
	if in.USL != nil && in.LSL != nil {
		cp := (*in.USL - *in.LSL) / (6 * in.SigmaWithin)
		c.Cp = &cp
		if in.SigmaOverall > 0 {
			pp := (*in.USL - *in.LSL) / (6 * in.SigmaOverall)
			c.Pp = &pp
		}
		if in.Target != nil {
			root := math.Sqrt(in.SigmaOverall*in.SigmaOverall + (in.Mean-*in.Target)*(in.Mean-*in.Target))
			if root > 0 {
				cpm := (*in.USL - *in.LSL) / (6 * root)
				c.Cpm = &cpm
			}
		}
	}

	// DPMO and yield use the overall sigma and the normal CDF. They are
	// estimable whenever sigma_overall > 0 and at least one spec is present.
	if in.SigmaOverall > 0 {
		var tail float64
		if in.LSL != nil {
			tail += stats.Phi((*in.LSL - in.Mean) / in.SigmaOverall)
		}
		if in.USL != nil {
			tail += stats.Phi((in.Mean - *in.USL) / in.SigmaOverall)
		}
		dpmo := tail * 1e6
		c.DPMO = &dpmo
		y := 1 - dpmo/1e6
		c.Yield = &y
	}

	// Sigma level (short-term, no 1.5-shift convention) = 3 * Cpk.
	if c.Cpk != nil {
		sl := 3 * *c.Cpk
		c.SigmaLevel = &sl
	}

	return c
}

// minPtr returns the smaller of two *float64, or whichever is non-nil. Used to
// build Cpk/Ppk = min(CPU, CPL): the bilateral index must reflect the spec
// side the process mean sits closer to, which is the worse (smaller) of the two
// one-sided capabilities — not the better one.
func minPtr(a, b *float64) *float64 {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case *a < *b:
		return a
	default:
		return b
	}
}
