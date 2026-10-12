package api

import (
	"encoding/json"
	"net/http"
)

// decodeJSON reads and decodes a JSON request body into T. It's generic
// so every handler gets type-safe decoding without writing the same
// boilerplate three times for three different request shapes.
func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // typo'd or stale fields in a request body fail loudly instead of silently being ignored
	err := dec.Decode(&v)
	return v, err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
