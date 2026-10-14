package push

import (
	"encoding/json"
	"time"

	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/scan"
)

// announcedCap bounds ids remembered for the life of the process. Clearing
// the set can repeat an alert; holding every scan id cannot.
const announcedCap = 4096

type clock interface {
	After(delay time.Duration, fn func()) (cancel func())
}

type realClock struct{}

func (realClock) After(delay time.Duration, fn func()) func() {
	timer := time.AfterFunc(delay, fn)
	return func() { timer.Stop() }
}

type openBatch struct {
	ids    map[string]struct{}
	order  []string
	names  map[string]string
	cancel func()
}

func newBatch() *openBatch {
	return &openBatch{
		ids:   map[string]struct{}{},
		names: map[string]string{},
	}
}

// attention turns scan events into at most one notice per failure, one per
// unrecognized product, and one per in or out session after it goes quiet.
type attention struct {
	emit      func(Notice)
	clock     clock
	settle    time.Duration
	announced map[string]struct{}
	batches   map[scan.ScanDirection]*openBatch
}

func newAttention(emit func(Notice), settle time.Duration, clk clock) *attention {
	if clk == nil {
		clk = realClock{}
	}
	if settle <= 0 {
		settle = BatchSettle
	}
	return &attention{
		emit:      emit,
		clock:     clk,
		settle:    settle,
		announced: map[string]struct{}{},
		batches: map[scan.ScanDirection]*openBatch{
			scan.StockIn:  newBatch(),
			scan.StockOut: newBatch(),
		},
	}
}

func (a *attention) handle(msg events.Message) {
	switch msg.EventType {
	case "scan_processing_failed":
		var failure scan.ProcessingFailure
		if err := json.Unmarshal(msg.Data, &failure); err != nil {
			return
		}
		a.noteFailure(failure)
	case "scan":
		var entry scan.ScanEntry
		if err := json.Unmarshal(msg.Data, &entry); err != nil {
			return
		}
		a.noteScan(entry)
	}
}

func (a *attention) noteFailure(failure scan.ProcessingFailure) {
	key := "failure:" + failure.ID
	if failure.ID == "" {
		key = "failure:" + failure.Barcode + ":" + failure.Message
	}
	if !a.remember(key) {
		return
	}
	a.emit(failureNotice(failure))
}

func (a *attention) noteScan(entry scan.ScanEntry) {
	if entry.ID == "" {
		return
	}
	if entry.Status == scan.QR {
		a.drop(entry.ID)
		if !a.remember("qr:" + entry.ID) {
			return
		}
		a.emit(qrNotice(entry))
		return
	}
	if unrecognized(entry) {
		a.drop(entry.ID)
		if !a.remember("unrecognized:" + entry.ID) {
			return
		}
		a.emit(unrecognizedNotice(entry))
		return
	}
	if entry.Status == scan.Committed || entry.Status == scan.Cancelled {
		a.drop(entry.ID)
		return
	}
	direction, ok := batchDirection(entry)
	if !ok {
		return
	}
	a.place(entry.ID, productName(entry), direction)
}

func (a *attention) stop() {
	a.disarm(scan.StockIn)
	a.disarm(scan.StockOut)
}

func (a *attention) remember(key string) bool {
	if _, ok := a.announced[key]; ok {
		return false
	}
	if len(a.announced) >= announcedCap {
		a.announced = map[string]struct{}{}
	}
	a.announced[key] = struct{}{}
	return true
}

func unrecognized(entry scan.ScanEntry) bool {
	if entry.Status == scan.Flagged {
		return true
	}
	return entry.Status == scan.Pending && entry.ProductID == nil
}

func batchDirection(entry scan.ScanEntry) (scan.ScanDirection, bool) {
	if entry.Status != scan.Pending || entry.ProductID == nil || entry.Direction == nil {
		return "", false
	}
	if *entry.Direction != scan.StockIn && *entry.Direction != scan.StockOut {
		return "", false
	}
	return *entry.Direction, true
}

func (a *attention) place(id, name string, direction scan.ScanDirection) {
	other := scan.StockOut
	if direction == scan.StockOut {
		other = scan.StockIn
	}
	if _, ok := a.batches[other].ids[id]; ok {
		a.batches[other].remove(id)
		if len(a.batches[other].ids) == 0 {
			a.disarm(other)
		}
	}
	a.batches[direction].put(id, name)
	a.arm(direction)
}

func (a *attention) drop(id string) {
	for _, direction := range []scan.ScanDirection{scan.StockIn, scan.StockOut} {
		batch := a.batches[direction]
		if _, ok := batch.ids[id]; !ok {
			continue
		}
		batch.remove(id)
		if len(batch.ids) == 0 {
			a.disarm(direction)
		}
	}
}

func (b *openBatch) put(id, name string) {
	if _, ok := b.ids[id]; !ok {
		b.order = append(b.order, id)
	}
	b.ids[id] = struct{}{}
	if name != "" {
		b.names[id] = name
	}
}

func (b *openBatch) remove(id string) {
	delete(b.ids, id)
	delete(b.names, id)
	kept := b.order[:0]
	for _, existing := range b.order {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	b.order = kept
}

// take reports the scans still waiting and the product names in scan order,
// then forgets them so a later session starts empty.
func (b *openBatch) take() (int, []string) {
	count := len(b.ids)
	var names []string
	if len(b.names) > 0 {
		names = make([]string, 0, len(b.order))
		for _, id := range b.order {
			if name := b.names[id]; name != "" {
				names = append(names, name)
			}
		}
	}
	b.ids = map[string]struct{}{}
	b.names = map[string]string{}
	b.order = nil
	return count, names
}

func (a *attention) arm(direction scan.ScanDirection) {
	batch := a.batches[direction]
	a.disarm(direction)
	batch.cancel = a.clock.After(a.settle, func() {
		batch.cancel = nil
		count, names := batch.take()
		if count > 0 {
			a.emit(batchNotice(direction, count, names))
		}
	})
}

func (a *attention) disarm(direction scan.ScanDirection) {
	batch := a.batches[direction]
	if batch.cancel == nil {
		return
	}
	batch.cancel()
	batch.cancel = nil
}
