// Package selfcheck runs the --smoke-test for the SPC engine. Each scenario
// builds its own temporary SQLite file and httptest server so contract
// assertions don't trip over state left by an earlier scenario. The restart
// scenario seeds state, closes the store, reopens the same file and asserts
// the recomputed limits and violations are unchanged.
//
// The smoke test never sleeps and never touches the network; it talks to the
// real mux over httptest.NewServer so the HTTP/JSON contract itself is
// exercised end to end, including the embedded frontend route.
package selfcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"task120-spc/internal/chart"
	"task120-spc/internal/httpapi"
	"task120-spc/internal/store"
	"task120-spc/internal/webfs"
)

// adminToken used by the smoke test for the consistency endpoint.
const adminToken = "admin-secret"

// Run executes every smoke scenario. Returns the first failure.
func Run() error {
	dir, err := os.MkdirTemp("", "spc-smoke-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	cases := []struct {
		name string
		fn   func(*httptest.Server, string) error
	}{
		{"individuals-limits-recompute", smokeIndividualsLimits},
		{"xbar-r-subgroup-size-guard", smokeXbarRSubgroupGuard},
		{"p-chart-bounds", smokePChartBounds},
		{"exclude-restore-recompute", smokeExcludeRestore},
		{"westgard-1-3s-triggers", smokeWestgard13s},
		{"westgard-2-2s-triggers", smokeWestgard22s},
		{"westgard-r-4s-triggers", smokeWestgardR4s},
		{"westgard-4-1s-triggers", smokeWestgard41s},
		{"westgard-10-x-triggers", smokeWestgard10x},
		{"rule-toggle-disables", smokeRuleToggle},
		{"capability-estimable-and-not", smokeCapability},
		{"one-sided-spec", smokeOneSidedSpec},
		{"xbar-r-overall-sigma-from-raw-observations", smokeXbarROverallSigma},
		{"consistency-check-ok", smokeConsistency},
		{"frontend-page-served", smokeFrontend},
	}
	for i, c := range cases {
		dbPath := filepath.Join(dir, fmt.Sprintf("smoke-%02d.db", i))
		srv, err := newServer(dbPath)
		if err != nil {
			return fmt.Errorf("%s: new server: %w", c.name, err)
		}
		if err := c.fn(srv, dbPath); err != nil {
			srv.Close()
			return fmt.Errorf("%s: %w", c.name, err)
		}
		srv.Close()
	}

	// Restart-recovery runs separately because it controls the store lifecycle
	// (close + reopen on the same file) itself.
	if err := smokeRestartRecovery(filepath.Join(dir, "smoke-recover.db")); err != nil {
		return fmt.Errorf("restart-recovery: %w", err)
	}
	return nil
}

// newServer opens a fresh SQLite file, builds the service + mux and returns an
// httptest server over the real HTTP handler tree. The store is closed via a
// shutdown hook so callers only need to call srv.Close().
func newServer(dbPath string) (*httptest.Server, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	svc := httpapi.Services{Chart: chart.New(st)}
	webFS := http.FS(webfs.FS())
	mux := httpapi.NewMux(svc, adminToken, webFS)
	srv := httptest.NewServer(mux)
	srv.Config.RegisterOnShutdown(func() { _ = st.Close() })
	return srv, nil
}

// --- HTTP helpers ---

func doJSON(srv *httptest.Server, method, path string, body any, admin bool) (int, []byte, error) {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(buf))
		r.Header.Set("Content-Type", "application/json")
	}
	if admin {
		r.Header.Set(httpapi.AdminTokenHeader, adminToken)
	}
	rec := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(rec, r)
	return rec.Code, rec.Body.Bytes(), nil
}

// mustDo performs an HTTP call and returns the decoded body; it fails the
// scenario immediately on a non-2xx status (per the selfcheck-helper-error
// convention: success-expecting helpers must surface non-200s, not swallow
// them).
func mustDo(srv *httptest.Server, method, path string, body any, admin bool, out any) error {
	code, respBody, err := doJSON(srv, method, path, body, admin)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("%s %s: expected 2xx, got %d: %s", method, path, code, respBody)
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

// expectStatus asserts the call returns the exact status code; it returns the
// response body for further inspection.
func expectStatus(srv *httptest.Server, method, path string, body any, admin bool, want int) ([]byte, error) {
	code, respBody, err := doJSON(srv, method, path, body, admin)
	if err != nil {
		return nil, err
	}
	if code != want {
		return respBody, fmt.Errorf("%s %s: expected %d, got %d: %s", method, path, want, code, respBody)
	}
	return respBody, nil
}

// --- floats ---

func approxEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	if d <= 1e-6 {
		return true
	}
	if b < 0 {
		b = -b
	}
	return d <= 1e-6*b
}

// --- fixtures ---

func createIndividuals(srv *httptest.Server, name string, usl, lsl *float64) (string, error) {
	body := map[string]any{"name": name, "characteristic": "x", "chart_type": "individuals", "unit": "mm"}
	if usl != nil {
		body["usl"] = *usl
	}
	if lsl != nil {
		body["lsl"] = *lsl
	}
	var c struct{ ChartID string `json:"chart_id"` }
	if err := mustDo(srv, "POST", "/charts", body, false, &c); err != nil {
		return "", fmt.Errorf("create chart: %w", err)
	}
	return c.ChartID, nil
}

func addValue(srv *httptest.Server, id string, v float64) error {
	return mustDo(srv, "POST", "/charts/"+id+"/measurements",
		map[string]any{"values": []float64{v}}, false, nil)
}

func addValuesN(srv *httptest.Server, id string, vals []float64) error {
	for _, v := range vals {
		if err := addValue(srv, id, v); err != nil {
			return err
		}
	}
	return nil
}

func getLimits(srv *httptest.Server, id string) (limitResp, error) {
	var lim limitResp
	if err := mustDo(srv, "GET", "/charts/"+id+"/limits", nil, false, &lim); err != nil {
		return limitResp{}, err
	}
	return lim, nil
}

func getViolations(srv *httptest.Server, id string) ([]violationResp, error) {
	var v struct{ Violations []violationResp `json:"violations"` }
	if err := mustDo(srv, "GET", "/charts/"+id+"/violations", nil, false, &v); err != nil {
		return nil, err
	}
	return v.Violations, nil
}

func getCapability(srv *httptest.Server, id string) (capabilityResp, error) {
	var c capabilityResp
	if err := mustDo(srv, "GET", "/charts/"+id+"/capability", nil, false, &c); err != nil {
		return capabilityResp{}, err
	}
	return c, nil
}

type limitResp struct {
	ChartID       string  `json:"chart_id"`
	BaselineCount int     `json:"baseline_count"`
	CL            float64 `json:"cl"`
	UCL           float64 `json:"ucl"`
	LCL           float64 `json:"lcl"`
	SigmaWithin   float64 `json:"sigma_within"`
	SigmaOverall  float64 `json:"sigma_overall"`
}

type violationResp struct {
	MeasurementSeq int    `json:"measurement_seq"`
	Rule           string `json:"rule"`
	Severity       string `json:"severity"`
}

type capabilityResp struct {
	Estimable bool     `json:"estimable"`
	Status    string   `json:"status"`
	Cp        *float64 `json:"cp,omitempty"`
	Cpk       *float64 `json:"cpk,omitempty"`
	CPU       *float64 `json:"cpu,omitempty"`
	CPL       *float64 `json:"cpl,omitempty"`
	Cpm       *float64 `json:"cpm,omitempty"`
	Pp        *float64 `json:"pp,omitempty"`
	Ppk       *float64 `json:"ppk,omitempty"`
	DPMO      *float64 `json:"dpmo,omitempty"`
	Yield     *float64 `json:"yield,omitempty"`
	SigmaLevel *float64 `json:"sigma_level,omitempty"`
}

// --- expected-stats helpers (compute in the smoke test so assertions are not
// hardcoded floats; the engine and the test use the same formulas) ---

func mean(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func sampleStdDev(xs []float64) float64 {
	n := len(xs)
	if n < 2 {
		return 0
	}
	m := mean(xs)
	var ss float64
	for _, x := range xs {
		d := x - m
		ss += d * d
	}
	return sqrtf(ss / float64(n-1))
}

// movingRangeMean returns the mean of consecutive |x_i - x_{i-1}| (individuals MR).
func movingRangeMean(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var s float64
	for i := 1; i < len(xs); i++ {
		d := xs[i] - xs[i-1]
		if d < 0 {
			d = -d
		}
		s += d
	}
	return s / float64(len(xs)-1)
}

func sqrtf(x float64) float64 {
	if x < 0 {
		return 0
	}
	// Newton's method; selfcheck keeps zero third-party deps.
	if x == 0 {
		return 0
	}
	g := x
	for i := 0; i < 40; i++ {
		g = 0.5 * (g + x/g)
	}
	return g
}

// hasViolation reports whether the stored violations include a (seq, rule) hit.
func hasViolation(vs []violationResp, seq int, rule string) bool {
	for _, v := range vs {
		if v.MeasurementSeq == seq && v.Rule == rule {
			return true
		}
	}
	return false
}

// --- scenarios ---

// smokeIndividualsLimits: 5 single values; CL/sigma recompute to the expected
// values, baseline_count=5.
func smokeIndividualsLimits(srv *httptest.Server, dbPath string) error {
	usl, lsl := 20.0, 0.0
	id, err := createIndividuals(srv, "ilim", &usl, &lsl)
	if err != nil {
		return err
	}
	vals := []float64{10, 12, 11, 9, 10}
	if err := addValuesN(srv, id, vals); err != nil {
		return err
	}
	lim, err := getLimits(srv, id)
	if err != nil {
		return err
	}
	if lim.BaselineCount != 5 {
		return fmt.Errorf("baseline_count %d != 5", lim.BaselineCount)
	}
	expCL := mean(vals) // 10.4
	if !approxEq(lim.CL, expCL) {
		return fmt.Errorf("CL %g != %g", lim.CL, expCL)
	}
	expSigma := movingRangeMean(vals) / 1.128 // 1.5/1.128 ~ 1.3298
	if !approxEq(lim.SigmaWithin, expSigma) {
		return fmt.Errorf("sigma_within %g != %g", lim.SigmaWithin, expSigma)
	}
	expUCL := expCL + 3*expSigma
	if !approxEq(lim.UCL, expUCL) {
		return fmt.Errorf("UCL %g != %g", lim.UCL, expUCL)
	}
	if !approxEq(lim.SigmaOverall, sampleStdDev(vals)) {
		return fmt.Errorf("sigma_overall %g != %g", lim.SigmaOverall, sampleStdDev(vals))
	}
	return nil
}

// smokeXbarRSubgroupGuard: n=3; a 2-value subgroup is rejected 422; a 3-value
// subgroup is accepted.
func smokeXbarRSubgroupGuard(srv *httptest.Server, dbPath string) error {
	body := map[string]any{"name": "xr", "characteristic": "d", "chart_type": "xbar_r", "unit": "mm", "subgroup_size": 3}
	var c struct{ ChartID string `json:"chart_id"` }
	if err := mustDo(srv, "POST", "/charts", body, false, &c); err != nil {
		return err
	}
	id := c.ChartID
	// Wrong size -> 422.
	bad := map[string]any{"values": []float64{1, 2}}
	if _, err := expectStatus(srv, "POST", "/charts/"+id+"/measurements", bad, false, http.StatusUnprocessableEntity); err != nil {
		return err
	}
	// Right size -> 201.
	good := map[string]any{"values": []float64{1, 2, 3}}
	if err := mustDo(srv, "POST", "/charts/"+id+"/measurements", good, false, nil); err != nil {
		return err
	}
	return nil
}

// smokePChartBounds: defectives>n_observed rejected; a valid point records p.
func smokePChartBounds(srv *httptest.Server, dbPath string) error {
	body := map[string]any{"name": "pc", "characteristic": "def", "chart_type": "p_chart", "unit": "cnt", "subgroup_size": 1}
	var c struct{ ChartID string `json:"chart_id"` }
	if err := mustDo(srv, "POST", "/charts", body, false, &c); err != nil {
		return err
	}
	id := c.ChartID
	// defectives=5 > n_observed=3 -> 422.
	bad := map[string]any{"defectives": 5, "n_observed": 3}
	if _, err := expectStatus(srv, "POST", "/charts/"+id+"/measurements", bad, false, http.StatusUnprocessableEntity); err != nil {
		return err
	}
	// defectives=0,n=10 and defectives=3,n=10 valid.
	if err := mustDo(srv, "POST", "/charts/"+id+"/measurements", map[string]any{"defectives": 0, "n_observed": 10}, false, nil); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/charts/"+id+"/measurements", map[string]any{"defectives": 3, "n_observed": 10}, false, nil); err != nil {
		return err
	}
	lim, err := getLimits(srv, id)
	if err != nil {
		return err
	}
	// pbar = (0+3)/(10+10) = 0.15.
	if !approxEq(lim.CL, 0.15) {
		return fmt.Errorf("pbar %g != 0.15", lim.CL)
	}
	return nil
}

// smokeExcludeRestore: excluding a point recomputes limits from the remaining
// baseline; restoring brings them back.
func smokeExcludeRestore(srv *httptest.Server, dbPath string) error {
	id, err := createIndividuals(srv, "excl", nil, nil)
	if err != nil {
		return err
	}
	vals := []float64{10, 12, 11, 9, 10}
	if err := addValuesN(srv, id, vals); err != nil {
		return err
	}
	lim0, err := getLimits(srv, id)
	if err != nil {
		return err
	}
	// Find the measurement with subgroup_seq=2 (value 12) and exclude it.
	var list struct {
		Measurements []struct {
			MeasurementID string `json:"measurement_id"`
			SubgroupSeq   int    `json:"subgroup_seq"`
		} `json:"measurements"`
	}
	if err := mustDo(srv, "GET", "/charts/"+id+"/measurements", nil, false, &list); err != nil {
		return err
	}
	var mid string
	for _, m := range list.Measurements {
		if m.SubgroupSeq == 2 {
			mid = m.MeasurementID
		}
	}
	if mid == "" {
		return fmt.Errorf("measurement seq 2 not found")
	}
	if err := mustDo(srv, "DELETE", "/charts/"+id+"/measurements/"+mid, nil, false, nil); err != nil {
		return err
	}
	lim1, err := getLimits(srv, id)
	if err != nil {
		return err
	}
	// Remaining baseline = {10,11,9,10} (removed value 12).
	rem := []float64{10, 11, 9, 10}
	if lim1.BaselineCount != 4 {
		return fmt.Errorf("after exclude baseline %d != 4", lim1.BaselineCount)
	}
	if !approxEq(lim1.CL, mean(rem)) {
		return fmt.Errorf("after exclude CL %g != %g", lim1.CL, mean(rem))
	}
	// Restore.
	if err := mustDo(srv, "POST", "/charts/"+id+"/measurements/"+mid+"/restore", nil, false, nil); err != nil {
		return err
	}
	lim2, err := getLimits(srv, id)
	if err != nil {
		return err
	}
	if !approxEq(lim2.CL, lim0.CL) {
		return fmt.Errorf("after restore CL %g != %g", lim2.CL, lim0.CL)
	}
	if lim2.BaselineCount != 5 {
		return fmt.Errorf("after restore baseline %d != 5", lim2.BaselineCount)
	}
	return nil
}

// smokeWestgard13s: an outlier >3 sigma trips 1-3s (reject).
func smokeWestgard13s(srv *httptest.Server, dbPath string) error {
	id, err := createIndividuals(srv, "w13s", nil, nil)
	if err != nil {
		return err
	}
	// 10 points ~10.6 with small MRs establish a sigma, then a 100 outlier.
	base := []float64{10, 12, 11, 9, 10, 12, 11, 9, 10, 12}
	if err := addValuesN(srv, id, base); err != nil {
		return err
	}
	if err := addValue(srv, id, 100); err != nil {
		return err
	}
	vs, err := getViolations(srv, id)
	if err != nil {
		return err
	}
	if !hasViolation(vs, 11, "1-3s") {
		return fmt.Errorf("no 1-3s at seq 11; got %+v", vs)
	}
	// 1-3s is a reject.
	for _, v := range vs {
		if v.MeasurementSeq == 11 && v.Rule == "1-3s" && v.Severity != "reject" {
			return fmt.Errorf("1-3s severity %q != reject", v.Severity)
		}
	}
	return nil
}

// smokeWestgard22s: two consecutive same-side points beyond 2s trip 2-2s.
func smokeWestgard22s(srv *httptest.Server, dbPath string) error {
	id, err := createIndividuals(srv, "w22s", nil, nil)
	if err != nil {
		return err
	}
	base := []float64{10, 12, 11, 9, 10, 12, 11, 9, 10, 12}
	if err := addValuesN(srv, id, base); err != nil {
		return err
	}
	if err := addValue(srv, id, 100); err != nil {
		return err
	}
	if err := addValue(srv, id, 100); err != nil {
		return err
	}
	vs, err := getViolations(srv, id)
	if err != nil {
		return err
	}
	if !hasViolation(vs, 12, "2-2s") {
		return fmt.Errorf("no 2-2s at seq 12; got %+v", vs)
	}
	return nil
}

// smokeWestgardR4s: opposite-side points beyond 2s trip R-4s.
func smokeWestgardR4s(srv *httptest.Server, dbPath string) error {
	id, err := createIndividuals(srv, "wr4s", nil, nil)
	if err != nil {
		return err
	}
	base := []float64{10, 12, 11, 9, 10, 12, 11, 9, 10, 12}
	if err := addValuesN(srv, id, base); err != nil {
		return err
	}
	if err := addValue(srv, id, 100); err != nil {
		return err
	}
	if err := addValue(srv, id, -100); err != nil {
		return err
	}
	vs, err := getViolations(srv, id)
	if err != nil {
		return err
	}
	if !hasViolation(vs, 12, "R-4s") {
		return fmt.Errorf("no R-4s at seq 12; got %+v", vs)
	}
	return nil
}

// smokeWestgard41s: four consecutive same-side points beyond 1s trip 4-1s.
func smokeWestgard41s(srv *httptest.Server, dbPath string) error {
	id, err := createIndividuals(srv, "w41s", nil, nil)
	if err != nil {
		return err
	}
	base := []float64{10, 12, 11, 9, 10, 12, 11, 9, 10, 12}
	if err := addValuesN(srv, id, base); err != nil {
		return err
	}
	for _, v := range []float64{100, 100, 100, 100} {
		if err := addValue(srv, id, v); err != nil {
			return err
		}
	}
	vs, err := getViolations(srv, id)
	if err != nil {
		return err
	}
	if !hasViolation(vs, 14, "4-1s") {
		return fmt.Errorf("no 4-1s at seq 14; got %+v", vs)
	}
	return nil
}

// smokeWestgard10x: ten same-side points trip 10-x.
func smokeWestgard10x(srv *httptest.Server, dbPath string) error {
	id, err := createIndividuals(srv, "w10x", nil, nil)
	if err != nil {
		return err
	}
	// Ten zeros, then ten 100s. CL=50, last ten points all on + side -> 10-x.
	for i := 0; i < 10; i++ {
		if err := addValue(srv, id, 0); err != nil {
			return err
		}
	}
	for i := 0; i < 10; i++ {
		if err := addValue(srv, id, 100); err != nil {
			return err
		}
	}
	vs, err := getViolations(srv, id)
	if err != nil {
		return err
	}
	if !hasViolation(vs, 20, "10-x") {
		return fmt.Errorf("no 10-x at seq 20; got %+v", vs)
	}
	return nil
}

// smokeRuleToggle: disabling the 1-3s rule removes the 1-3s violation.
func smokeRuleToggle(srv *httptest.Server, dbPath string) error {
	id, err := createIndividuals(srv, "toggle", nil, nil)
	if err != nil {
		return err
	}
	base := []float64{10, 12, 11, 9, 10, 12, 11, 9, 10, 12}
	if err := addValuesN(srv, id, base); err != nil {
		return err
	}
	if err := addValue(srv, id, 100); err != nil {
		return err
	}
	before, err := getViolations(srv, id)
	if err != nil {
		return err
	}
	if !hasViolation(before, 11, "1-3s") {
		return fmt.Errorf("expected 1-3s before toggle")
	}
	// Disable 1-3s.
	rules := map[string]bool{"1-3s": false}
	if err := mustDo(srv, "PUT", "/charts/"+id+"/rules", map[string]any{"rules": rules}, false, nil); err != nil {
		return err
	}
	after, err := getViolations(srv, id)
	if err != nil {
		return err
	}
	if hasViolation(after, 11, "1-3s") {
		return fmt.Errorf("1-3s still present after disable")
	}
	return nil
}

// smokeCapability: <2 points or sigma==0 -> not_estimable; with a baseline and
// spec -> estimable with Cpk.
func smokeCapability(srv *httptest.Server, dbPath string) error {
	usl, lsl := 20.0, 0.0
	id, err := createIndividuals(srv, "cap", &usl, &lsl)
	if err != nil {
		return err
	}
	// One point -> sigma_within=0 -> not_estimable.
	if err := addValue(srv, id, 10); err != nil {
		return err
	}
	c, err := getCapability(srv, id)
	if err != nil {
		return err
	}
	if c.Estimable || c.Status != "not_estimable" {
		return fmt.Errorf("1 point: status %q, estimable=%v", c.Status, c.Estimable)
	}
	// Add more points -> estimable.
	if err := addValuesN(srv, id, []float64{12, 11, 9, 10}); err != nil {
		return err
	}
	c2, err := getCapability(srv, id)
	if err != nil {
		return err
	}
	if !c2.Estimable || c2.Status != "estimable" {
		return fmt.Errorf("5 points: status %q, estimable=%v", c2.Status, c2.Estimable)
	}
	if c2.Cpk == nil {
		return fmt.Errorf("estimable but Cpk nil")
	}
	if c2.Cp == nil {
		return fmt.Errorf("two-sided spec but Cp nil")
	}
	return nil
}

// smokeOneSidedSpec: only USL -> Cp/Pp/Cpm not estimable, but CPU/Cpk present.
func smokeOneSidedSpec(srv *httptest.Server, dbPath string) error {
	usl := 20.0
	body := map[string]any{"name": "one", "characteristic": "x", "chart_type": "individuals", "unit": "mm", "usl": usl}
	var ch struct{ ChartID string `json:"chart_id"` }
	if err := mustDo(srv, "POST", "/charts", body, false, &ch); err != nil {
		return err
	}
	id := ch.ChartID
	if err := addValuesN(srv, id, []float64{10, 12, 11, 9, 10}); err != nil {
		return err
	}
	c, err := getCapability(srv, id)
	if err != nil {
		return err
	}
	if !c.Estimable {
		return fmt.Errorf("one-sided should be estimable")
	}
	if c.Cp != nil {
		return fmt.Errorf("one-sided should not have Cp")
	}
	if c.CPU == nil || c.Cpk == nil {
		return fmt.Errorf("one-sided should have CPU and Cpk")
	}
	return nil
}

// smokeXbarROverallSigma: identical subgroup means (no between-subgroup drift)
// but varying raw observations. sigma_overall and the overall-sigma capability
// indices must reflect the raw variation, not collapse to 0. Before the fix,
// sigma_overall was computed over the subgroup means (== 0 here) and the
// capability report passed sigma_within as sigma_overall, so Pp/Ppk/Cpm/DPMO/
// Yield were either nil or computed against the wrong sigma.
func smokeXbarROverallSigma(srv *httptest.Server, dbPath string) error {
	usl, lsl, target := 30.0, -10.0, 3.0
	body := map[string]any{"name": "xro", "characteristic": "d", "chart_type": "xbar_r", "unit": "mm", "subgroup_size": 2,
		"usl": usl, "lsl": lsl, "target": target}
	var ch struct{ ChartID string `json:"chart_id"` }
	if err := mustDo(srv, "POST", "/charts", body, false, &ch); err != nil {
		return err
	}
	id := ch.ChartID
	// Every subgroup has mean 3; the raw observations vary (range 4 each).
	for _, sub := range [][]float64{{1, 5}, {2, 4}, {3, 3}, {4, 2}, {5, 1}} {
		if err := mustDo(srv, "POST", "/charts/"+id+"/measurements",
			map[string]any{"values": sub}, false, nil); err != nil {
			return err
		}
	}
	lim, err := getLimits(srv, id)
	if err != nil {
		return err
	}
	if !approxEq(lim.CL, 3) {
		return fmt.Errorf("CL %g != 3", lim.CL)
	}
	if !(lim.SigmaOverall > 0) {
		return fmt.Errorf("sigma_overall %g must be > 0 (raw observations vary even though means are equal)", lim.SigmaOverall)
	}
	c, err := getCapability(srv, id)
	if err != nil {
		return err
	}
	if !c.Estimable {
		return fmt.Errorf("capability not estimable (status %q)", c.Status)
	}
	// The overall-sigma indices must be present; before the fix they were nil
	// (sigma_overall collapsed to 0) or computed against sigma_within.
	if c.Pp == nil || c.Ppk == nil || c.Cpm == nil || c.DPMO == nil || c.Yield == nil {
		return fmt.Errorf("overall-sigma indices missing: Pp=%v Ppk=%v Cpm=%v DPMO=%v Yield=%v",
			c.Pp, c.Ppk, c.Cpm, c.DPMO, c.Yield)
	}
	// Consistency must still hold after the fix.
	var rep struct {
		OK            bool `json:"ok"`
		ChartsChecked int  `json:"charts_checked"`
	}
	if err := mustDo(srv, "POST", "/admin/recompute", nil, true, &rep); err != nil {
		return err
	}
	if !rep.OK {
		return fmt.Errorf("consistency not ok after fix")
	}
	return nil
}

// smokeConsistency: POST /admin/recompute reports ok=true after seeding data.
func smokeConsistency(srv *httptest.Server, dbPath string) error {
	usl, lsl := 30.0, -10.0
	id, err := createIndividuals(srv, "cons", &usl, &lsl)
	if err != nil {
		return err
	}
	if err := addValuesN(srv, id, []float64{10, 12, 11, 9, 10, 12, 11, 9}); err != nil {
		return err
	}
	var rep struct {
		OK            bool `json:"ok"`
		ChartsChecked int  `json:"charts_checked"`
	}
	if err := mustDo(srv, "POST", "/admin/recompute", nil, true, &rep); err != nil {
		return err
	}
	if !rep.OK || rep.ChartsChecked < 1 {
		return fmt.Errorf("consistency: ok=%v checked=%d", rep.OK, rep.ChartsChecked)
	}
	return nil
}

// smokeFrontend: the embedded frontend index page is served at GET /.
func smokeFrontend(srv *httptest.Server, dbPath string) error {
	code, body, err := doJSON(srv, "GET", "/", nil, false)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET /: expected 200, got %d", code)
	}
	if !strings.Contains(string(body), "SPC") {
		return fmt.Errorf("GET /: page does not mention SPC")
	}
	return nil
}

// smokeRestartRecovery seeds limits + violations, closes the store, reopens
// the same file and asserts the recomputed limits and violations are
// unchanged. This is the restart-recovery path: limits + violations are a pure
// function of the persisted measurements.
func smokeRestartRecovery(dbPath string) error {
	srv, err := newServer(dbPath)
	if err != nil {
		return err
	}
	usl, lsl := 30.0, -10.0
	id, err := createIndividuals(srv, "recover", &usl, &lsl)
	if err != nil {
		srv.Close()
		return err
	}
	if err := addValuesN(srv, id, []float64{10, 12, 11, 9, 10, 12, 11, 9, 10, 12, 100}); err != nil {
		srv.Close()
		return err
	}
	limBefore, err := getLimits(srv, id)
	if err != nil {
		srv.Close()
		return err
	}
	vioBefore, err := getViolations(srv, id)
	if err != nil {
		srv.Close()
		return err
	}
	// Close (closes the store via the shutdown hook), then reopen the same file.
	srv.Close()
	srv2, err := newServer(dbPath)
	if err != nil {
		return fmt.Errorf("reopen store: %w", err)
	}
	defer srv2.Close()
	limAfter, err := getLimits(srv2, id)
	if err != nil {
		return err
	}
	if limBefore.BaselineCount != limAfter.BaselineCount {
		return fmt.Errorf("restart baseline %d -> %d", limBefore.BaselineCount, limAfter.BaselineCount)
	}
	if !approxEq(limBefore.CL, limAfter.CL) || !approxEq(limBefore.SigmaWithin, limAfter.SigmaWithin) ||
		!approxEq(limBefore.UCL, limAfter.UCL) || !approxEq(limBefore.LCL, limAfter.LCL) {
		return fmt.Errorf("restart limits changed: before %+v after %+v", limBefore, limAfter)
	}
	vioAfter, err := getViolations(srv2, id)
	if err != nil {
		return err
	}
	if len(vioBefore) != len(vioAfter) {
		return fmt.Errorf("restart violation count %d -> %d", len(vioBefore), len(vioAfter))
	}
	// Compare as multisets keyed by (seq, rule).
	key := func(v violationResp) string { return fmt.Sprintf("%d:%s", v.MeasurementSeq, v.Rule) }
	same := make(map[string]int, len(vioBefore))
	for _, v := range vioBefore {
		same[key(v)]++
	}
	for _, v := range vioAfter {
		same[key(v)]--
	}
	for k, n := range same {
		if n != 0 {
			return fmt.Errorf("restart violation mismatch %s delta %d", k, n)
		}
	}
	return nil
}
