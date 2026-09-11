package scanlistener

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"pgregory.net/rapid"
)

// Feature: background-scan-listener, Property 1: Empty-line discard rule
// **Validates: Requirements 1.2, 1.4**
//
// For any line read from standard input, if the line is empty, the
// ScanListener discards it, creates no scan entry, and leaves Current_Mode
// unchanged; if the line is non-empty, its full content is treated as the
// assembled barcode value passed on to classification.
func TestProperty1_EmptyLineDiscardRule(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		line := rapid.String().Draw(rt, "line")

		queue := &fakeQueue{}
		lookup := &fakeLookupService{result: product.LookupResult{}}
		l := &ScanListener{
			StockInBarcode:  "STOCK_IN",
			StockOutBarcode: "STOCK_OUT",
			Queue:           queue,
			LookupService:   lookup,
		}
		mode := newModeState()
		startMode := mode.get()

		l.handleLine(context.Background(), line, mode)

		if line == "" {
			if len(queue.created) != 0 {
				rt.Fatalf("empty line: want no scan entry created, got %d", len(queue.created))
			}
			if got := mode.get(); got != startMode {
				rt.Fatalf("empty line: mode changed from %v to %v", startMode, got)
			}
			return
		}

		// Non-empty, non-control lines are treated as a Product_Barcode and
		// create exactly one entry.
		if _, isControl := classify(line, l.StockInBarcode, l.StockOutBarcode); !isControl {
			if len(queue.created) != 1 {
				rt.Fatalf("non-empty non-control line %q: want exactly 1 scan entry created, got %d", line, len(queue.created))
			}
			if queue.created[0].Barcode != line {
				rt.Fatalf("created entry barcode = %q, want %q", queue.created[0].Barcode, line)
			}
		}
	})
}

// fakeQueue is a minimal in-memory stand-in for scan.Queue, recording every
// entry passed to CreateScanEntry.
type fakeQueue struct {
	created []scan.ScanEntry
	err     error
}

func (f *fakeQueue) CreateScanEntry(ctx context.Context, entry scan.ScanEntry) (*scan.ScanEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = append(f.created, entry)
	return &entry, nil
}

// fakeLookupService is a minimal stand-in for product.LookupService, always
// returning the configured result/error regardless of barcode.
type fakeLookupService struct {
	result product.LookupResult
	err    error
}

func (f *fakeLookupService) Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error) {
	return f.result, f.err
}

// Feature: background-scan-listener, Property 2: Mode transitions and Control_Barcode vs Product_Barcode classification
// **Validates: Requirements 2.2, 2.3, 2.4, 2.5, 2.6**
//
// For any starting Current_Mode and any sequence of lines where each one is
// either the configured stock-in Control_Barcode, the configured stock-out
// Control_Barcode, or any other non-empty value (a Product_Barcode): every
// occurrence of the stock-in Control_Barcode sets Current_Mode to stock_in
// and creates no scan entry; every occurrence of the stock-out
// Control_Barcode sets Current_Mode to stock_out and creates no scan entry;
// every Product_Barcode creates exactly one scan entry whose direction
// equals whichever Current_Mode was in effect immediately before that line
// was processed, without itself changing Current_Mode; and after the full
// sequence, Current_Mode equals the mode of the last Control_Barcode
// scanned, or stock_in if the sequence contained no Control_Barcode.
//
// ScanListener.Run always starts a fresh modeState defaulting to stock_in
// (Requirement 2.5) — there is no way to inject a different starting mode
// through the public ScanListener struct, so this exercises the real
// default start state rather than an arbitrary injected one. A guaranteed
// sentinel Product_Barcode is appended after the generated sequence; the
// direction recorded on its created entry reveals the mode in effect after
// the full sequence, letting this test assert the "final mode" clause
// without reaching into Run's private modeState.
func TestProperty2_ModeTransitionsAndClassification(t *testing.T) {
	const (
		stockInBarcode  = "STOCK_IN"
		stockOutBarcode = "STOCK_OUT"
		sentinelBarcode = "SENTINEL_FINAL_CHECK_BARCODE"
	)

	rapid.Check(t, func(rt *rapid.T) {
		// Each line is the stock-in barcode, the stock-out barcode, or an
		// arbitrary non-empty Product_Barcode that cannot collide with
		// either control barcode or the sentinel.
		lineGen := rapid.OneOf(
			rapid.Just(stockInBarcode),
			rapid.Just(stockOutBarcode),
			rapid.StringN(1, -1, 100).Filter(func(s string) bool {
				// Exclude \r and \n: bufio.Scanner's line splitting treats
				// them as line terminators, so a barcode containing one
				// would not round-trip as a single line through Stdin.
				return s != stockInBarcode && s != stockOutBarcode && s != sentinelBarcode &&
					!strings.ContainsAny(s, "\r\n")
			}),
		)
		lines := rapid.SliceOfN(lineGen, 0, 20).Draw(rt, "lines")

		// Simulate the same classify() rule ScanListener.Run applies, to
		// compute the expected created entries and the expected final mode.
		mode := scan.StockIn // Requirement 2.5: default start mode
		type expectedEntry struct {
			barcode   string
			direction scan.ScanDirection
		}
		var expected []expectedEntry
		for _, line := range lines {
			if newMode, isControl := classify(line, stockInBarcode, stockOutBarcode); isControl {
				mode = newMode
				continue
			}
			expected = append(expected, expectedEntry{barcode: line, direction: mode})
		}
		expectedFinalMode := mode

		queue := &fakeQueue{}
		allLines := append(append([]string{}, lines...), sentinelBarcode)
		l := &ScanListener{
			StockInBarcode:  stockInBarcode,
			StockOutBarcode: stockOutBarcode,
			Queue:           queue,
			LookupService:   &fakeLookupService{result: product.LookupResult{}},
			Stdin:           strings.NewReader(strings.Join(allLines, "\n") + "\n"),
		}

		l.Run(context.Background())

		if len(queue.created) != len(expected)+1 {
			rt.Fatalf("created entries = %d, want %d (product barcodes) + 1 (sentinel)", len(queue.created), len(expected)+1)
		}

		for i, want := range expected {
			got := queue.created[i]
			if got.Barcode != want.barcode {
				rt.Fatalf("entry %d: barcode = %q, want %q", i, got.Barcode, want.barcode)
			}
			if got.Direction == nil || *got.Direction != want.direction {
				rt.Fatalf("entry %d (barcode %q): direction = %v, want %v", i, want.barcode, got.Direction, want.direction)
			}
		}

		sentinelEntry := queue.created[len(queue.created)-1]
		if sentinelEntry.Barcode != sentinelBarcode {
			rt.Fatalf("final entry barcode = %q, want sentinel %q", sentinelEntry.Barcode, sentinelBarcode)
		}
		if sentinelEntry.Direction == nil || *sentinelEntry.Direction != expectedFinalMode {
			rt.Fatalf("final mode = %v, want %v", sentinelEntry.Direction, expectedFinalMode)
		}
	})
}

// Feature: background-scan-listener, Property 3: Headless entries follow the shared lookup-based creation rule
// **Validates: Requirements 3.1**
//
// For any Product_Barcode and any product-lookup result, the scan entry the
// ScanListener creates has ProductID set to the found product's ID when the
// lookup result is found and nil when it is not found, has Status set to
// Pending when found and Flagged when not found, and has UnitCount equal to
// 1 regardless of the lookup result.
func TestProperty3_HeadlessEntriesFollowLookupCreationRule(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		barcode := rapid.String().Draw(rt, "barcode")
		direction := rapid.SampledFrom([]scan.ScanDirection{scan.StockIn, scan.StockOut}).Draw(rt, "direction")
		scannedAt := time.Unix(rapid.Int64Range(0, time.Now().Unix()).Draw(rt, "scannedAtUnix"), 0)

		found := rapid.Bool().Draw(rt, "found")
		var lookup product.LookupResult
		if found {
			productID := rapid.StringN(1, -1, 50).Draw(rt, "productID")
			lookup = product.LookupResult{Product: &product.ProductSummary{ID: productID}}
		}

		entry := scan.NewEntryFromLookup("user-1", barcode, lookup, &direction, scannedAt)

		if found {
			if entry.ProductID == nil || *entry.ProductID != lookup.Product.ID {
				rt.Fatalf("ProductID = %v, want %q", entry.ProductID, lookup.Product.ID)
			}
			if entry.Status != scan.Pending {
				rt.Fatalf("Status = %v, want %v", entry.Status, scan.Pending)
			}
		} else {
			if entry.ProductID != nil {
				rt.Fatalf("ProductID = %v, want nil", *entry.ProductID)
			}
			if entry.Status != scan.Flagged {
				rt.Fatalf("Status = %v, want %v", entry.Status, scan.Flagged)
			}
		}

		if entry.UnitCount != 1 {
			rt.Fatalf("UnitCount = %d, want 1", entry.UnitCount)
		}
	})
}

// Feature: background-scan-listener, Property 4: Structural identity with HTTP-created entries
// **Validates: Requirements 3.4**
//
// For any barcode, product-lookup result, and direction, the scan entry
// scan.NewEntryFromLookup produces for the ScanListener's headless path is
// identical, field for field, to the scan entry the same helper produces for
// the HTTP ScanCreateHandler path given the same barcode, lookup result, and
// direction — except for the UserID field, which differs by construction
// (Headless_User_ID vs. the HTTP-supplied user ID).
func TestProperty4_StructuralIdentityWithHTTPCreatedEntries(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		barcode := rapid.String().Draw(rt, "barcode")
		direction := rapid.SampledFrom([]scan.ScanDirection{scan.StockIn, scan.StockOut}).Draw(rt, "direction")
		scannedAt := time.Unix(rapid.Int64Range(0, time.Now().Unix()).Draw(rt, "scannedAtUnix"), 0)

		found := rapid.Bool().Draw(rt, "found")
		var lookup product.LookupResult
		if found {
			productID := rapid.StringN(1, -1, 50).Draw(rt, "productID")
			lookup = product.LookupResult{Product: &product.ProductSummary{ID: productID}}
		}

		headlessUserID := rapid.StringN(1, -1, 30).Draw(rt, "headlessUserID")
		httpUserID := rapid.StringN(1, -1, 30).Filter(func(s string) bool {
			return s != headlessUserID
		}).Draw(rt, "httpUserID")

		headlessEntry := scan.NewEntryFromLookup(headlessUserID, barcode, lookup, &direction, scannedAt)
		httpEntry := scan.NewEntryFromLookup(httpUserID, barcode, lookup, &direction, scannedAt)

		if headlessEntry.UserID == httpEntry.UserID {
			rt.Fatalf("expected distinct UserID values by construction, got %q for both", headlessEntry.UserID)
		}

		// Zero out UserID so every remaining field is compared exactly.
		headlessEntry.UserID = ""
		httpEntry.UserID = ""
		if !reflect.DeepEqual(headlessEntry, httpEntry) {
			rt.Fatalf("entries differ beyond UserID:\nheadless=%+v\nhttp=%+v", headlessEntry, httpEntry)
		}
	})
}

// Feature: background-scan-listener, Property 5: Headless user attribution and default fallback
// **Validates: Requirements 4.1, 4.2**
//
// For any configured Headless_User_ID value, the scan entry the ScanListener
// creates for a Product_Barcode has UserID equal to that configured value
// whenever it is non-empty, and has UserID equal to the existing
// single-user default identifier ("user-1") whenever the configured value
// is empty or was never set.
func TestProperty5_HeadlessUserAttributionAndDefaultFallback(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		headlessUserID := rapid.OneOf(
			rapid.Just(""),
			rapid.StringN(1, -1, 30),
		).Draw(rt, "headlessUserID")
		barcode := rapid.StringN(1, -1, 50).Draw(rt, "barcode")

		queue := &fakeQueue{}
		l := &ScanListener{
			StockInBarcode:  "STOCK_IN",
			StockOutBarcode: "STOCK_OUT",
			HeadlessUserID:  headlessUserID,
			Queue:           queue,
			LookupService:   &fakeLookupService{result: product.LookupResult{}},
		}

		l.createEntry(context.Background(), barcode, scan.StockIn)

		if len(queue.created) != 1 {
			rt.Fatalf("created entries = %d, want 1", len(queue.created))
		}

		wantUserID := headlessUserID
		if wantUserID == "" {
			wantUserID = "user-1"
		}
		if queue.created[0].UserID != wantUserID {
			rt.Fatalf("UserID = %q, want %q", queue.created[0].UserID, wantUserID)
		}
	})
}
