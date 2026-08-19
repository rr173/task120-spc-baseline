package httpapi

import (
	"net/http"
	"testing"
)

func TestBug04_EmptyCollectionsAreArrays(t *testing.T) {
	s, cl := newTestServer(t)
	defer cl()
	st, body := do(t, s, http.MethodGet, "/charts", nil, false)
	if st != 200 || string(body) != "{\"charts\":[]}\n" {
		t.Fatalf("empty charts: %d %s", st, body)
	}
}
