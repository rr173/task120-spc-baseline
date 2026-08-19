package chart

import (
	"context"
	"task120-spc/internal/domain"
	"testing"
)

func TestBug06_RuleToggleRefreshesViolations(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	c, e := s.CreateChart(context.Background(), "x", "x", domain.ChartIndividuals, "", 1, nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []float64{0, 1, 10} {
		s.AddMeasurement(context.Background(), c.ChartID, []float64{v}, 0, 0)
	}
	if e = s.SetRules(context.Background(), c.ChartID, map[domain.RuleName]bool{domain.Rule13s: false}); e != nil {
		t.Fatal(e)
	}
	rules, e := s.GetRules(context.Background(), c.ChartID)
	if e != nil || rules[domain.Rule13s] {
		t.Fatalf("disabled rule was not persisted: %#v %v", rules, e)
	}
	vs, _ := s.ListViolations(context.Background(), c.ChartID)
	for _, v := range vs {
		if v.Rule == domain.Rule13s {
			t.Fatal("disabled rule still persisted")
		}
	}
}
