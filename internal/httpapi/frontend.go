package httpapi

import (
	"io"
	"net/http"
	"strings"
)

// registerFrontend serves the embedded static frontend. The mux already
// registers specific patterns for every API route (GET /healthz, POST /charts,
// etc.), and Go 1.22's ServeMux gives more specific patterns precedence, so the
// broad frontend patterns below only catch non-API paths:
//
//	GET /            -> web/index.html
//	GET /static/{...} -> web/{file} (app.js, style.css)
func registerFrontend(mux *http.ServeMux, webFS http.FileSystem) {
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		f, err := webFS.Open("/index.html")
		if err != nil {
			writeError(w, http.StatusNotFound, "frontend not available")
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, f)
	})

	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/static/")
		if rel == "" || strings.Contains(rel, "..") {
			http.NotFound(w, r)
			return
		}
		f, err := webFS.Open("/" + rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		setStaticType(w, rel)
		_, _ = io.Copy(w, f)
	})
}

// setStaticType sets the Content-Type for the well-known static asset kinds
// the frontend uses. Unknown kinds fall back to octet-stream.
func setStaticType(w http.ResponseWriter, name string) {
	switch {
	case strings.HasSuffix(name, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(name, ".json"):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
}
