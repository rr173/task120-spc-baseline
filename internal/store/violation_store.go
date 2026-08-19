package store

import (
	"context"
	"database/sql"
	"fmt"

	"task120-spc/internal/domain"
)

// ViolationStore handles persistence of Westgard rule violations and the
// recomputed control-limit cache.
type ViolationStore struct{}

// NewViolationStore returns a ViolationStore.
func NewViolationStore() *ViolationStore { return &ViolationStore{} }

// InsertViolation persists one violation row inside tx.
func (ViolationStore) InsertViolation(ctx context.Context, tx DBTX, v domain.Violation) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO violations(violation_id,chart_id,measurement_seq,rule,severity,detected_seq)
VALUES(?,?,?,?,?,?)`,
		v.ViolationID, v.ChartID, v.MeasurementSeq, string(v.Rule), string(v.Severity), v.DetectedSeq)
	if err != nil {
		return fmt.Errorf("insert violation: %w", err)
	}
	return nil
}

// DeleteViolationsByChart removes all violations for a chart inside tx. Called
// before re-running Westgard so the stored set is exactly the recomputed set.
func (ViolationStore) DeleteViolationsByChart(ctx context.Context, tx DBTX, chartID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM violations WHERE chart_id=?`, chartID)
	if err != nil {
		return fmt.Errorf("delete violations: %w", err)
	}
	return nil
}

// ListViolationsByChart returns all violations for a chart ordered by the
// triggering point's subgroup_seq, then by detected_seq for determinism.
func (ViolationStore) ListViolationsByChart(ctx context.Context, q DBTX, chartID string) ([]domain.Violation, error) {
	rows, err := q.QueryContext(ctx, `
SELECT violation_id,chart_id,measurement_seq,rule,severity,detected_seq
FROM violations WHERE chart_id=? ORDER BY measurement_seq ASC, detected_seq ASC`, chartID)
	if err != nil {
		return nil, fmt.Errorf("list violations: %w", err)
	}
	defer rows.Close()
	var out []domain.Violation
	for rows.Next() {
		var v domain.Violation
		var rule, sev string
		if err := rows.Scan(&v.ViolationID, &v.ChartID, &v.MeasurementSeq, &rule, &sev, &v.DetectedSeq); err != nil {
			return nil, err
		}
		v.Rule = domain.RuleName(rule)
		v.Severity = domain.Severity(sev)
		out = append(out, v)
	}
	return out, rows.Err()
}

// PutLimit upserts the recomputed control-limit cache for a chart inside tx.
func (ViolationStore) PutLimit(ctx context.Context, tx DBTX, c domain.ControlLimit) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO control_limits(chart_id,baseline_count,cl,ucl,lcl,sigma_within,sigma_overall,computed_seq)
VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(chart_id) DO UPDATE SET
	baseline_count=excluded.baseline_count,
	cl=excluded.cl,
	ucl=excluded.ucl,
	lcl=excluded.lcl,
	sigma_within=excluded.sigma_within,
	sigma_overall=excluded.sigma_overall,
	computed_seq=excluded.computed_seq`,
		c.ChartID, c.BaselineCount, c.CL, c.UCL, c.LCL, c.SigmaWithin, c.SigmaOverall, c.ComputedSeq)
	if err != nil {
		return fmt.Errorf("put limit: %w", err)
	}
	return nil
}

// GetLimit loads the cached control limit for a chart. Returns ErrNotFound when
// no limit has been computed yet (e.g. zero baseline points).
func (ViolationStore) GetLimit(ctx context.Context, q DBTX, chartID string) (domain.ControlLimit, error) {
	var cl domain.ControlLimit
	err := q.QueryRowContext(ctx, `
SELECT chart_id,baseline_count,cl,ucl,lcl,sigma_within,sigma_overall,computed_seq
FROM control_limits WHERE chart_id=?`, chartID).Scan(
		&cl.ChartID, &cl.BaselineCount, &cl.CL, &cl.UCL, &cl.LCL, &cl.SigmaWithin, &cl.SigmaOverall, &cl.ComputedSeq)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ControlLimit{}, fmt.Errorf("limit %s: %w", chartID, domain.ErrNotFound)
		}
		return domain.ControlLimit{}, fmt.Errorf("get limit: %w", err)
	}
	return cl, nil
}

// DeleteLimit removes the cached limit for a chart inside tx.
func (ViolationStore) DeleteLimit(ctx context.Context, tx DBTX, chartID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM control_limits WHERE chart_id=?`, chartID)
	if err != nil {
		return fmt.Errorf("delete limit: %w", err)
	}
	return nil
}
