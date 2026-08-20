package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"task120-spc/internal/domain"
)

// decode reads a JSON body into v. An empty body is allowed (v stays zero) so
// endpoints with no required fields can be called with no body; a malformed
// non-empty body is a 400.
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
	return json.Unmarshal(body, v)
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
