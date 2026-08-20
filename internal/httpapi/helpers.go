package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"task120-spc/internal/domain"
)

// decode reads exactly one JSON value from the body into v. An empty body is
// allowed (v stays zero) so endpoints with no required fields can be called
// with no body. A malformed body, or one carrying any trailing content after
// the first JSON value (a second JSON object, stray bytes, ...), is rejected:
// the streaming decoder otherwise silently consumes only the leading value and
// lets a smuggled second payload through.
func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(v); err != nil {
		return err
	}
	// A clean single-value body leaves the decoder at io.EOF. Anything else —
	// a second JSON value (dec.Decode succeeds) or trailing garbage (a syntax
	// error) — means the body is not a single JSON document.
	var extra json.RawMessage
	if err := dec.Decode(&extra); err == nil {
		return errors.New("unexpected trailing content")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// writeJSON sets the content type and writes v as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeServiceError maps a domain sentinel error to an HTTP status. NotEstimable
// is NOT a hard error: callers must branch on it before calling this helper.
// Unknown errors fall back to 500.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvariant):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrTerminal):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// ptrFloat copies a *float64 so the handler layer can pass spec values into the
// service without aliasing request-local storage.
func ptrFloat(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
