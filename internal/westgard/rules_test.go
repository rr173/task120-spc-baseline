package westgard

import (
	"testing"

	"task120-spc/internal/domain"
)

// pts builds a variables-chart baseline with a constant sigma.
func pts(vals []float64, cl, sigma float64) []Point {
	out := make([]Point, len(vals))
	for i, v := range vals {
		out[i] = Point{Seq: i + 1, Value: v, Sigma: sigma, CL: cl}
	}
	return out
}

func allOn() map[domain.RuleName]bool {
	m := make(map[domain.RuleName]bool, domain.AllRules)
	for _, r := range domain.WestgardRules {
		m[r] = true
	}
	return m
}

func TestDetectEmptyOrNoSigma(t *testing.T) {
	if got := Detect(nil, 10, 1, allOn()); got != nil {
		t.Errorf("empty -> nil, got %v", got)
	}
	if got := Detect(pts([]float64{9, 11}, 10, 0), 10, 0, allOn()); got != nil {
		t.Errorf("sigma=0 -> nil, got %v", got)
	}
}

func TestDetect13s(t *testing.T) {
	// CL=0, sigma=1; a point at 4 -> |z|=4 > 3 -> 1-3s.
	ps := pts([]float64{0, 0, 0, 0, 4}, 0, 1)
	vs := Detect(ps, 0, 1, allOn())
	if !has(vs, 5, "1-3s") {
		t.Errorf("expected 1-3s at seq 5, got %+v", vs)
	}
}

func TestDetect22s(t *testing.T) {
	// Two consecutive +2.5 points beyond 2 sigma on the same side.
	ps := pts([]float64{0, 0, 2.5, 2.5}, 0, 1)
	vs := Detect(ps, 0, 1, allOn())
	if !has(vs, 4, "2-2s") {
		t.Errorf("expected 2-2s at seq 4, got %+v", vs)
	}
}

func TestDetectR4s(t *testing.T) {
	// Opposite-side points beyond 2 sigma: +3 then -3 -> range 6 > 4*sigma.
	ps := pts([]float64{0, 0, 3, -3}, 0, 1)
	vs := Detect(ps, 0, 1, allOn())
	if !has(vs, 4, "R-4s") {
		t.Errorf("expected R-4s at seq 4, got %+v", vs)
	}
}

func TestDetect41s(t *testing.T) {
	// Four consecutive +2 points beyond 1 sigma on the same side.
	ps := pts([]float64{0, 2, 2, 2, 2}, 0, 1)
	vs := Detect(ps, 0, 1, allOn())
	if !has(vs, 5, "4-1s") {
		t.Errorf("expected 4-1s at seq 5, got %+v", vs)
	}
}

func TestDetect10x(t *testing.T) {
	// Ten points all positive -> 10-x at seq 10.
	ps := pts([]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, 0, 1)
	vs := Detect(ps, 0, 1, allOn())
	if !has(vs, 10, "10-x") {
		t.Errorf("expected 10-x at seq 10, got %+v", vs)
	}
}

func TestDetectRuleToggle(t *testing.T) {
	// With 1-3s disabled, a 1-3s point produces no 1-3s violation.
	ps := pts([]float64{0, 0, 0, 0, 4}, 0, 1)
	on := allOn()
	on[domain.Rule13s] = false
	vs := Detect(ps, 0, 1, on)
	if has(vs, 5, "1-3s") {
		t.Errorf("1-3s disabled but violation present: %+v", vs)
	}
}

func TestDetect12sIsWarning(t *testing.T) {
	// A point at 2.5 (|z|>2 but not >3) -> 1-2s warning, not 1-3s.
	ps := pts([]float64{0, 0, 0, 0, 2.5}, 0, 1)
	vs := Detect(ps, 0, 1, allOn())
	if !has(vs, 5, "1-2s") {
		t.Errorf("expected 1-2s at seq 5, got %+v", vs)
	}
	if has(vs, 5, "1-3s") {
		t.Errorf("did not expect 1-3s at seq 5")
	}
	for _, v := range vs {
		if v.Rule == "1-2s" && v.Severity != domain.SeverityWarning {
			t.Errorf("1-2s severity %q want warning", v.Severity)
		}
	}
}

func has(vs []domain.Violation, seq int, rule string) bool {
	for _, v := range vs {
		if v.MeasurementSeq == seq && string(v.Rule) == rule {
			return true
		}
	}
	return false
}
