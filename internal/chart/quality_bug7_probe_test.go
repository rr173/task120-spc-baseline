package chart

import (
	"context"
	"task120-spc/internal/domain"
	"testing"
)

func TestBug07_SubgroupSequenceIsPerChart(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	a, _ := s.CreateChart(context.Background(), "a", "a", domain.ChartIndividuals, "", 1, nil, nil, nil)
	b, _ := s.CreateChart(context.Background(), "b", "b", domain.ChartIndividuals, "", 1, nil, nil, nil)
	ma, _ := s.AddMeasurement(context.Background(), a.ChartID, []float64{1}, 0, 0)
	mb, _ := s.AddMeasurement(context.Background(), b.ChartID, []float64{1}, 0, 0)
	if ma.SubgroupSeq != 1 || mb.SubgroupSeq != 1 {
		t.Fatalf("cross-chart sequence: %d %d", ma.SubgroupSeq, mb.SubgroupSeq)
	}
}
