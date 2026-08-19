package chart

import (
	"context"
	"math"
	"task120-spc/internal/domain"
	"testing"
)

func TestBug01_ExcludeRefreshesDerivedState(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	c, e := s.CreateChart(context.Background(), "x", "x", domain.ChartIndividuals, "", 1, nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []float64{10, 12, 20} {
		if _, e = s.AddMeasurement(context.Background(), c.ChartID, []float64{v}, 0, 0); e != nil {
			t.Fatal(e)
		}
	}
	ms, _ := s.ListMeasurements(context.Background(), c.ChartID, false)
	if e = s.ExcludeMeasurement(context.Background(), ms[1].MeasurementID); e != nil {
		t.Fatal(e)
	}
	lim, e := s.GetLimit(context.Background(), c.ChartID)
	if e != nil || lim.BaselineCount != 2 || math.Abs(lim.CL-15) > 1e-12 {
		t.Fatalf("stale derived state: %#v %v", lim, e)
	}
	r, e := s.ConsistencyCheck(context.Background())
	if e != nil || !r.OK {
		t.Fatalf("consistency: %#v %v", r, e)
	}
}
