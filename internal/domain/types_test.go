package domain

import "testing"

func TestValidChartType(t *testing.T) {
	valid := []ChartType{ChartIndividuals, ChartXbarR, ChartP}
	for _, v := range valid {
		if !ValidChartType(v) {
			t.Errorf("%q should be valid", v)
		}
	}
	if ValidChartType("bogus") {
		t.Error("bogus should be invalid")
	}
}

func TestSeverityOf(t *testing.T) {
	if SeverityOf(Rule12s) != SeverityWarning {
		t.Error("1-2s should be warning")
	}
	for _, r := range []RuleName{Rule13s, Rule22s, RuleR4s, Rule41s, Rule10x} {
		if SeverityOf(r) != SeverityReject {
			t.Errorf("%s should be reject", r)
		}
	}
}

func TestIDFor(t *testing.T) {
	if got := ChartID(1); got != "chr-000001" {
		t.Errorf("ChartID(1)=%q want chr-000001", got)
	}
	if got := MeasurementID(42); got != "mea-000042" {
		t.Errorf("MeasurementID(42)=%q want mea-000042", got)
	}
	if got := ViolationID(7); got != "vio-000007" {
		t.Errorf("ViolationID(7)=%q want vio-000007", got)
	}
}

func TestHasSpec(t *testing.T) {
	usl := 10.0
	c := Chart{USL: &usl}
	if !c.HasSpec() || c.HasTwoSidedSpec() {
		t.Error("one-sided spec logic")
	}
	lsl := 0.0
	c.LSL = &lsl
	if !c.HasTwoSidedSpec() {
		t.Error("two-sided should be true")
	}
}
