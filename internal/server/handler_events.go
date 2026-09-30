package server

import (
	"fmt"
	"net/http"

	"github.com/Rhionin/pantry/internal/events"
)

type EventsHandler struct {
	Broadcaster *events.Broadcaster
}

// Handle implements GET /api/events (Requirement 1.1). It blocks for the
// lifetime of the connection, writing one "event: <type>\ndata: <json>\n\n"
// frame per published message until the client disconnects (Requirement 1.3)
// or a write to it fails (Requirement 1.4).
func (h *EventsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Subscribe before flushing the response headers. A client that observes
	// the 200 OK is then guaranteed to already be subscribed, so an event
	// published immediately after the client's request returns cannot slip
	// through the gap between headers being sent and the subscription being
	// registered.
	messages, unsubscribe := h.Broadcaster.Subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return // buffer overflow: Broadcaster already dropped us
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.EventType, msg.Data); err != nil {
				return // write failed: stop, defer unsubscribes us
			}
			flusher.Flush()
		case <-r.Context().Done():
			return // client disconnected (Requirement 1.3)
		}
	}
}
