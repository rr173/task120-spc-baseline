package httpapi

import (
	"bytes"
	"net/http"
	"testing"
)

func TestBug08_TrailingJSONIsRejected(t *testing.T) {
	s, cl := newTestServer(t)
	defer cl()
	body := []byte(`{"name":"x","characteristic":"x","chart_type":"individuals","subgroup_size":1}{"name":"y"}`)
	r, _ := http.NewRequest(http.MethodPost, s.URL+"/charts", bytes.NewReader(body))
	resp, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("trailing JSON accepted with status %d", resp.StatusCode)
	}
}
