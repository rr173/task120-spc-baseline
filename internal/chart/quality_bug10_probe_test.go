package chart

import (
	"context"
	"task120-spc/internal/domain"
	"testing"
)

func TestBug10_BatchRuleTogglesAllPersist(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	c, _ := s.CreateChart(context.Background(), "x", "x", domain.ChartIndividuals, "", 1, nil, nil, nil)
	e := s.SetRules(context.Background(), c.ChartID, map[domain.RuleName]bool{domain.Rule12s: false, domain.Rule13s: false})
	if e != nil {
		t.Fatal(e)
	}
	rs, e := s.GetRules(context.Background(), c.ChartID)
	if e != nil {
		t.Fatal(e)
	}
	if rs[domain.Rule12s] || rs[domain.Rule13s] {
		t.Fatalf("batch toggles not persisted: %#v", rs)
	}
}
