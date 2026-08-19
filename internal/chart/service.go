// Package chart orchestrates the SPC business logic: chart lifecycle, adding
// measurements, recomputing control limits and Westgard violations, and
// excluding/restoring points. It is the only place that knows how a chart
// type maps to a statistics pipeline, so the store stays a thin CRUD layer and
// the HTTP layer stays a thin adapter.
//
// The recomputation path is the engine's spine: after every mutation (add,
// exclude, restore, spec change) the service reloads the non-excluded baseline
// of the chart, recomputes the center line + within/overall sigma, stores the
// control_limits cache, wipes and re-runs Westgard over the new baseline, and
// re-stores the violation set. Control limits and violations are therefore a
// pure function of the persisted measurements — which is exactly what the
// restart-recovery and consistency checks assert.
package chart

import (
	"context"
	"database/sql"
	"fmt"

	"task120-spc/internal/capability"
	"task120-spc/internal/domain"
	"task120-spc/internal/stats"
	"task120-spc/internal/store"
)

// Service ties the stores together and exposes business operations. All
// mutating operations run inside a single IMMEDIATE transaction so an
// invariant failure rolls back the measurement, the limit cache and the
// violations together.
type Service struct {
	st *store.Store
	ch *store.ChartStore
	me *store.MeasurementStore
	vi *store.ViolationStore
}

// New constructs a Service over the given store.
func New(st *store.Store) *Service {
	return &Service{
		st: st,
		ch: store.NewChartStore(),
		me: store.NewMeasurementStore(),
		vi: store.NewViolationStore(),
	}
}

// CreateChart validates and persists a new chart, returning the id. subgroup
// size defaults to 1 for individuals and must be in [2,10] for xbar_r; p_chart
// needs n in [1,] (we accept >=1). At least one spec limit must be given
// eventually, but a chart may start without one and gain it later via Update
// (capability stays not_estimable meanwhile).
func (s *Service) CreateChart(ctx context.Context, name, characteristic string, ct domain.ChartType, unit string, n int, usl, lsl, target *float64) (domain.Chart, error) {
	if !domain.ValidChartType(ct) {
		return domain.Chart{}, fmt.Errorf("%w: bad chart_type %q", domain.ErrInvalid, ct)
	}
	if name == "" || characteristic == "" {
		return domain.Chart{}, fmt.Errorf("%w: name and characteristic required", domain.ErrInvalid)
	}
	// Subgroup size rules per chart type.
	switch ct {
	case domain.ChartIndividuals:
		n = 1 // individuals is single-value; ignore caller's n
	case domain.ChartXbarR:
		if n < 2 || n > 10 {
			return domain.Chart{}, fmt.Errorf("%w: xbar_r subgroup_size must be in [2,10]", domain.ErrInvalid)
		}
	case domain.ChartP:
		if n < 1 {
			return domain.Chart{}, fmt.Errorf("%w: p_chart subgroup_size must be >= 1", domain.ErrInvalid)
		}
	}
	c := domain.Chart{
		Name: name, Characteristic: characteristic, ChartType: ct, Unit: unit,
		SubgroupSize: n, USL: usl, LSL: lsl, Target: target,
	}
	err := s.st.InTx(ctx, func(tx *sql.Tx) error {
		seq, err := s.st.NextSeq(ctx, tx)
		if err != nil {
			return err
		}
		c.ChartID = domain.ChartID(seq)
		c.CreatedSeq = seq
		return s.ch.InsertChart(ctx, tx, c)
	})
	if err != nil {
		return domain.Chart{}, err
	}
	return c, nil
}

// GetChart returns a chart by id.
func (s *Service) GetChart(ctx context.Context, id string) (domain.Chart, error) {
	return s.ch.GetChart(ctx, s.st, id)
}

// ListCharts returns all charts.
func (s *Service) ListCharts(ctx context.Context, includeArchived bool) ([]domain.Chart, error) {
	return s.ch.ListCharts(ctx, s.st, includeArchived)
}

// UpdateChart applies mutable fields. A nil pointer in the request leaves the
// corresponding spec field unchanged; an explicit value sets it. Archiving is
// terminal: an archived chart rejects further mutations.
func (s *Service) UpdateChart(ctx context.Context, id string, name, characteristic *string, usl, lsl, target **float64, archived *bool) (domain.Chart, error) {
	c, err := s.ch.GetChart(ctx, s.st, id)
	if err != nil {
		return domain.Chart{}, err
	}
	if c.Archived {
		return domain.Chart{}, fmt.Errorf("%w: chart %s archived", domain.ErrTerminal, id)
	}
	if name != nil {
		c.Name = *name
	}
	if characteristic != nil {
		c.Characteristic = *characteristic
	}
	applySpecPtr(&c.USL, usl)
	applySpecPtr(&c.LSL, lsl)
	applySpecPtr(&c.Target, target)
	if archived != nil {
		c.Archived = *archived
	}
	err = s.st.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.ch.UpdateChart(ctx, tx, c); err != nil {
			return err
		}
		// Spec change can move capability/limits meaningfully; recompute cache.
		return s.recompute(ctx, tx, c.ChartID, c.ChartType, c.SubgroupSize)
	})
	if err != nil {
		return domain.Chart{}, err
	}
	return c, nil
}

// ArchiveChart soft-deletes a chart (sets archived=1). Idempotent.
func (s *Service) ArchiveChart(ctx context.Context, id string) error {
	c, err := s.ch.GetChart(ctx, s.st, id)
	if err != nil {
		return err
	}
	if c.Archived {
		return nil
	}
	return s.st.InTx(ctx, func(tx *sql.Tx) error {
		return s.ch.DeleteChart(ctx, tx, id)
	})
}

// AddMeasurement validates and records one measurement subgroup, then
// recomputes limits + violations in the same transaction.
func (s *Service) AddMeasurement(ctx context.Context, chartID string, values []float64, defectives, nObserved int) (domain.Measurement, error) {
	c, err := s.ch.GetChart(ctx, s.st, chartID)
	if err != nil {
		return domain.Measurement{}, err
	}
	if c.Archived {
		return domain.Measurement{}, fmt.Errorf("%w: chart %s archived", domain.ErrTerminal, chartID)
	}
	m, err := s.buildMeasurement(ctx, c, values, defectives, nObserved)
	if err != nil {
		return domain.Measurement{}, err
	}
	// Compute the next subgroup sequence BEFORE opening the write transaction:
	// the chart has SetMaxOpenConns(1), so a read via the pooled connection
	// would deadlock waiting for the very connection the IMMEDIATE tx holds.
	nextSeq, err := s.me.NextSubgroupSeq(ctx, s.st, chartID)
	if err != nil {
		return domain.Measurement{}, err
	}
	m.SubgroupSeq = nextSeq
	err = s.st.InTx(ctx, func(tx *sql.Tx) error {
		seq, err := s.st.NextSeq(ctx, tx)
		if err != nil {
			return err
		}
		m.MeasurementID = domain.MeasurementID(seq)
		m.CreatedSeq = seq
		m.OccurredSeq = seq
		if err := s.me.InsertMeasurement(ctx, tx, m); err != nil {
			return err
		}
		return s.recompute(ctx, tx, chartID, c.ChartType, c.SubgroupSize)
	})
	if err != nil {
		return domain.Measurement{}, err
	}
	return m, nil
}

// buildMeasurement validates the input against the chart type and fills the
// controlled quantity (value_avg) and, for xbar_r, the subgroup range.
// individuals MR is left 0 here and back-filled by recompute.
func (s *Service) buildMeasurement(ctx context.Context, c domain.Chart, values []float64, defectives, nObserved int) (domain.Measurement, error) {
	m := domain.Measurement{ChartID: c.ChartID}
	switch c.ChartType {
	case domain.ChartIndividuals:
		if len(values) != 1 {
			return domain.Measurement{}, fmt.Errorf("%w: individuals needs exactly 1 value", domain.ErrInvariant)
		}
		m.Values = values
		m.ValueAvg = values[0]
	case domain.ChartXbarR:
		if len(values) != c.SubgroupSize {
			return domain.Measurement{}, fmt.Errorf("%w: xbar_r subgroup must have %d values, got %d",
				domain.ErrInvariant, c.SubgroupSize, len(values))
		}
		m.Values = values
		m.ValueAvg = stats.Mean(values)
		m.RangeValue = stats.Range(values)
	case domain.ChartP:
		if nObserved < 1 {
			return domain.Measurement{}, fmt.Errorf("%w: p_chart n_observed must be >= 1", domain.ErrInvariant)
		}
		if defectives < 0 || defectives > nObserved {
			return domain.Measurement{}, fmt.Errorf("%w: defectives %d out of [0,%d]", domain.ErrInvariant, defectives, nObserved)
		}
		m.NObserved = nObserved
		m.Defectives = defectives
		m.ValueAvg = float64(defectives) / float64(nObserved)
	}
	return m, nil
}

// ListMeasurements returns the measurements of a chart in subgroup order.
func (s *Service) ListMeasurements(ctx context.Context, chartID string, excludeExcluded bool) ([]domain.Measurement, error) {
	if _, err := s.ch.GetChart(ctx, s.st, chartID); err != nil {
		return nil, err
	}
	return s.me.ListMeasurementsByChart(ctx, s.st, chartID, excludeExcluded)
}

// GetMeasurement returns one measurement.
func (s *Service) GetMeasurement(ctx context.Context, id string) (domain.Measurement, error) {
	return s.me.GetMeasurement(ctx, s.st, id)
}

// ExcludeMeasurement soft-deletes a measurement from the baseline. Idempotent.
// Excluding recomputes limits + violations without the point.
func (s *Service) ExcludeMeasurement(ctx context.Context, id string) error {
	return s.setExcluded(ctx, id, true)
}

// RestoreMeasurement re-includes an excluded measurement. Idempotent.
func (s *Service) RestoreMeasurement(ctx context.Context, id string) error {
	return s.setExcluded(ctx, id, false)
}

func (s *Service) setExcluded(ctx context.Context, id string, excluded bool) error {
	m, err := s.me.GetMeasurement(ctx, s.st, id)
	if err != nil {
		return err
	}
	c, err := s.ch.GetChart(ctx, s.st, m.ChartID)
	if err != nil {
		return err
	}
	if c.Archived {
		return fmt.Errorf("%w: chart %s archived", domain.ErrTerminal, m.ChartID)
	}
	return s.st.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.me.SetExcluded(ctx, tx, id, excluded); err != nil {
			return err
		}
		return nil
	})
}

// GetLimit returns the cached recomputed control limit for a chart.
func (s *Service) GetLimit(ctx context.Context, chartID string) (domain.ControlLimit, error) {
	if _, err := s.ch.GetChart(ctx, s.st, chartID); err != nil {
		return domain.ControlLimit{}, err
	}
	return s.vi.GetLimit(ctx, s.st, chartID)
}

// ListViolations returns the violations stored for a chart.
func (s *Service) ListViolations(ctx context.Context, chartID string) ([]domain.Violation, error) {
	if _, err := s.ch.GetChart(ctx, s.st, chartID); err != nil {
		return nil, err
	}
	return s.vi.ListViolationsByChart(ctx, s.st, chartID)
}

// GetCapability recomputes the capability indices on the fly from the stored
// baseline (it does not need a cache because the indices are cheap and are
// derived from sigma + spec which recompute() already keeps in sync).
func (s *Service) GetCapability(ctx context.Context, chartID string) (domain.Capability, error) {
	c, err := s.ch.GetChart(ctx, s.st, chartID)
	if err != nil {
		return domain.Capability{}, err
	}
	return s.computeCapability(ctx, c), nil
}

// computeCapability rebuilds the input for the capability package from the
// current baseline. It never errors: not-estimable is a normal status.
func (s *Service) computeCapability(ctx context.Context, c domain.Chart) domain.Capability {
	lim, err := s.vi.GetLimit(ctx, s.st, c.ChartID)
	if err != nil {
		return domain.Capability{Estimable: false}
	}
	in := capability.Input{
		USL: c.USL, LSL: c.LSL, Target: c.Target,
		Mean: lim.CL, SigmaWithin: lim.SigmaWithin, SigmaOverall: lim.SigmaOverall,
	}
	return capability.Compute(in)
}

// GetRules returns the per-chart rule enable map.
func (s *Service) GetRules(ctx context.Context, chartID string) (map[domain.RuleName]bool, error) {
	if _, err := s.ch.GetChart(ctx, s.st, chartID); err != nil {
		return nil, err
	}
	return s.ch.ListRules(ctx, s.st, chartID)
}

// SetRules applies a batch of rule toggles and re-runs Westgard so the stored
// violation set reflects the new enable map.
func (s *Service) SetRules(ctx context.Context, chartID string, toggles map[domain.RuleName]bool) error {
	c, err := s.ch.GetChart(ctx, s.st, chartID)
	if err != nil {
		return err
	}
	if c.Archived {
		return fmt.Errorf("%w: chart %s archived", domain.ErrTerminal, chartID)
	}
	return s.st.InTx(ctx, func(tx *sql.Tx) error {
		for rule, en := range toggles {
			if err := s.ch.SetRule(ctx, tx, chartID, rule, en); err != nil {
				return err
			}
		}
		return s.recompute(ctx, tx, chartID, c.ChartType, c.SubgroupSize)
	})
}

// Recompute is the admin entry point: force a fresh recompute of the cache +
// violations from the persisted baseline. Used by the smoke test and the
// consistency check.
func (s *Service) Recompute(ctx context.Context, chartID string) error {
	c, err := s.ch.GetChart(ctx, s.st, chartID)
	if err != nil {
		return err
	}
	return s.st.InTx(ctx, func(tx *sql.Tx) error {
		return s.recompute(ctx, tx, chartID, c.ChartType, c.SubgroupSize)
	})
}

// applySpecPtr applies a **float64 update: nil pointer-to-pointer means "leave
// unchanged", a non-nil pointer-to-nil means "clear", a non-nil pointer-to-value
// means "set".
func applySpecPtr(dst **float64, src **float64) {
	if src == nil {
		return
	}
	*dst = *src
}
