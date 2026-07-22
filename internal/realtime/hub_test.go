package realtime

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/trimo/backend/internal/authz"
)

// TestStreamDeliversEvent boots the real SSE handler over an httptest server and
// asserts a published event reaches a connected client with the right headers
// and framing — exercising subscribe → publish → flush end-to-end.
func TestStreamDeliversEvent(t *testing.T) {
	hub := NewHub()
	r := chi.NewRouter()
	passthrough := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := authz.WithCurrentAccess(r.Context(), authz.CurrentAccess{IsSuper: true, Permissions: authz.PermissionSet{}})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	} // stand in for RequireAdmin
	RegisterRoutes(r, hub, passthrough)

	srv := httptest.NewServer(r)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/admin/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read the first data: line off the stream in the background.
	got := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "data:") {
				got <- line
				return
			}
		}
	}()

	// Publish repeatedly to avoid racing the client's subscription registration.
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case line := <-got:
			if !strings.Contains(line, "order.created") {
				t.Fatalf("data line missing event type: %q", line)
			}
			if !strings.Contains(line, "ORD-1") {
				t.Fatalf("data line missing payload: %q", line)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for streamed event")
		case <-tick.C:
			hub.Publish(Event{Type: "order.created", Payload: map[string]any{"id": 1, "number": "ORD-1"}})
		}
	}
}

// TestPublishNeverBlocks verifies a slow/full subscriber can't stall a producer.
func TestPublishNeverBlocks(t *testing.T) {
	hub := NewHub()
	sub := hub.subscribe() // never drained
	defer hub.unsubscribe(sub)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			hub.Publish(Event{Type: "order.created"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full subscriber buffer")
	}
}
