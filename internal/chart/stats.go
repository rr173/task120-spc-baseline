package chart

import (
	"context"
	"fmt"
)

// Stats reports engine-wide counts for the GET /stats endpoint. Every chart's
// measurement/violation totals are aggregated at query time so the numbers are
// always live, never a stale cache.
func (s *Service) Stats(ctx context.Context) (map[string]any, error) {
	chartCount, err := s.st.Count(ctx, `SELECT COUNT(*) FROM charts WHERE archived=0`)
	if err != nil {
		return nil, fmt.Errorf("count charts: %w", err)
	}
	measCount, err := s.st.Count(ctx, `SELECT COUNT(*) FROM measurements`)
	if err != nil {
		return nil, fmt.Errorf("count measurements: %w", err)
	}
	excludedCount, err := s.st.Count(ctx, `SELECT COUNT(*) FROM measurements WHERE excluded=1`)
	if err != nil {
		return nil, fmt.Errorf("count excluded: %w", err)
	}
	violCount, err := s.st.Count(ctx, `SELECT COUNT(*) FROM violations`)
	if err != nil {
		return nil, fmt.Errorf("count violations: %w", err)
	}
	rejectCount, err := s.st.Count(ctx, `SELECT COUNT(*) FROM violations WHERE severity='reject'`)
	if err != nil {
		return nil, fmt.Errorf("count rejects: %w", err)
	}
	return map[string]any{
		"charts":                chartCount,
		"measurements":          measCount,
		"excluded_measurements": excludedCount,
		"violations":            violCount,
		"reject_violations":     rejectCount,
	}, nil
}
