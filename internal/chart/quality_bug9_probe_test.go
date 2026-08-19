package chart

import (
	"context"
	"math"
	"task120-spc/internal/domain"
	"testing"
)

func TestBug09_CpkUsesWorseSpecSide(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	c, e := s.CreateChart(context.Background(), "x", "x", domain.ChartIndividuals, "", 1, ptrR(20), ptrR(0), nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []float64{8, 9, 10} {
		s.AddMeasurement(context.Background(), c.ChartID, []float64{v}, 0, 0)
	}
	lim, _ := s.GetLimit(context.Background(), c.ChartID)
	cap, _ := s.GetCapability(context.Background(), c.ChartID)
	want := math.Min((*c.USL-lim.CL)/(3*lim.SigmaWithin), (lim.CL-*c.LSL)/(3*lim.SigmaWithin))
	if cap.Cpk == nil || math.Abs(*cap.Cpk-want) > 1e-12 {
		t.Fatalf("Cpk=%v want %v", cap.Cpk, want)
	}
}
func ptrR(v float64) *float64 { return &v }
