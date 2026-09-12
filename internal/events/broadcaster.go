// Package events provides an in-memory publish/subscribe broadcaster that
// delivers Scan_Events and Inventory_Events to every browser connected to
// GET /api/events. It has no persistence: a subscriber only ever receives
// events published after it subscribes (Requirement 1.2), and there is no
// buffering or replay for a client that reconnects after a gap
// (Requirement 6.2 is satisfied by this absence, not by extra bookkeeping).
package events

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/scan"
)

// subscriberBuffer bounds how many unread messages a single connection may
// accumulate before it is treated as unable to keep up and dropped, so one
// slow reader can never block delivery to the others (Requirement 1.4).
const subscriberBuffer = 16

// Message is one server-sent event: EventType becomes the SSE "event:" field
// ("scan" or "inventory") and Data is the JSON-encoded ScanEntry or
// InventoryItem that becomes the "data:" field.
type Message struct {
	EventType string
	Data      []byte
}

// Broadcaster fans out published events to every currently-subscribed
// connection. The zero value is not usable; construct with NewBroadcaster.
type Broadcaster struct {
	mu          sync.Mutex
	nextID      int64
	subscribers map[int64]chan Message
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subscribers: make(map[int64]chan Message)}
}

// Subscribe registers a new connection and returns the channel it should
// read published messages from, plus an unsubscribe function the caller
// MUST call exactly once when the connection ends (Requirement 1.3). The
// returned channel receives only messages published after Subscribe returns.
func (b *Broadcaster) Subscribe() (<-chan Message, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan Message, subscriberBuffer)
	b.subscribers[id] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subscribers, id)
	}
	return ch, unsubscribe
}

// PublishScanEvent delivers entry to every currently-subscribed connection.
func (b *Broadcaster) PublishScanEvent(entry scan.ScanEntry) {
	b.publish("scan", entry)
}

// PublishInventoryEvent delivers item to every currently-subscribed connection.
func (b *Broadcaster) PublishInventoryEvent(item inventory.InventoryItem) {
	b.publish("inventory", item)
}

func (b *Broadcaster) publish(eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("events: marshal %s event: %v", eventType, err)
		return
	}
	msg := Message{EventType: eventType, Data: data}

	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			// Subscriber's buffer is full: it cannot keep up. Drop it here
			// rather than block every other subscriber's delivery
			// (Requirement 1.4's "continue delivering ... to all other
			// connected clients"). EventsHandler treats a dropped/closed
			// subscriber the same way it treats a write error: the browser's
			// native EventSource reconnect (Requirement 6.1) recovers it.
			close(ch)
			delete(b.subscribers, id)
		}
	}
}
