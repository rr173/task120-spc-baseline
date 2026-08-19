package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"task120-spc/internal/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOpenIdempotentSchema(t *testing.T) {
	st := newTestStore(t)
	// Re-applying schema on the same file is a no-op.
	if _, err := st.db.Exec(schema); err != nil {
		t.Fatalf("re-apply schema: %v", err)
	}
}

func TestNextSeqMonotonic(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	var prev int64 = -1
	for i := 0; i < 5; i++ {
		var got int64
		err := st.InTx(ctx, func(tx *sql.Tx) error {
			s, err := st.NextSeq(ctx, tx)
			if err != nil {
				return err
			}
			got = s
			return nil
		})
		if err != nil {
			t.Fatalf("next seq: %v", err)
		}
		if got <= prev {
			t.Errorf("seq not monotonic: %d after %d", got, prev)
		}
		prev = got
	}
}

func TestInsertChartAndRules(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cs := NewChartStore()
	c := domain.Chart{
		ChartID: "chr-000001", Name: "n", Characteristic: "c", ChartType: domain.ChartIndividuals,
		SubgroupSize: 1, CreatedSeq: 0,
	}
	err := st.InTx(ctx, func(tx *sql.Tx) error {
		return cs.InsertChart(ctx, tx, c)
	})
	if err != nil {
		t.Fatalf("insert chart: %v", err)
	}
	rules, err := cs.ListRules(ctx, st, "chr-000001")
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != domain.AllRules {
		t.Errorf("rules %d != %d", len(rules), domain.AllRules)
	}
}

func TestGetMissingChartReturnsNotFound(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cs := NewChartStore()
	_, err := cs.GetChart(ctx, st, "chr-999999")
	if err == nil {
		t.Fatal("expected error for missing chart")
	}
}

func TestMeasurementInsertAndList(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cs := NewChartStore()
	ms := NewMeasurementStore()
	c := domain.Chart{ChartID: "chr-000001", ChartType: domain.ChartIndividuals, SubgroupSize: 1, CreatedSeq: 0}
	if err := st.InTx(ctx, func(tx *sql.Tx) error { return cs.InsertChart(ctx, tx, c) }); err != nil {
		t.Fatal(err)
	}
	m := domain.Measurement{
		MeasurementID: "mea-000001", ChartID: "chr-000001", SubgroupSeq: 1,
		Values: []float64{10}, ValueAvg: 10, CreatedSeq: 1, OccurredSeq: 1,
	}
	if err := st.InTx(ctx, func(tx *sql.Tx) error { return ms.InsertMeasurement(ctx, tx, m) }); err != nil {
		t.Fatalf("insert measurement: %v", err)
	}
	got, err := ms.ListMeasurementsByChart(ctx, st, "chr-000001", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ValueAvg != 10 {
		t.Errorf("list got %+v", got)
	}
}

func TestCount(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cs := NewChartStore()
	c := domain.Chart{ChartID: "chr-000001", ChartType: domain.ChartIndividuals, SubgroupSize: 1, CreatedSeq: 0}
	if err := st.InTx(ctx, func(tx *sql.Tx) error { return cs.InsertChart(ctx, tx, c) }); err != nil {
		t.Fatal(err)
	}
	n, err := st.Count(ctx, `SELECT COUNT(*) FROM charts WHERE archived=0`)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("count %d != 1", n)
	}
}
