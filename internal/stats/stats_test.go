package stats

import (
	"math"
	"testing"
)

func TestMean(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{5}, 5},
		{[]float64{1, 2, 3, 4}, 2.5},
		{[]float64{-1, 1}, 0},
	}
	for _, c := range cases {
		if got := Mean(c.in); got != c.want {
			t.Errorf("Mean(%v)=%g want %g", c.in, got, c.want)
		}
	}
}

func TestRange(t *testing.T) {
	if got := Range([]float64{3, 1, 2, 5, 4}); got != 4 {
		t.Errorf("Range=%g want 4", got)
	}
	if got := Range([]float64{7}); got != 0 {
		t.Errorf("Range single=%g want 0", got)
	}
}

func TestSampleStdDev(t *testing.T) {
	// Population [2,4,4,4,5,5,7,9]: sample stddev (Bessel) = 2.138...
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	got := SampleStdDev(xs)
	want := 2.138089935299395
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("SampleStdDev=%g want %g", got, want)
	}
	if SampleStdDev([]float64{1}) != 0 {
		t.Error("SampleStdDev of single point should be 0")
	}
}

func TestPhi(t *testing.T) {
	// Phi(0)=0.5, Phi(1.96)~=0.975, Phi(-1.96)~=0.025.
	if got := Phi(0); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("Phi(0)=%g want 0.5", got)
	}
	if got := Phi(1.96); math.Abs(got-0.975) > 1e-3 {
		t.Errorf("Phi(1.96)=%g want ~0.975", got)
	}
	if got := Phi(-1.96); math.Abs(got-0.025) > 1e-3 {
		t.Errorf("Phi(-1.96)=%g want ~0.025", got)
	}
}

func TestConstants(t *testing.T) {
	if D2(2) != 1.128 {
		t.Errorf("D2(2)=%g want 1.128", D2(2))
	}
	if D4(2) != 3.267 {
		t.Errorf("D4(2)=%g want 3.267", D4(2))
	}
	if D3(7) != 0.076 {
		t.Errorf("D3(7)=%g want 0.076", D3(7))
	}
	if A2(5) != 0.577 {
		t.Errorf("A2(5)=%g want 0.577", A2(5))
	}
}

func TestClampLower(t *testing.T) {
	if ClampLower(-1.5) != 0 {
		t.Error("ClampLower(-1.5) should be 0")
	}
	if ClampLower(2.0) != 2.0 {
		t.Error("ClampLower(2) should be 2")
	}
}
