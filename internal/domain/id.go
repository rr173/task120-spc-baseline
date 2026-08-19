package domain

import "fmt"

// IDFor returns a human-readable, monotonic identifier for an entity kind. The
// seq is stamped from the store's global next_seq counter inside the write
// transaction so identifiers reflect true insertion order across restarts. The
// prefix makes log lines and URLs self-describing (chr-/mea-/vio-).
func IDFor(kind string, seq int64) string {
	return fmt.Sprintf("%s-%06d", kind, seq)
}

// ChartID formats a chart identifier from a sequence number.
func ChartID(seq int64) string { return IDFor("chr", seq) }

// MeasurementID formats a measurement identifier from a sequence number.
func MeasurementID(seq int64) string { return IDFor("mea", seq) }

// ViolationID formats a violation identifier from a sequence number.
func ViolationID(seq int64) string { return IDFor("vio", seq) }

// Chart, Measurement and Violation are the core domain records. They mirror the
// persisted rows but stay free of database concerns (no *sql.Tx, no JSON tags
// for storage). The httpapi layer attaches JSON tags when encoding responses.

// Chart is a statistical process control chart for one quality characteristic.
type Chart struct {
	ChartID       string
	Name          string
	Characteristic string
	ChartType     ChartType
	Unit          string
	SubgroupSize  int    // n; meaningful for xbar_r and p_chart
	USL           *float64 // upper spec limit; nil = absent (one-sided)
	LSL           *float64 // lower spec limit; nil = absent (one-sided)
	Target        *float64 // target value; only required for Cpm
	CreatedSeq    int64
	Archived      bool
}

// HasSpec reports whether at least one spec limit is set (capability needs this).
func (c Chart) HasSpec() bool { return c.USL != nil || c.LSL != nil }

// HasTwoSidedSpec reports whether both spec limits are set (Cp/Pp/Cpm need
// this).
func (c Chart) HasTwoSidedSpec() bool { return c.USL != nil && c.LSL != nil }

// Measurement is one recorded subgroup of a chart.
type Measurement struct {
	MeasurementID string
	ChartID       string
	SubgroupSeq   int      // strictly increasing within a chart; the time clock
	Values        []float64 // xbar_r: the n subgroup values; individuals: single; p_chart: empty
	ValueAvg      float64  // controlled quantity: individuals=value; xbar=mean; p_chart=p=d/n
	RangeValue    float64  // xbar_r=subgroup range; individuals=MR (filled on recompute); p_chart=0
	Defectives    int      // p_chart only
	NObserved     int      // p_chart only
	OccurredSeq   int64    // = created_seq, used for display ordering
	Excluded      bool
	CreatedSeq    int64
}

// Violation is one Westgard rule violation detected at a measurement.
type Violation struct {
	ViolationID   string
	ChartID       string
	MeasurementSeq int     // subgroup_seq of the triggering point
	Rule          RuleName
	Severity      Severity
	DetectedSeq   int64
}

// RuleConfig is the per-chart enable/disable flag for one Westgard rule.
type RuleConfig struct {
	ChartID string
	Rule    RuleName
	Enabled bool
}

// ControlLimit is the recomputed chart limit cache. It is NOT the source of
// truth: the consistency check recomputes from the non-excluded measurements
// and asserts equality with this row.
type ControlLimit struct {
	ChartID       string
	BaselineCount int     // number of non-excluded measurements used
	CL            float64 // center line (mean of controlled quantity)
	UCL           float64 // upper control limit
	LCL           float64 // lower control limit
	SigmaWithin   float64 // within-subgroup sigma (Rbar/d2 or MRbar/d2)
	SigmaOverall  float64 // overall sigma (Bessel over all individual values)
	ComputedSeq    int64
}

// Capability is the process-capability report for a chart.
type Capability struct {
	Estimable bool
	// Two-sided indices; not_estimable when only one spec present.
	Cp  *float64
	Pp  *float64
	Cpm *float64
	// One-sided / always-present (when estimable & spec on that side).
	CPU  *float64
	CPL  *float64
	Cpk  *float64
	PPU  *float64
	PPL  *float64
	Ppk  *float64
	DPMO *float64
	Yield *float64
	SigmaLevel *float64
}
