package chart

import (
	"context"
	"task120-spc/internal/domain"
	"testing"
)

func TestBug02_PChartUsesPerPointSigma(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	c, e := s.CreateChart(context.Background(), "p", "p", domain.ChartP, "", 1, nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddMeasurement(context.Background(), c.ChartID, nil, 1, 4); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddMeasurement(context.Background(), c.ChartID, nil, 10, 100); e != nil {
		t.Fatal(e)
	}
	ms, _ := s.ListMeasurements(context.Background(), c.ChartID, false)
	lim, _ := s.GetLimit(context.Background(), c.ChartID)
	pts := buildPoints(domain.ChartP, ms, lim)
	if len(pts) != 2 || pts[0].Sigma == pts[1].Sigma {
		t.Fatalf("p-chart points share sigma: %#v", pts)
	}
}
