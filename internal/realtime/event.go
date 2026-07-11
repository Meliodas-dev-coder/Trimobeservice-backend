// Package realtime is an in-process pub/sub hub that fans domain events out to
// connected admins over Server-Sent Events (SSE). Producers (the domain
// handlers) depend only on the Publisher interface; the Hub is the single
// subscriber-side implementation. It is intentionally in-memory: the backend is
// a single-process monolith, so no external broker is needed. If the service is
// ever scaled to multiple replicas, swap the Hub behind Publisher for a
// Redis/NATS-backed fan-out without touching producers.
package realtime

import "time"

// Event is one thing that happened, streamed to admins as an SSE message. Type
// is the SSE event name (e.g. "order.created"); Payload is a small JSON summary
// the dashboard renders and links from.
type Event struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	At      time.Time      `json:"at"`
	Payload map[string]any `json:"payload,omitempty"`
}

// Publisher is the producer-facing surface. Domain handlers hold a Publisher
// (nil when realtime is disabled) and never import the Hub directly.
type Publisher interface {
	Publish(Event)
}
