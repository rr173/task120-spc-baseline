package chart

import (
	"context"
	"testing"

	"task120-spc/internal/domain"
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

// TestBug07_InterleavedSubgroupSequenceIsolated exercises the exact scenario
// from the bug report: points are added across two charts in an interleaved
// fashion. Each chart must count its own subgroups 1,2,3,… independently, and a
// violation recorded on one chart must stamp that chart's subgroup number —
// never a number borrowed from the other chart's counter.
func TestBug07_InterleavedSubgroupSequenceIsolated(t *testing.T) {
	s, cl := newService(t)
	defer cl()
	ctx := context.Background()
	// Wide spec so the 100 outlier trips 1-3s but the small points stay in control.
	usl, lsl := 200.0, -200.0
	a, err := s.CreateChart(ctx, "a", "a", domain.ChartIndividuals, "mm", 1, &usl, &lsl, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateChart(ctx, "b", "b", domain.ChartIndividuals, "mm", 1, &usl, &lsl, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Interleave: a1, b1, a2, b2, a3, b3, a4, b4, a5(outlier), b5(outlier).
	// Each chart gets four tight baseline points then one large outlier, so the
	// outlier is the 5th subgroup of its own chart and trips 1-3s there.
	seq := func(m domain.Measurement) int { return m.SubgroupSeq }
	pairs := []struct {
		chart string
		val   float64
	}{
		{a.ChartID, 10}, {b.ChartID, 20},
		{a.ChartID, 10}, {b.ChartID, 20},
		{a.ChartID, 10}, {b.ChartID, 20},
		{a.ChartID, 10}, {b.ChartID, 20},
		{a.ChartID, 1000}, {b.ChartID, 2000},
	}
	var aOut, bOut domain.Measurement
	for i, p := range pairs {
		m, err := s.AddMeasurement(ctx, p.chart, []float64{p.val}, 0, 0)
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		// Each chart must see 1,2,3,4,5 regardless of the other chart's points.
		want := i/2 + 1
		if got := seq(m); got != want {
			t.Fatalf("chart %s point %d: subgroup_seq %d, want %d (per-chart isolation broken)",
				p.chart, i, got, want)
		}
		if p.chart == a.ChartID && p.val == 1000 {
			aOut = m
		}
		if p.chart == b.ChartID && p.val == 2000 {
			bOut = m
		}
	}

	// Each outlier must be the 5th subgroup of its own chart, not a value
	// pulled from the shared global counter.
	if aOut.SubgroupSeq != 5 {
		t.Fatalf("chart a outlier seq %d, want 5", aOut.SubgroupSeq)
	}
	if bOut.SubgroupSeq != 5 {
		t.Fatalf("chart b outlier seq %d, want 5", bOut.SubgroupSeq)
	}

	// Event records (violations) must be isolated per chart too: the 1-3s hit on
	// each chart stamps that chart's own subgroup number (5), never the other
	// chart's counter.
	for _, c := range []struct {
		id   string
		name string
	}{
		{a.ChartID, "a"}, {b.ChartID, "b"},
	} {
		vs, err := s.ListViolations(ctx, c.id)
		if err != nil {
			t.Fatalf("list violations %s: %v", c.name, err)
		}
		var hit bool
		for _, v := range vs {
			if v.Rule == domain.Rule13s {
				hit = true
				if v.MeasurementSeq != 5 {
					t.Fatalf("chart %s 1-3s stamped seq %d, want 5 (event record not isolated per chart)",
						c.name, v.MeasurementSeq)
				}
			}
		}
		if !hit {
			t.Fatalf("chart %s: expected a 1-3s violation at subgroup 5", c.name)
		}
	}
}
