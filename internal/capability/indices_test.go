package capability

import (
	"math"
	"testing"
)

func ptr(v float64) *float64 { return &v }

func TestComputeNoSpec(t *testing.T) {
	c := Compute(Input{Mean: 5, SigmaWithin: 1, SigmaOverall: 1})
	if c.Estimable {
		t.Error("no spec should be not estimable")
	}
}

func TestComputeNoSigma(t *testing.T) {
	c := Compute(Input{USL: ptr(10), Mean: 5, SigmaWithin: 0})
	if c.Estimable {
		t.Error("sigma_within=0 should be not estimable")
	}
}

func TestComputeTwoSided(t *testing.T) {
	// USL=20, LSL=0, mean=10, sigma_within=1, sigma_overall=1.
	c := Compute(Input{USL: ptr(20), LSL: ptr(0), Target: ptr(10), Mean: 10, SigmaWithin: 1, SigmaOverall: 1})
	if !c.Estimable {
		t.Fatal("should be estimable")
	}
	if c.Cp == nil || math.Abs(*c.Cp-20.0/6) > 1e-9 {
		t.Errorf("Cp %v want %g", c.Cp, 20.0/6)
	}
	// Cpk = min((20-10)/3, (10-0)/3) = 10/3.
	if c.Cpk == nil || math.Abs(*c.Cpk-10.0/3) > 1e-9 {
		t.Errorf("Cpk %v want %g", c.Cpk, 10.0/3)
	}
	// Cpm with mean==target -> same as Cp.
	if c.Cpm == nil || math.Abs(*c.Cpm-20.0/6) > 1e-9 {
		t.Errorf("Cpm %v want %g", c.Cpm, 20.0/6)
	}
	// sigma level = 3*Cpk = 10.
	if c.SigmaLevel == nil || math.Abs(*c.SigmaLevel-10.0) > 1e-9 {
		t.Errorf("sigma level %v want 10", c.SigmaLevel)
	}
}

func TestComputeOneSided(t *testing.T) {
	// Only USL: Cp/Pp/Cpm nil; CPU/Cpk present.
	c := Compute(Input{USL: ptr(20), Mean: 10, SigmaWithin: 1, SigmaOverall: 1})
	if !c.Estimable {
		t.Fatal("one-sided should be estimable")
	}
	if c.Cp != nil {
		t.Error("one-sided Cp should be nil")
	}
	if c.Cpm != nil {
		t.Error("one-sided Cpm should be nil")
	}
	if c.CPU == nil || math.Abs(*c.CPU-(20-10)/3.0) > 1e-9 {
		t.Errorf("CPU %v want %g", c.CPU, (20-10)/3.0)
	}
	if c.Cpk == nil {
		t.Error("one-sided Cpk should be present")
	}
}

func TestComputeDPMO(t *testing.T) {
	// Symmetric: USL=13, LSL=7, mean=10, sigma_overall=1.
	// tail = Phi((7-10)/1) + Phi((10-13)/1) = 2*Phi(-3) ~= 2*0.00135 = 0.0027.
	// DPMO ~= 2700.
	c := Compute(Input{USL: ptr(13), LSL: ptr(7), Mean: 10, SigmaWithin: 1, SigmaOverall: 1})
	if c.DPMO == nil {
		t.Fatal("DPMO nil")
	}
	if math.Abs(*c.DPMO-2*(1-0.99865)*1e6) > 5 {
		t.Errorf("DPMO %g want ~2700", *c.DPMO)
	}
	if c.Yield == nil || *c.Yield <= 0.997 || *c.Yield > 0.998 {
		t.Errorf("yield %v want ~0.9973", c.Yield)
	}
}
