package realtime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// heartbeat keeps the connection (and any intermediary proxy) from treating an
// idle stream as dead.
const heartbeat = 25 * time.Second

// RegisterRoutes mounts the admin SSE stream at GET /admin/events (admin only).
// Native EventSource can't send an Authorization header, so the frontend reads
// this with fetch() + the Bearer token; RequireAdmin validates it normally.
func RegisterRoutes(r chi.Router, h *Hub, adminOnly func(http.Handler) http.Handler) {
	r.Group(func(r chi.Router) {
		r.Use(adminOnly)
		r.Get("/admin/events", h.stream)
	})
}

func (h *Hub) stream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)

	rc := http.NewResponseController(w)
	// SSE is a long-lived write; clear the server's WriteTimeout for THIS
	// connection only (leaving it in place for every other endpoint).
	_ = rc.SetWriteDeadline(time.Time{})

	sub := h.subscribe()
	defer h.unsubscribe(sub)

	// Tell the client how long to wait before reconnecting, then flush headers.
	if _, err := io.WriteString(w, "retry: 5000\n\n"); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return // ResponseWriter can't stream (no flusher) — nothing to do
	}

	ping := time.NewTicker(heartbeat)
	defer ping.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done(): // client disconnected
			return
		case <-h.Done(): // server shutting down
			return
		case evt := <-sub.ch:
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\nid: %s\ndata: %s\n\n", evt.Type, evt.ID, data); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
