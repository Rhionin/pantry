package push

import (
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

type fakeClock struct {
	now    time.Duration
	next   int
	timers map[int]*fakeTimer
}

type fakeTimer struct {
	at time.Duration
	fn func()
}

func newFakeClock() *fakeClock {
	return &fakeClock{timers: map[int]*fakeTimer{}}
}

func (c *fakeClock) After(delay time.Duration, fn func()) func() {
	c.next++
	id := c.next
	c.timers[id] = &fakeTimer{at: c.now + delay, fn: fn}
	return func() { delete(c.timers, id) }
}

func (c *fakeClock) advance(d time.Duration) {
	c.now += d
	for {
		var dueID int
		var due *fakeTimer
		for id, timer := range c.timers {
			if timer.at > c.now {
				continue
			}
			if due == nil || timer.at < due.at || (timer.at == due.at && id < dueID) {
				due = timer
				dueID = id
			}
		}
		if due == nil {
			return
		}
		delete(c.timers, dueID)
		due.fn()
	}
}

func strPtr(value string) *string { return &value }

func dirPtr(value scan.ScanDirection) *scan.ScanDirection { return &value }

func pending(id, barcode string, direction scan.ScanDirection) scan.ScanEntry {
	return scan.ScanEntry{
		ID:        id,
		Barcode:   barcode,
		Status:    scan.Pending,
		Direction: dirPtr(direction),
		ProductID: strPtr("product-1"),
	}
}

func harness() (*attention, *fakeClock, *[]Notice) {
	notices := []Notice{}
	clock := newFakeClock()
	alerts := newAttention(func(notice Notice) {
		notices = append(notices, notice)
	}, BatchSettle, clock)
	return alerts, clock, &notices
}

func TestAttentionFailureAndUnrecognized(t *testing.T) {
	alerts, _, notices := harness()
	failure := scan.ProcessingFailure{ID: "lookup-1", Barcode: "999", Message: "Couldn't look up that barcode."}
	alerts.noteFailure(failure)
	alerts.noteFailure(failure)
	flagged := scan.ScanEntry{
		ID: "flag-1", Barcode: "000111", Status: scan.Flagged, Direction: nil, ProductID: nil,
	}
	alerts.noteScan(flagged)
	alerts.noteScan(flagged)
	if len(*notices) != 2 {
		t.Fatalf("notices = %#v", *notices)
	}
	got := (*notices)[0]
	if got.Title != "Scan failed" || got.URL != "/" || got.Tag != "pantry-scan-failure-lookup-1" {
		t.Fatalf("failure notice = %#v", got)
	}
	if got.Body != "Couldn't look up that barcode. Open the scan queue to try again." {
		t.Fatalf("failure notice = %#v", got)
	}
	if (*notices)[1].Title != "Unrecognized product" || (*notices)[1].URL != "/?scanAlert=stock_out" {
		t.Fatalf("unrecognized notice = %#v", (*notices)[1])
	}
	if (*notices)[1].Body != "Barcode 000111 needs a product. Open the scan queue to match it." {
		t.Fatalf("body = %q", (*notices)[1].Body)
	}
}

func TestAttentionBatchWaitsOutTheQuietPeriod(t *testing.T) {
	alerts, clock, notices := harness()
	alerts.noteScan(pending("milk", "111", scan.StockIn))
	clock.advance(4 * time.Minute)
	alerts.noteScan(pending("oats", "222", scan.StockIn))
	clock.advance(4 * time.Minute)
	if len(*notices) != 0 {
		t.Fatalf("batch fired early: %#v", *notices)
	}
	clock.advance(time.Minute)
	if len(*notices) != 1 {
		t.Fatalf("notices = %#v", *notices)
	}
	got := (*notices)[0]
	if got.Title != "Stock in batch is ready" || got.URL != "/?scanAlert=stock_in" {
		t.Fatalf("batch = %#v", got)
	}
	if got.Body != "2 scans are waiting. Open the scan queue to confirm them." {
		t.Fatalf("body = %q", got.Body)
	}
	if got.Tag != "pantry-scan-batch-stock_in" {
		t.Fatalf("tag = %q", got.Tag)
	}
}

func TestAttentionDirectionsStaySeparate(t *testing.T) {
	alerts, clock, notices := harness()
	alerts.noteScan(pending("in", "111", scan.StockIn))
	clock.advance(3 * time.Minute)
	alerts.noteScan(pending("out", "222", scan.StockOut))
	clock.advance(2 * time.Minute)
	if len(*notices) != 1 || (*notices)[0].URL != "/?scanAlert=stock_in" {
		t.Fatalf("after stock in settled: %#v", *notices)
	}
	clock.advance(3 * time.Minute)
	if len(*notices) != 2 || (*notices)[1].Title != "Stock out batch is ready" {
		t.Fatalf("notices = %#v", *notices)
	}
	if (*notices)[1].Body != "1 scan is waiting. Open the scan queue to confirm it." {
		t.Fatalf("body = %q", (*notices)[1].Body)
	}
}

func TestAttentionConfirmedScansLeaveTheBatch(t *testing.T) {
	alerts, clock, notices := harness()
	alerts.noteScan(pending("milk", "111", scan.StockIn))
	alerts.noteScan(pending("oats", "222", scan.StockIn))
	done := pending("milk", "111", scan.StockIn)
	done.Status = scan.Committed
	alerts.noteScan(done)
	cancelled := pending("oats", "222", scan.StockIn)
	cancelled.Status = scan.Cancelled
	alerts.noteScan(cancelled)
	clock.advance(BatchSettle)
	if len(*notices) != 0 {
		t.Fatalf("empty batch notified: %#v", *notices)
	}
}

func TestAttentionBatchCarriesProductNames(t *testing.T) {
	alerts, clock, notices := harness()
	milk := pending("milk", "111", scan.StockOut)
	milk.Product = &product.ProductSummary{Name: "Whole Milk"}
	bread := pending("bread", "222", scan.StockOut)
	bread.Product = &product.ProductSummary{Name: "Rye Bread"}
	alerts.noteScan(milk)
	alerts.noteScan(bread)
	clock.advance(BatchSettle)
	if len(*notices) != 1 {
		t.Fatalf("notices = %#v", *notices)
	}
	got := (*notices)[0]
	if got.Count != 2 || len(got.Names) != 2 || got.Names[0] != "Whole Milk" || got.Names[1] != "Rye Bread" {
		t.Fatalf("names = %#v count %d", got.Names, got.Count)
	}
	if got.Title != "Stock out batch is ready" || got.URL != "/?scanAlert=stock_out" {
		t.Fatalf("notice = %#v", got)
	}
	if got.Body != "2 scans are waiting. Open the scan queue to confirm them." {
		t.Fatalf("body = %q", got.Body)
	}
}

func TestAttentionStopsOnDispose(t *testing.T) {
	alerts, clock, notices := harness()
	alerts.noteScan(pending("milk", "111", scan.StockIn))
	alerts.stop()
	clock.advance(BatchSettle)
	if len(*notices) != 0 {
		t.Fatalf("disposed batch notified: %#v", *notices)
	}
}

func TestAttentionFailureDoesNotResetTheBatch(t *testing.T) {
	alerts, clock, notices := harness()
	alerts.noteScan(pending("milk", "111", scan.StockIn))
	clock.advance(4 * time.Minute)
	alerts.noteFailure(scan.ProcessingFailure{ID: "lookup-1", Message: "Couldn't look up that barcode."})
	alerts.noteScan(scan.ScanEntry{ID: "flag", Barcode: "333", Status: scan.Flagged})
	clock.advance(time.Minute)
	if len(*notices) != 3 {
		t.Fatalf("notices = %#v", *notices)
	}
	if (*notices)[2].Body != "1 scan is waiting. Open the scan queue to confirm it." {
		t.Fatalf("batch body = %q", (*notices)[2].Body)
	}
}
