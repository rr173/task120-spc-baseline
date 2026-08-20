// Package stats holds the statistical primitives and the locked SPC constant
// tables used to derive control limits and capability indices. Every formula
// here is deterministic and documented in the project design doc; the engine's
// self-check and consistency check assert against these same formulas, so the
// engine is self-consistent across a restart.
package stats

import "math"

// Constants for subgroup sizes n=2..10, the standard SPC control-chart
// factors (ASTM/ASQ B3). individuals charts reuse the n=2 row.
//
//   d2: unbiasing factor for the range -> sigma_within = Rbar / d2.
//   A2: factor for the X-bar control limits  = 3 / (d2 * sqrt(n)).
//   D3: factor for the lower range limit (0 for n<7 -> no LCL).
//   D4: factor for the upper range limit.
type factors struct {
	D2, A2, D3, D4 float64
}

var spcFactors = map[int]factors{
	2:  {1.128, 1.880, 0.0, 3.267},
	3:  {1.693, 1.023, 0.0, 2.574},
	4:  {2.059, 0.729, 0.0, 2.282},
	5:  {2.326, 0.577, 0.0, 2.114},
	6:  {2.534, 0.483, 0.0, 2.004},
	7:  {2.704, 0.419, 0.076, 1.924},
	8:  {2.847, 0.373, 0.136, 1.864},
	9:  {2.970, 0.337, 0.184, 1.816},
	10: {3.078, 0.308, 0.223, 1.777},
}

// D2 returns the d2 unbiasing factor for subgroup size n (n in [2,10]).
// individuals callers pass n=2.
func D2(n int) float64 { return spcFactors[n].D2 }

// A2 returns the X-bar control-limit factor A2 for subgroup size n.
func A2(n int) float64 { return spcFactors[n].A2 }

// D3 returns the lower range-limit factor D3 for subgroup size n (0 for n<7).
func D3(n int) float64 { return spcFactors[n].D3 }

// D4 returns the upper range-limit factor D4 for subgroup size n.
func D4(n int) float64 { return spcFactors[n].D4 }

// Mean returns the arithmetic mean of xs, or 0 for an empty slice.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// Range returns max(xs)-min(xs), the subgroup range. Returns 0 for an empty or
// single-element slice.
func Range(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	mn, mx := xs[0], xs[0]
	for _, x := range xs[1:] {
		if x < mn {
			mn = x
		}
		if x > mx {
			mx = x
		}
	}
	return mx - mn
}

// SampleStdDev returns the sample standard deviation (Bessel's correction,
// dividing by n-1) of xs. Returns 0 when fewer than 2 points are present so the
// caller can treat sigma as "not estimable" rather than NaN.
func SampleStdDev(xs []float64) float64 {
	n := len(xs)
	if n < 2 {
		return 0
	}
	m := Mean(xs)
	var ss float64
	for _, x := range xs {
		d := x - m
		ss += d * d
	}
	return math.Sqrt(ss / float64(n-1))
}

// Phi is the standard normal CDF, Phi(z) = 0.5 * (1 + erf(z / sqrt(2))). Used
// for DPMO. math.Erf is the Go standard library implementation.
func Phi(z float64) float64 {
	return 0.5 * (1 + math.Erf(z/math.Sqrt2))
}

// ClampLower returns max(0, v) so p-chart lower control limits never go negative.
func ClampLower(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}
