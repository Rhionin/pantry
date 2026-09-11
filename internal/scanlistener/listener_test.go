package scanlistener

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

// selectiveErrQueue is a fakeQueue variant whose CreateScanEntry fails only
// for a configured barcode, letting a test observe that ScanListener.Run
// keeps processing subsequent lines after that failure (Requirement 3.5).
type selectiveErrQueue struct {
	errFor  string
	created []scan.ScanEntry
}

func (q *selectiveErrQueue) CreateScanEntry(ctx context.Context, entry scan.ScanEntry) (*scan.ScanEntry, error) {
	if entry.Barcode == q.errFor {
		return nil, errors.New("create scan entry failed")
	}
	q.created = append(q.created, entry)
	return &entry, nil
}

// selectiveErrLookup is a fakeLookupService variant whose Lookup fails only
// for a configured barcode, letting a test observe that ScanListener.Run
// keeps processing subsequent lines after a lookup failure.
type selectiveErrLookup struct {
	errFor string
	result product.LookupResult
}

func (l *selectiveErrLookup) Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error) {
	if barcode == l.errFor {
		return product.LookupResult{}, errors.New("lookup failed")
	}
	return l.result, nil
}

func TestScanListenerRun_EOFProcessesEveryAvailableLine(t *testing.T) {
	// Requirement 1.3: Run reads every line already available on Stdin and
	// returns cleanly once it hits EOF, without hanging or reporting an
	// error.
	queue := &fakeQueue{}
	l := &ScanListener{
		StockInBarcode:  "STOCK_IN",
		StockOutBarcode: "STOCK_OUT",
		Queue:           queue,
		LookupService:   &fakeLookupService{result: product.LookupResult{}},
		Stdin:           strings.NewReader("111\n222\n"),
	}

	l.Run(context.Background())

	if len(queue.created) != 2 {
		t.Fatalf("created entries = %d, want 2", len(queue.created))
	}
	if queue.created[0].Barcode != "111" || queue.created[1].Barcode != "222" {
		t.Errorf("created barcodes = %q, %q, want 111, 222", queue.created[0].Barcode, queue.created[1].Barcode)
	}
}

func TestScanListenerRun_BlankLineCreatesNoEntryAndKeepsMode(t *testing.T) {
	// Requirement 1.4: a blank line is discarded, creating no entry and
	// leaving the mode unchanged for the barcode that follows.
	queue := &fakeQueue{}
	l := &ScanListener{
		StockInBarcode:  "STOCK_IN",
		StockOutBarcode: "STOCK_OUT",
		Queue:           queue,
		LookupService:   &fakeLookupService{result: product.LookupResult{}},
		Stdin:           strings.NewReader("111\n\n222\n"),
	}

	l.Run(context.Background())

	if len(queue.created) != 2 {
		t.Fatalf("created entries = %d, want 2 (blank line must not create one)", len(queue.created))
	}
	if queue.created[0].Direction == nil || queue.created[1].Direction == nil {
		t.Fatal("expected both entries to have a direction set")
	}
	if *queue.created[0].Direction != *queue.created[1].Direction {
		t.Errorf("direction changed across blank line: %v vs %v", *queue.created[0].Direction, *queue.created[1].Direction)
	}
	if *queue.created[0].Direction != scan.StockIn {
		t.Errorf("direction = %v, want default %v", *queue.created[0].Direction, scan.StockIn)
	}
}

func TestScanListenerRun_ControlBarcodeSetsModeForNextProductBarcode(t *testing.T) {
	// A control-barcode line updates mode and creates no entry; a
	// subsequent product-barcode line creates an entry with that mode.
	queue := &fakeQueue{}
	l := &ScanListener{
		StockInBarcode:  "STOCK_IN",
		StockOutBarcode: "STOCK_OUT",
		Queue:           queue,
		LookupService:   &fakeLookupService{result: product.LookupResult{}},
		Stdin:           strings.NewReader("STOCK_OUT\n555\n"),
	}

	l.Run(context.Background())

	if len(queue.created) != 1 {
		t.Fatalf("created entries = %d, want 1 (control barcode must not create one)", len(queue.created))
	}
	entry := queue.created[0]
	if entry.Barcode != "555" {
		t.Errorf("barcode = %q, want %q", entry.Barcode, "555")
	}
	if entry.Direction == nil || *entry.Direction != scan.StockOut {
		t.Errorf("direction = %v, want %v", entry.Direction, scan.StockOut)
	}
}

func TestScanListenerRun_LookupErrorSkipsEntryButKeepsReading(t *testing.T) {
	// fakeLookupService.Lookup returning an error for a Product_Barcode
	// creates no entry for that barcode, and Run keeps reading subsequent
	// lines.
	queue := &fakeQueue{}
	l := &ScanListener{
		StockInBarcode:  "STOCK_IN",
		StockOutBarcode: "STOCK_OUT",
		Queue:           queue,
		LookupService:   &selectiveErrLookup{errFor: "BADCODE", result: product.LookupResult{}},
		Stdin:           strings.NewReader("BADCODE\nGOOD\n"),
	}

	l.Run(context.Background())

	if len(queue.created) != 1 {
		t.Fatalf("created entries = %d, want 1 (BADCODE must not create one)", len(queue.created))
	}
	if queue.created[0].Barcode != "GOOD" {
		t.Errorf("barcode = %q, want %q", queue.created[0].Barcode, "GOOD")
	}
}

func TestScanListenerRun_QueueErrorNotSurfacedAndLaterBarcodeStillCreatesEntry(t *testing.T) {
	// Requirement 3.5: fakeQueue.CreateScanEntry returning an error is not
	// surfaced from Run, and a later barcode in the same input still
	// creates an entry.
	queue := &selectiveErrQueue{errFor: "BADCODE"}
	l := &ScanListener{
		StockInBarcode:  "STOCK_IN",
		StockOutBarcode: "STOCK_OUT",
		Queue:           queue,
		LookupService:   &fakeLookupService{result: product.LookupResult{}},
		Stdin:           strings.NewReader("BADCODE\nGOOD\n"),
	}

	l.Run(context.Background())

	if len(queue.created) != 1 {
		t.Fatalf("created entries = %d, want 1 (BADCODE must not create one)", len(queue.created))
	}
	if queue.created[0].Barcode != "GOOD" {
		t.Errorf("barcode = %q, want %q", queue.created[0].Barcode, "GOOD")
	}
}
