// Package domain holds the cross-cutting types, identifier schemes and sentinel
// errors for the SPC engine. It is imported by every business package so the
// store, services and HTTP layers share one vocabulary. Nothing here touches a
// database or the network.
package domain

// ChartType enumerates the supported SPC chart kinds.
type ChartType string

const (
	// ChartIndividuals is the single-value / moving-range (I-MR) chart: one
	// measured value per subgroup, within-subgroup variation estimated from the
	// moving range of consecutive points (d2 at n=2).
	ChartIndividuals ChartType = "individuals"
	// ChartXbarR is the mean / range chart: each subgroup has a fixed size n>=2;
	// the controlled quantity is the subgroup mean and within-subgroup sigma
	// is estimated from the average subgroup range via d2(n).
	ChartXbarR ChartType = "xbar_r"
	// ChartP is the fraction-defective (attribute) chart: each subgroup records
	// defectives out of n_observed items; the controlled quantity is p=d/n.
	ChartP ChartType = "p_chart"
)

// ValidChartType reports whether t is one of the supported chart kinds.
func ValidChartType(t ChartType) bool {
	switch t {
	case ChartIndividuals, ChartXbarR, ChartP:
		return true
	}
	return false
}

// Severity ranks a Westgard rule violation. "warning" means the point is out
// of the 2-sigma zone but the run is not yet rejected; "reject" means the run
// is out of statistical control and must be investigated.
type Severity string

const (
	SeverityWarning Severity = "warning"
	SeverityReject  Severity = "reject"
)

// RuleName enumerates the six Westgard multi-rules the engine evaluates.
type RuleName string

const (
	Rule12s  RuleName = "1-2s"
	Rule13s  RuleName = "1-3s"
	Rule22s  RuleName = "2-2s"
	RuleR4s  RuleName = "R-4s"
	Rule41s  RuleName = "4-1s"
	Rule10x  RuleName = "10-x"
	AllRules         = 6
)

// WestgardRules is the fixed evaluation order; a measurement may trip several
// rules at once and each hit is recorded.
var WestgardRules = []RuleName{Rule12s, Rule13s, Rule22s, RuleR4s, Rule41s, Rule10x}

// SeverityOf returns the severity for a rule. Only 1-2s is a warning; the rest
// reject the run.
func SeverityOf(r RuleName) Severity {
	if r == Rule12s {
		return SeverityWarning
	}
	return SeverityReject
}

// RuleMinPoints is the minimum number of in-control baseline points required
// for a rule to be able to fire. Rules that need more history than is present
// simply do not fire (never an error).
var RuleMinPoints = map[RuleName]int{
	Rule12s: 1,
	Rule13s: 1,
	Rule22s: 2,
	RuleR4s: 2,
	Rule41s: 4,
	Rule10x: 10,
}
