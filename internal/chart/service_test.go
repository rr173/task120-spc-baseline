package chart

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"task120-spc/internal/domain"
	"task120-spc/internal/store"
)

func newService(t *testing.T) (*Service, func()) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return New(st), func() { _ = st.Close() }
}

func TestCreateChartValidatesType(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	_, err := svc.CreateChart(ctx, "n", "c", "bogus", "mm", 1, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for bad chart type")
	}
}

func TestCreateChartXbarRSizeGuard(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	if _, err := svc.CreateChart(ctx, "n", "c", domain.ChartXbarR, "mm", 1, nil, nil, nil); err == nil {
		t.Fatal("expected error for n=1 xbar_r")
	}
	if _, err := svc.CreateChart(ctx, "n", "c", domain.ChartXbarR, "mm", 11, nil, nil, nil); err == nil {
		t.Fatal("expected error for n=11 xbar_r")
	}
}

func TestAddMeasurementIndividuals(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	c, err := svc.CreateChart(ctx, "n", "c", domain.ChartIndividuals, "mm", 1, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := svc.AddMeasurement(ctx, c.ChartID, []float64{10}, 0, 0)
	if err != nil {
		t.Fatalf("add measurement: %v", err)
	}
	if m.ValueAvg != 10 {
		t.Errorf("value_avg %g != 10", m.ValueAvg)
	}
}

func TestAddMeasurementXbarRMeanAndRange(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	c, err := svc.CreateChart(ctx, "n", "c", domain.ChartXbarR, "mm", 3, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := svc.AddMeasurement(ctx, c.ChartID, []float64{2, 4, 6}, 0, 0)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if m.ValueAvg != 4 {
		t.Errorf("mean %g != 4", m.ValueAvg)
	}
	if m.RangeValue != 4 {
		t.Errorf("range %g != 4", m.RangeValue)
	}
}

func TestAddMeasurementXbarRSizeMismatch(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	c, err := svc.CreateChart(ctx, "n", "c", domain.ChartXbarR, "mm", 3, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddMeasurement(ctx, c.ChartID, []float64{1, 2}, 0, 0)
	if err == nil {
		t.Fatal("expected size mismatch error")
	}
}

func TestAddMeasurementPChartBounds(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	c, err := svc.CreateChart(ctx, "n", "c", domain.ChartP, "cnt", 1, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddMeasurement(ctx, c.ChartID, nil, 5, 3); err == nil {
		t.Fatal("expected defectives>n error")
	}
	m, err := svc.AddMeasurement(ctx, c.ChartID, nil, 1, 4)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if math.Abs(m.ValueAvg-0.25) > 1e-9 {
		t.Errorf("p %g != 0.25", m.ValueAvg)
	}
}

func TestIndividualsLimitsRecompute(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	c, err := svc.CreateChart(ctx, "n", "c", domain.ChartIndividuals, "mm", 1, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{10, 12, 11, 9, 10} {
		if _, err := svc.AddMeasurement(ctx, c.ChartID, []float64{v}, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	lim, err := svc.GetLimit(ctx, c.ChartID)
	if err != nil {
		t.Fatalf("get limit: %v", err)
	}
	if lim.BaselineCount != 5 {
		t.Errorf("baseline %d != 5", lim.BaselineCount)
	}
	// CL = mean(10,12,11,9,10) = 10.4
	if math.Abs(lim.CL-10.4) > 1e-9 {
		t.Errorf("CL %g != 10.4", lim.CL)
	}
}

func TestExcludeRecomputeChangesBaseline(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	c, err := svc.CreateChart(ctx, "n", "c", domain.ChartIndividuals, "mm", 1, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{10, 12, 11, 9, 10} {
		_, _ = svc.AddMeasurement(ctx, c.ChartID, []float64{v}, 0, 0)
	}
	before, _ := svc.GetLimit(ctx, c.ChartID)
	ms, _ := svc.ListMeasurements(ctx, c.ChartID, false)
	var mid string
	for _, m := range ms {
		if m.SubgroupSeq == 2 {
			mid = m.MeasurementID
		}
	}
	if err := svc.ExcludeMeasurement(ctx, mid); err != nil {
		t.Fatalf("exclude: %v", err)
	}
	after, _ := svc.GetLimit(ctx, c.ChartID)
	if after.BaselineCount != 4 {
		t.Errorf("after baseline %d != 4", after.BaselineCount)
	}
	// Restoring brings it back.
	if err := svc.RestoreMeasurement(ctx, mid); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, _ := svc.GetLimit(ctx, c.ChartID)
	if restored.BaselineCount != 5 || math.Abs(restored.CL-before.CL) > 1e-9 {
		t.Errorf("restore mismatch: before %+v restored %+v", before, restored)
	}
}

func TestConsistencyCheckOK(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	usl, lsl := 30.0, -10.0
	c, err := svc.CreateChart(ctx, "n", "c", domain.ChartIndividuals, "mm", 1, &usl, &lsl, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{10, 12, 11, 9, 10, 12, 11, 9} {
		_, _ = svc.AddMeasurement(ctx, c.ChartID, []float64{v}, 0, 0)
	}
	rep, err := svc.ConsistencyCheck(ctx)
	if err != nil {
		t.Fatalf("consistency: %v", err)
	}
	if !rep.OK {
		t.Errorf("consistency not ok: %+v", rep)
		for _, c := range rep.Checks {
			t.Logf("check: %+v", c)
		}
	}
}

// TestXbarROverallSigmaFromRawObservations locks the bug where identical
// subgroup means (no between-subgroup drift) still left within-subgroup
// variation in the raw observations: sigma_overall and the capability report's
// overall-sigma indices must reflect that raw variation, not collapse to 0.
// Subgroups {1,5}, {2,4}, {3,3}, {4,2}, {5,1} all have mean 3 -> if sigma_overall
// were (wrongly) computed over the means it would be 0 and Ppk/Pp/Cpm/DPMO/Yield
// would all come back not-estimable / nil. Over the raw observations the std is
// ~1.5811 (population) / sqrt(2) for Bessel's n-1; we only assert it is > 0 and
// that the overall-based capability indices are present.
func TestXbarROverallSigmaFromRawObservations(t *testing.T) {
	svc, closeFn := newService(t)
	defer closeFn()
	ctx := context.Background()
	usl, lsl, target := 30.0, -10.0, 3.0
	c, err := svc.CreateChart(ctx, "xr", "c", domain.ChartXbarR, "mm", 2, &usl, &lsl, &target)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range [][]float64{{1, 5}, {2, 4}, {3, 3}, {4, 2}, {5, 1}} {
		if _, err := svc.AddMeasurement(ctx, c.ChartID, sub, 0, 0); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	lim, err := svc.GetLimit(ctx, c.ChartID)
	if err != nil {
		t.Fatalf("get limit: %v", err)
	}
	// Every subgroup mean is 3, so the (buggy) mean-of-means sigma would be 0.
	if lim.CL != 3 {
		t.Errorf("CL %g != 3", lim.CL)
	}
	if lim.SigmaOverall <= 0 {
		t.Fatalf("sigma_overall %g must be > 0 (raw observations vary even though means are equal)", lim.SigmaOverall)
	}
	// sigma_within comes from Rbar/d2(n); the ranges are all 4, so it is > 0.
	if lim.SigmaWithin <= 0 {
		t.Fatalf("sigma_within %g must be > 0", lim.SigmaWithin)
	}
	// Capability report: the overall-sigma indices (Ppk/Pp/Cpm/DPMO/Yield) must
	// be present because sigma_overall > 0 — the bug would leave them nil.
	cap, err := svc.GetCapability(ctx, c.ChartID)
	if err != nil {
		t.Fatalf("get capability: %v", err)
	}
	if !cap.Estimable {
		t.Fatal("capability should be estimable")
	}
	if cap.Ppk == nil {
		t.Error("Ppk should be present (sigma_overall>0)")
	}
	if cap.Pp == nil {
		t.Error("Pp should be present (two-sided spec, sigma_overall>0)")
	}
	if cap.Cpm == nil {
		t.Error("Cpm should be present (target set, sigma_overall>0)")
	}
	if cap.DPMO == nil || cap.Yield == nil {
		t.Error("DPMO and Yield should be present (sigma_overall>0)")
	}
	// Consistency must still hold after the fix: cache == recomputed.
	rep, err := svc.ConsistencyCheck(ctx)
	if err != nil {
		t.Fatalf("consistency: %v", err)
	}
	if !rep.OK {
		t.Errorf("consistency not ok after fix: %+v", rep)
	}
}
