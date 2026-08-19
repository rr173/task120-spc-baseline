package store

import "encoding/json"

// jsonMarshal / jsonUnmarshal wrap encoding/json so the store package keeps a
// single import point for the values-JSON encoding used by the measurement
// table.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
