// Package events fans out live updates to connected UI clients.
package events

import "sync"

// Message is a live update pushed to the UI over Server-Sent Events.
type Message struct {
	// "result": a check finished (Data is a monitor.Result).
	// "status": a monitor changed state (Data is a monitor.Event).
	// "monitors": monitors were created, edited or deleted (no Data).
	Type      string `json:"type"`
	MonitorID int64  `json:"monitor_id"`
	Data      any    `json:"data"`
}

// Hub is an in-memory pub/sub. Slow subscribers drop messages rather than
// blocking the scheduler; the UI refetches on reconnect anyway.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan Message]struct{}
	closed bool
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[chan Message]struct{}{}} }

// Subscribe returns a channel of messages and a function to unsubscribe.
func (h *Hub) Subscribe() (<-chan Message, func()) {
	ch := make(chan Message, 64)
	h.mu.Lock()
	if h.closed {
		close(ch)
	} else {
		h.subs[ch] = struct{}{}
	}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Publish sends m to every subscriber without blocking.
func (h *Hub) Publish(m Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- m:
		default:
		}
	}
}

// Close ends every subscription, which makes open SSE streams return. Call it
// when the server starts shutting down: streams never finish on their own,
// so without this a graceful shutdown waits out its full timeout.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}
