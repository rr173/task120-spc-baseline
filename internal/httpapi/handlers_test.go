package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"task120-spc/internal/chart"
	"task120-spc/internal/store"
	"task120-spc/internal/webfs"
)

func newTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "spc-http-*")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "http.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := Services{Chart: chart.New(st)}
	mux := NewMux(svc, "admin-secret", http.FS(webfs.FS()))
	srv := httptest.NewServer(mux)
	return srv, func() {
		srv.Close()
		_ = st.Close()
		os.RemoveAll(dir)
	}
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any, admin bool) (int, []byte) {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(buf))
		r.Header.Set("Content-Type", "application/json")
	}
	if admin {
		r.Header.Set(AdminTokenHeader, "admin-secret")
	}
	rec := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(rec, r)
	return rec.Code, rec.Body.Bytes()
}

func TestHealthz(t *testing.T) {
	srv, closeFn := newTestServer(t)
	defer closeFn()
	code, body := do(t, srv, "GET", "/healthz", nil, false)
	if code != 200 {
		t.Fatalf("healthz %d", code)
	}
	if !bytes.Contains(body, []byte("ok")) {
		t.Errorf("healthz body %s", body)
	}
}

func TestCreateChartAndLimitsFlow(t *testing.T) {
	srv, closeFn := newTestServer(t)
	defer closeFn()
	usl, lsl := 20.0, 0.0
	body := map[string]any{"name": "n", "characteristic": "c", "chart_type": "individuals", "unit": "mm", "usl": usl, "lsl": lsl}
	code, resp := do(t, srv, "POST", "/charts", body, false)
	if code != 201 {
		t.Fatalf("create chart %d: %s", code, resp)
	}
	var c struct{ ChartID string `json:"chart_id"` }
	json.Unmarshal(resp, &c)
	for _, v := range []float64{10, 12, 11, 9, 10} {
		code, _ = do(t, srv, "POST", "/charts/"+c.ChartID+"/measurements", map[string]any{"values": []float64{v}}, false)
		if code != 201 {
			t.Fatalf("add measurement %d", code)
		}
	}
	code, resp = do(t, srv, "GET", "/charts/"+c.ChartID+"/limits", nil, false)
	if code != 200 {
		t.Fatalf("limits %d", code)
	}
	var lim limitJSON
	json.Unmarshal(resp, &lim)
	if lim.BaselineCount != 5 {
		t.Errorf("baseline %d != 5", lim.BaselineCount)
	}
}

func TestXbarRSubgroupSizeRejected(t *testing.T) {
	srv, closeFn := newTestServer(t)
	defer closeFn()
	body := map[string]any{"name": "n", "characteristic": "c", "chart_type": "xbar_r", "unit": "mm", "subgroup_size": 3}
	_, resp := do(t, srv, "POST", "/charts", body, false)
	var c struct{ ChartID string `json:"chart_id"` }
	json.Unmarshal(resp, &c)
	code, _ := do(t, srv, "POST", "/charts/"+c.ChartID+"/measurements", map[string]any{"values": []float64{1, 2}}, false)
	if code != 422 {
		t.Errorf("expected 422 for wrong size, got %d", code)
	}
}

func TestCapabilityNotEstimableThenEstimable(t *testing.T) {
	srv, closeFn := newTestServer(t)
	defer closeFn()
	usl, lsl := 20.0, 0.0
	body := map[string]any{"name": "n", "characteristic": "c", "chart_type": "individuals", "unit": "mm", "usl": usl, "lsl": lsl}
	_, resp := do(t, srv, "POST", "/charts", body, false)
	var c struct{ ChartID string `json:"chart_id"` }
	json.Unmarshal(resp, &c)
	do(t, srv, "POST", "/charts/"+c.ChartID+"/measurements", map[string]any{"values": []float64{10}}, false)
	_, resp = do(t, srv, "GET", "/charts/"+c.ChartID+"/capability", nil, false)
	var cap capabilityJSON
	json.Unmarshal(resp, &cap)
	if cap.Estimable {
		t.Errorf("1 point should be not_estimable")
	}
	for _, v := range []float64{12, 11, 9, 10} {
		do(t, srv, "POST", "/charts/"+c.ChartID+"/measurements", map[string]any{"values": []float64{v}}, false)
	}
	_, resp = do(t, srv, "GET", "/charts/"+c.ChartID+"/capability", nil, false)
	json.Unmarshal(resp, &cap)
	if !cap.Estimable {
		t.Errorf("5 points should be estimable")
	}
}

func TestAdminRecomputeRequiresToken(t *testing.T) {
	srv, closeFn := newTestServer(t)
	defer closeFn()
	code, _ := do(t, srv, "POST", "/admin/recompute", nil, false)
	if code != 401 {
		t.Errorf("expected 401 without token, got %d", code)
	}
}

func TestFrontendServed(t *testing.T) {
	srv, closeFn := newTestServer(t)
	defer closeFn()
	code, body := do(t, srv, "GET", "/", nil, false)
	if code != 200 {
		t.Fatalf("GET / %d", code)
	}
	if !bytes.Contains(body, []byte("SPC")) {
		t.Errorf("frontend missing SPC")
	}
}
