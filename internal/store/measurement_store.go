package store

import (
	"context"
	"database/sql"
	"fmt"

	"task120-spc/internal/domain"
)

// MeasurementStore handles persistence of measurement rows.
type MeasurementStore struct{}

// NewMeasurementStore returns a MeasurementStore.
func NewMeasurementStore() *MeasurementStore { return &MeasurementStore{} }

// NextSubgroupSeq returns the next subgroup sequence number for a chart
// (existing max + 1, or 1 if none). subgroup_seq must be strictly increasing
// within a chart; assigning max+1 at insert time preserves submission order
// even across restarts because the rows themselves carry the seq.
//
// q is the query surface: pass the Store (or s.querier()) outside a tx, or the
// *sql.Tx inside one. With SetMaxOpenConns(1) the caller must NOT pass the bare
// store from inside a transaction (the pooled connection is held by the tx and
// the read would deadlock).
func (MeasurementStore) NextSubgroupSeq(ctx context.Context, q DBTX, chartID string) (int, error) {
	var maxSeq sql.NullInt64
	err := q.QueryRowContext(ctx,
		`SELECT MAX(subgroup_seq) FROM measurements`, chartID).Scan(&maxSeq)
	if err != nil {
		if err == sql.ErrNoRows {
			return 1, nil
		}
		return 0, fmt.Errorf("max subgroup_seq: %w", err)
	}
	if !maxSeq.Valid {
		return 1, nil
	}
	return int(maxSeq.Int64) + 1, nil
}

// InsertMeasurement persists a measurement row inside tx. The caller computes
// value_avg and (for xbar_r) range_value before insert; individuals MR is
// back-filled by the recompute path because it depends on the prior baseline
// point.
func (MeasurementStore) InsertMeasurement(ctx context.Context, tx DBTX, m domain.Measurement) error {
	vj, err := encodeValues(m.Values)
	if err != nil {
		return fmt.Errorf("encode values: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO measurements(measurement_id,chart_id,subgroup_seq,values_json,value_avg,range_value,defectives,n_observed,occurred_seq,excluded,created_seq)
VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		m.MeasurementID, m.ChartID, m.SubgroupSeq, vj, m.ValueAvg, m.RangeValue,
		m.Defectives, m.NObserved, m.OccurredSeq, boolToInt(m.Excluded), m.CreatedSeq)
	if err != nil {
		return fmt.Errorf("insert measurement: %w", err)
	}
	return nil
}

// ListMeasurementsByChart returns all measurements for a chart ordered by
// subgroup_seq. excludeExcluded=true filters out soft-deleted points.
func (MeasurementStore) ListMeasurementsByChart(ctx context.Context, q DBTX, chartID string, excludeExcluded bool) ([]domain.Measurement, error) {
	query := `SELECT measurement_id,chart_id,subgroup_seq,values_json,value_avg,range_value,defectives,n_observed,occurred_seq,excluded,created_seq
FROM measurements WHERE chart_id=?`
	if excludeExcluded {
		query += ` AND excluded=0`
	}
	query += ` ORDER BY subgroup_seq ASC`
	rows, err := q.QueryContext(ctx, query, chartID)
	if err != nil {
		return nil, fmt.Errorf("list measurements: %w", err)
	}
	defer rows.Close()
	var out []domain.Measurement
	for rows.Next() {
		m, err := scanMeasurement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMeasurement loads one measurement by id.
func (MeasurementStore) GetMeasurement(ctx context.Context, q DBTX, id string) (domain.Measurement, error) {
	row := q.QueryRowContext(ctx, `
SELECT measurement_id,chart_id,subgroup_seq,values_json,value_avg,range_value,defectives,n_observed,occurred_seq,excluded,created_seq
FROM measurements WHERE measurement_id=?`, id)
	return scanMeasurement(row)
}

// SetExcluded flips the excluded flag of a measurement inside tx. Idempotent:
// setting to the current value is a no-op success.
func (MeasurementStore) SetExcluded(ctx context.Context, tx DBTX, id string, excluded bool) error {
	_, err := tx.ExecContext(ctx, `UPDATE measurements SET excluded=? WHERE measurement_id=?`,
		boolToInt(excluded), id)
	if err != nil {
		return fmt.Errorf("set excluded: %w", err)
	}
	return nil
}

// UpdateRange back-fills the moving-range / subgroup-range column after a
// recompute, inside tx.
func (MeasurementStore) UpdateRange(ctx context.Context, tx DBTX, id string, rng float64) error {
	_, err := tx.ExecContext(ctx, `UPDATE measurements SET range_value=? WHERE measurement_id=?`, rng, id)
	if err != nil {
		return fmt.Errorf("update range: %w", err)
	}
	return nil
}

// DeleteMeasurementsByChart hard-deletes all measurements of a chart (used
// when an archived chart is purged). Not called by the normal flow.
func (MeasurementStore) DeleteMeasurementsByChart(ctx context.Context, tx DBTX, chartID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM measurements WHERE chart_id=?`, chartID)
	if err != nil {
		return fmt.Errorf("delete measurements: %w", err)
	}
	return nil
}

func scanMeasurement(sc scanner) (domain.Measurement, error) {
	var m domain.Measurement
	var valuesJSON string
	var excluded int
	if err := sc.Scan(&m.MeasurementID, &m.ChartID, &m.SubgroupSeq, &valuesJSON, &m.ValueAvg,
		&m.RangeValue, &m.Defectives, &m.NObserved, &m.OccurredSeq, &excluded, &m.CreatedSeq); err != nil {
		if err == sql.ErrNoRows {
			return domain.Measurement{}, fmt.Errorf("measurement: %w", domain.ErrNotFound)
		}
		return domain.Measurement{}, fmt.Errorf("scan measurement: %w", err)
	}
	m.Excluded = excluded == 1
	xs, err := decodeValues(valuesJSON)
	if err != nil {
		return domain.Measurement{}, fmt.Errorf("decode values: %w", err)
	}
	m.Values = xs
	return m, nil
}
