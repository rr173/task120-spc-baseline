package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBug05_DefaultListHidesExcluded(t *testing.T) {
	s, cl := newTestServer(t)
	defer cl()
	st, b := do(t, s, http.MethodPost, "/charts", map[string]any{"name": "x", "characteristic": "x", "chart_type": "individuals", "subgroup_size": 1}, false)
	if st != 201 {
		t.Fatal(st, string(b))
	}
	var c struct {
		ID string `json:"chart_id"`
	}
	json.Unmarshal(b, &c)
	st, b = do(t, s, http.MethodPost, "/charts/"+c.ID+"/measurements", map[string]any{"values": []float64{1}}, false)
	var m struct {
		ID string `json:"measurement_id"`
	}
	json.Unmarshal(b, &m)
	do(t, s, http.MethodDelete, "/charts/"+c.ID+"/measurements/"+m.ID, nil, false)
	st, b = do(t, s, http.MethodGet, "/charts/"+c.ID+"/measurements", nil, false)
	if st != 200 {
		t.Fatal(st)
	}
	var out struct {
		Measurements []any `json:"measurements"`
	}
	json.Unmarshal(b, &out)
	if len(out.Measurements) != 0 {
		t.Fatalf("excluded leaked: %s", b)
	}
}
