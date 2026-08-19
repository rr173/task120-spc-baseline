package chart

import (
	"context"
	"task120-spc/internal/domain"
	"testing"
)

func TestBug03_XbarOverallSigmaUsesRawValues(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	c, e := s.CreateChart(context.Background(), "x", "x", domain.ChartXbarR, "", 2, ptrQ(20), ptrQ(0), nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range [][]float64{{0, 10}, {4, 6}} {
		if _, e = s.AddMeasurement(context.Background(), c.ChartID, v, 0, 0); e != nil {
			t.Fatal(e)
		}
	}
	lim, _ := s.GetLimit(context.Background(), c.ChartID)
	if lim.SigmaOverall <= 0 {
		t.Fatalf("overall sigma lost within-subgroup variation: %#v", lim)
	}
	cap, _ := s.GetCapability(context.Background(), c.ChartID)
	if cap.Pp == nil {
		t.Fatal("Pp missing")
	}
}
func ptrQ(v float64) *float64 { return &v }
