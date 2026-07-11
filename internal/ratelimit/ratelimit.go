// Package ratelimit provides a lightweight per-client-IP request limiter used
// as HTTP middleware. It backs both a lenient global limit and a strict limit
// on authentication endpoints.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/trimo/backend/internal/httpx"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Limiter tracks a token-bucket rate limiter per client IP. Idle entries are
// evicted so the map cannot grow without bound.
type Limiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     rate.Limit
	burst    int
}

// New builds a Limiter allowing r requests/second per IP with the given burst,
// and starts a background eviction loop that stops when ctx would (via the
// process lifetime). Passing burst <= 0 disables limiting (Middleware is a
// pass-through), which keeps wiring simple when a limit is turned off.
func New(rps float64, burst int) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*visitor),
		rate:     rate.Limit(rps),
		burst:    burst,
	}
	if burst > 0 {
		go l.sweep()
	}
	return l
}

func (l *Limiter) limiterFor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.visitors[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter
}

func (l *Limiter) sweep() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		for ip, v := range l.visitors {
			if time.Since(v.lastSeen) > 5*time.Minute {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware enforces the limit per client IP, responding 429 when exceeded.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	if l.burst <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.limiterFor(clientIP(r)).Allow() {
			w.Header().Set("Retry-After", "60")
			httpx.Error(w, http.StatusTooManyRequests, "too many requests; please slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
