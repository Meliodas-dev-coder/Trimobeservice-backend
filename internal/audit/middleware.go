package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/trimo/backend/internal/auth"
)

// maxPayloadBytes caps how much of a request body we store. Larger bodies are
// recorded without a payload (the who/what/when is still captured).
const maxPayloadBytes = 16 << 10 // 16 KiB

// Middleware records admin writes. It is meant to wrap RequireAdmin so it runs
// with the acting user already in the request context.
type Middleware struct {
	svc *Service
}

func NewMiddleware(svc *Service) *Middleware {
	return &Middleware{svc: svc}
}

// Record wraps admin handlers: after a successful mutating request it stores an
// audit row (who / what / when). Non-mutating requests (GET/HEAD, incl. SSE) are
// passed straight through untouched.
func (m *Middleware) Record(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutating(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		payload := capturePayload(r) // reads + restores the body

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Only successful writes are worth an audit row.
		if rec.status < 200 || rec.status >= 300 {
			return
		}

		entry := Entry{
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: rec.status,
			Payload:    payload,
		}
		if uid, ok := auth.UserIDFromContext(r.Context()); ok {
			entry.ActorUserID = &uid
		}
		if tType, tID := parseTarget(r.URL.Path); tType != "" {
			entry.TargetType = &tType
			if tID > 0 {
				entry.TargetID = &tID
			}
		}
		m.svc.Record(r.Context(), entry)
	})
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// capturePayload reads a JSON request body and restores it so the handler can
// still read it. Non-JSON bodies (e.g. multipart uploads) are skipped, oversized
// bodies are dropped, and known secret keys are redacted.
func capturePayload(r *http.Request) []byte {
	if r.Body == nil || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return nil
	}
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	// Always restore the full body so the downstream handler is unaffected.
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) == 0 || len(body) > maxPayloadBytes || !json.Valid(body) {
		return nil
	}
	return redactSecrets(body)
}

// redactSecrets blanks common sensitive keys on top-level JSON objects so they
// never land in the audit log. Non-object JSON is stored as-is (already valid).
func redactSecrets(body []byte) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	redacted := false
	for _, k := range []string{"password", "current_password", "new_password", "token", "refresh_token", "access_token"} {
		if _, ok := obj[k]; ok {
			obj[k] = "[redacted]"
			redacted = true
		}
	}
	if !redacted {
		return body
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	return out
}

// parseTarget pulls a resource + numeric id out of an admin path, best-effort:
// /api/v1/admin/orders/5/status -> ("orders", 5); /api/v1/admin/products -> ("products", 0).
func parseTarget(path string) (string, int64) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p != "admin" || i+1 >= len(parts) {
			continue
		}
		resource := parts[i+1]
		var id int64
		for _, seg := range parts[i+2:] {
			if n, err := strconv.ParseInt(seg, 10, 64); err == nil {
				id = n
				break
			}
		}
		return resource, id
	}
	return "", 0
}

// statusRecorder captures the response status code so we only log successful
// writes. It forwards writes untouched.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.written = true // implicit 200 if WriteHeader was never called
	return s.ResponseWriter.Write(b)
}
