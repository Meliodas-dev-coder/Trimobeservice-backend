// Package httpx contains small helpers for JSON request/response handling
// shared across all HTTP handlers.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Envelope is a convenience type for ad-hoc JSON objects.
type Envelope map[string]any

// ErrorResponse is the standard error body shape.
type ErrorResponse struct {
	Error   string            `json:"error"`
	Details map[string]string `json:"details,omitempty"`
}

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "error", err)
	}
}

// Error writes a simple error message with the given status code.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, ErrorResponse{Error: msg})
}

// ValidationError writes a 422 with field-level problems.
func ValidationError(w http.ResponseWriter, details map[string]string) {
	JSON(w, http.StatusUnprocessableEntity, ErrorResponse{Error: "validation failed", Details: details})
}
