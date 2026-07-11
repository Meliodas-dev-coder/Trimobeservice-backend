package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareThrottlesPerIP(t *testing.T) {
	// 0 rps refill, burst of 3: the 4th request from an IP must be rejected.
	l := New(0, 3)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	call := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 3; i++ {
		if code := call("1.2.3.4"); code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, code)
		}
	}
	if code := call("1.2.3.4"); code != http.StatusTooManyRequests {
		t.Fatalf("4th request: got %d, want 429", code)
	}

	// A different IP has its own bucket and is unaffected.
	if code := call("5.6.7.8"); code != http.StatusOK {
		t.Fatalf("other IP: got %d, want 200", code)
	}
}

func TestMiddlewareDisabledWhenBurstZero(t *testing.T) {
	l := New(0, 0) // limiting disabled
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "9.9.9.9:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200 (limiting should be off)", i+1, rec.Code)
		}
	}
}
