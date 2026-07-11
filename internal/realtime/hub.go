package realtime

import (
	"strconv"
	"sync"
	"time"
)

// subscriber is one connected SSE client. The buffered channel absorbs short
// bursts; if a client falls far enough behind that the buffer fills, further
// events are dropped for it (it refetches the dashboard on reconnect, so the
// view self-heals — we never block a producer on a slow reader).
type subscriber struct {
	ch chan Event
}

// Hub is the in-memory event fan-out. It implements Publisher.
type Hub struct {
	mu     sync.Mutex
	subs   map[*subscriber]struct{}
	seq    uint64
	done   chan struct{}
	closed bool
}

// NewHub returns an empty Hub ready to accept subscribers and events.
func NewHub() *Hub {
	return &Hub{subs: make(map[*subscriber]struct{}), done: make(chan struct{})}
}

// Publish fans an event out to every current subscriber. It never blocks: a full
// subscriber buffer means that event is dropped for that subscriber only.
func (h *Hub) Publish(evt Event) {
	if evt.At.IsZero() {
		evt.At = time.Now().UTC()
	}
	h.mu.Lock()
	if evt.ID == "" {
		h.seq++
		evt.ID = strconv.FormatUint(h.seq, 10)
	}
	targets := make([]*subscriber, 0, len(h.subs))
	for s := range h.subs {
		targets = append(targets, s)
	}
	h.mu.Unlock()

	for _, s := range targets {
		select {
		case s.ch <- evt:
		default: // slow consumer — drop and move on
		}
	}
}

func (h *Hub) subscribe() *subscriber {
	s := &subscriber{ch: make(chan Event, 32)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// unsubscribe removes s. We deliberately never close s.ch: Publish only ever
// sends to subscribers still in the map (snapshotted under the lock), so once
// removed a subscriber receives no further sends and is garbage-collected.
func (h *Hub) unsubscribe(s *subscriber) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
}

// Done is closed by Close; SSE handlers select on it to end promptly on shutdown.
func (h *Hub) Done() <-chan struct{} { return h.done }

// Close signals all open streams to end. Safe to call more than once.
func (h *Hub) Close() {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		close(h.done)
	}
	h.mu.Unlock()
}
