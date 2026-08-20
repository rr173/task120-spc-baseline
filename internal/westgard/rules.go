// Package westgard implements the Westgard multi-rule quality-control detector.
// Given the chronological baseline of a chart (the controlled quantity per
// point with its CL and per-point sigma) and the per-chart rule enable map,
// Detect returns the full set of violations a fresh re-run would produce. The
// chart service calls Detect after every mutation and stores exactly this set,
// so the stored violations are a pure function of the measurements and limits
// — which is what the restart-recovery and consistency checks rely on.
//
// Detect evaluates every point in chronological order: for point i it looks at
// the prefix points[0..i] and asks "which rules trip at point i, given the
// history up to and including i?". Re-running over the same baseline therefore
// reproduces the identical violation multiset — the engine never loses an
// earlier violation on recompute. A single point may trip several rules and
// each hit is a separate violation.
//
// The "z" deviation is (value - CL) / sigma. For variables charts sigma is the
// within-subgroup sigma (constant across points); for p_chart each point
// carries its own sigma (p(1-p)/n).
package westgard

import "task120-spc/internal/domain"

// Point is one chronological baseline point fed to the detector. Value is the
// controlled quantity; Sigma is the within-subgroup sigma at that point (the
// same for all points on variables charts; per-point for p_chart). CL is the
// center line. Seq is the subgroup sequence number to stamp on violations.
type Point struct {
	Seq   int
	Value float64
	Sigma float64
	CL    float64
}

// Detect runs all enabled Westgard rules over the chronological baseline and
// returns the violations. It never errors: rules that need more history than
// is present simply do not fire.
//
// Rule semantics at point i (z = (value_i - CL) / sigma_i):
//   - 1-2s: |z| > 2                              -> warning
//   - 1-3s: |z| > 3                              -> reject
//   - 2-2s: points i-1,i same side and both |z|>2 -> reject
//   - R-4s: points i-1,i opposite sides, both |z|>2, |v_i - v_{i-1}| > 4*sigma_i -> reject
//   - 4-1s: points i-3..i same side and all |z|>1 -> reject
//   - 10-x: points i-9..i all on the same side of CL -> reject
func Detect(points []Point, cl, sigma float64, enabled map[domain.RuleName]bool) []domain.Violation {
	if len(points) == 0 || sigma <= 0 {
		// Without a baseline or a non-zero sigma there are no zones to test.
		return nil
	}
	var out []domain.Violation
	for i := 0; i < len(points); i++ {
		sub := points[:i+1]
		last := sub[len(sub)-1]
		if last.Sigma <= 0 {
			continue // a point with no estimable sigma cannot trip a zone rule
		}
		z := (last.Value - last.CL) / last.Sigma

		// 1-2s and 1-3s test the current point alone.
		if enabled[domain.Rule12s] && absf(z) > 2 {
			out = append(out, violation(domain.Rule12s, last.Seq))
		}
		if enabled[domain.Rule13s] && absf(z) > 3 {
			out = append(out, violation(domain.Rule13s, last.Seq))
		}

		// 2-2s and R-4s need the previous point too.
		if len(sub) >= 2 {
			prev := sub[len(sub)-2]
			if prev.Sigma > 0 {
				zPrev := (prev.Value - prev.CL) / prev.Sigma
				if enabled[domain.Rule22s] &&
					sameSign(last.Value-last.CL, prev.Value-prev.CL) &&
					absf(z) > 2 && absf(zPrev) > 2 {
					out = append(out, violation(domain.Rule22s, last.Seq))
				}
				if enabled[domain.RuleR4s] &&
					!sameSign(last.Value-last.CL, prev.Value-prev.CL) &&
					absf(z) > 2 && absf(zPrev) > 2 &&
					absf(last.Value-prev.Value) > 4*last.Sigma {
					out = append(out, violation(domain.RuleR4s, last.Seq))
				}
			}
		}

		// 4-1s needs the last 4 points on the same side beyond 1 sigma.
		if len(sub) >= 4 && enabled[domain.Rule41s] {
			side := sign(last.Value - last.CL)
			if side != 0 {
				ok := true
				for j := len(sub) - 1; j >= len(sub)-4 && ok; j-- {
					p := sub[j]
					if p.Sigma <= 0 || sign(p.Value-p.CL) != side || absf((p.Value-p.CL)/p.Sigma) <= 1 {
						ok = false
					}
				}
				if ok {
					out = append(out, violation(domain.Rule41s, last.Seq))
				}
			}
		}

		// 10-x needs the last 10 points on the same side of CL.
		if len(sub) >= 10 && enabled[domain.Rule10x] {
			side := sign(last.Value - last.CL)
			if side != 0 {
				ok := true
				for j := len(sub) - 1; j >= len(sub)-10 && ok; j-- {
					if sign(sub[j].Value-sub[j].CL) != side {
						ok = false
					}
				}
				if ok {
					out = append(out, violation(domain.Rule10x, last.Seq))
				}
			}
		}
	}
	return out
}

// violation builds a violation with the rule stamped; the chart service fills
// the IDs and chart_id when persisting.
func violation(rule domain.RuleName, seq int) domain.Violation {
	return domain.Violation{
		MeasurementSeq: seq,
		Rule:           rule,
		Severity:       domain.SeverityOf(rule),
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// sameSign reports whether two non-zero deviations share a sign. Zero is
// treated as neither, so a point exactly on CL breaks any "same side" run.
func sameSign(a, b float64) bool {
	return sign(a) != 0 && sign(a) == sign(b)
}
