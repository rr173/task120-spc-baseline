package store

import (
	"context"
	"database/sql"
	"fmt"

	"task120-spc/internal/domain"
)

// ChartStore handles persistence of charts and per-chart rule configs.
type ChartStore struct{}

// NewChartStore returns a ChartStore. Stateless; kept as a struct so the API
// mirrors the other stores and future caches can attach cleanly.
func NewChartStore() *ChartStore { return &ChartStore{} }

// InsertChart persists a chart row inside tx.
func (ChartStore) InsertChart(ctx context.Context, tx DBTX, c domain.Chart) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO charts(chart_id,name,characteristic,chart_type,unit,subgroup_size,usl,lsl,target,created_seq,archived)
VALUES(?,?,?,?,?,?,?,?,?,?,0)`,
		c.ChartID, c.Name, c.Characteristic, string(c.ChartType), c.Unit, c.SubgroupSize,
		nullableFloat(c.USL), nullableFloat(c.LSL), nullableFloat(c.Target), c.CreatedSeq)
	if err != nil {
		return fmt.Errorf("insert chart: %w", err)
	}
	// Seed default rule config: every Westgard rule enabled.
	for _, r := range domain.WestgardRules {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rule_config(chart_id,rule,enabled) VALUES(?,?,1)`,
			c.ChartID, string(r)); err != nil {
			return fmt.Errorf("seed rule_config %s: %w", r, err)
		}
	}
	return nil
}

// GetChart loads one chart by id.
func (ChartStore) GetChart(ctx context.Context, q DBTX, id string) (domain.Chart, error) {
	row := q.QueryRowContext(ctx, `
SELECT chart_id,name,characteristic,chart_type,unit,subgroup_size,usl,lsl,target,created_seq,archived
FROM charts WHERE chart_id=?`, id)
	return scanChart(row)
}

// ListCharts returns all non-archived charts ordered by creation sequence.
func (ChartStore) ListCharts(ctx context.Context, q DBTX, includeArchived bool) ([]domain.Chart, error) {
	query := `SELECT chart_id,name,characteristic,chart_type,unit,subgroup_size,usl,lsl,target,created_seq,archived
FROM charts`
	if !includeArchived {
		query += ` WHERE archived=0`
	}
	query += ` ORDER BY created_seq ASC`
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list charts: %w", err)
	}
	defer rows.Close()
	var out []domain.Chart
	for rows.Next() {
		c, err := scanChart(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateChart applies the mutable fields (spec limits, target, archived, name,
// characteristic). Nil pointers leave the field unchanged.
func (ChartStore) UpdateChart(ctx context.Context, tx DBTX, c domain.Chart) error {
	_, err := tx.ExecContext(ctx, `
UPDATE charts SET name=?,characteristic=?,usl=?,lsl=?,target=?,archived=? WHERE chart_id=?`,
		c.Name, c.Characteristic, nullableFloat(c.USL), nullableFloat(c.LSL), nullableFloat(c.Target), boolToInt(c.Archived), c.ChartID)
	if err != nil {
		return fmt.Errorf("update chart: %w", err)
	}
	return nil
}

// DeleteChart marks a chart archived (soft delete) inside tx.
func (ChartStore) DeleteChart(ctx context.Context, tx DBTX, id string) error {
	_, err := tx.ExecContext(ctx, `UPDATE charts SET archived=1 WHERE chart_id=?`, id)
	if err != nil {
		return fmt.Errorf("archive chart: %w", err)
	}
	return nil
}

// SetRule toggles one Westgard rule for a chart inside tx.
func (ChartStore) SetRule(ctx context.Context, tx DBTX, chartID string, rule domain.RuleName, enabled bool) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO rule_config(chart_id,rule,enabled) VALUES(?,?,?)
		 ON CONFLICT(chart_id,rule) DO UPDATE SET enabled=excluded.enabled`,
		chartID, string(rule), boolToInt(enabled))
	if err != nil {
		return fmt.Errorf("set rule %s: %w", rule, err)
	}
	return nil
}

// ListRules returns the per-chart rule enable map (defaulting absent rows to
// enabled, which never happens because InsertChart seeds all six).
func (ChartStore) ListRules(ctx context.Context, q DBTX, chartID string) (map[domain.RuleName]bool, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT rule,enabled FROM rule_config WHERE chart_id=?`, chartID)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	m := make(map[domain.RuleName]bool, domain.AllRules)
	for _, r := range domain.WestgardRules {
		m[r] = true
	}
	for rows.Next() {
		var r string
		var en int
		if err := rows.Scan(&r, &en); err != nil {
			return nil, err
		}
		// enabled is stored 1 (true) / 0 (false) via boolToInt, so a row's
		// value is enabled iff en==1. Absent rows keep the default above.
		m[domain.RuleName(r)] = en == 1
	}
	return m, rows.Err()
}

// scanner abstracts *sql.Row and *sql.Rows for scanChart.
type scanner interface {
	Scan(dest ...any) error
}

func scanChart(sc scanner) (domain.Chart, error) {
	var c domain.Chart
	var chartType string
	var usl, lsl, target sql.NullFloat64
	var archived int
	if err := sc.Scan(&c.ChartID, &c.Name, &c.Characteristic, &chartType, &c.Unit, &c.SubgroupSize,
		&usl, &lsl, &target, &c.CreatedSeq, &archived); err != nil {
		if err == sql.ErrNoRows {
			return domain.Chart{}, fmt.Errorf("chart %s: %w", c.ChartID, domain.ErrNotFound)
		}
		return domain.Chart{}, fmt.Errorf("scan chart: %w", err)
	}
	c.ChartType = domain.ChartType(chartType)
	c.Archived = archived == 1
	if usl.Valid {
		v := usl.Float64
		c.USL = &v
	}
	if lsl.Valid {
		v := lsl.Float64
		c.LSL = &v
	}
	if target.Valid {
		v := target.Float64
		c.Target = &v
	}
	return c, nil
}

// nullableFloat converts a *float64 to a sql-driver-compatible interface.
func nullableFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// boolToInt returns 1 for true, 0 for false.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// encodeValues marshals a subgroup's values for storage.
func encodeValues(xs []float64) (string, error) {
	b, err := jsonMarshal(xs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeValues unmarshals a subgroup's values.
func decodeValues(s string) ([]float64, error) {
	var xs []float64
	if err := jsonUnmarshal([]byte(s), &xs); err != nil {
		return nil, err
	}
	return xs, nil
}
