package scanlistener

import (
	"bufio"
	"context"
	"io"
	"log"
	"os"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

// defaultHeadlessUserID matches the single-user default hardcoded across
// internal/server's handlers (const userID = "user-1").
const defaultHeadlessUserID = "user-1"

// ScanListener reads barcode scans as lines from standard input and feeds
// Product_Barcodes into the shared scan queue, switching Current_Mode on
// Control_Barcodes instead.
type ScanListener struct {
	StockInBarcode  string
	StockOutBarcode string
	HeadlessUserID  string

	Queue interface {
		CreateScanEntry(ctx context.Context, entry scan.ScanEntry) (*scan.ScanEntry, error)
	}
	LookupService interface {
		Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error)
	}

	// Stdin is the stream ScanListener reads lines from. Defaults to
	// os.Stdin; tests inject their own reader.
	Stdin io.Reader

	// Now supplies the current time. Defaults to time.Now when nil.
	Now func() time.Time
}

// Run reads lines from Stdin until it reaches EOF, hits a read error, or ctx
// is cancelled. It never returns an error: a closed or errored stdin is
// logged and Run returns, so the caller always launches it as a bare
// `go listener.Run(ctx)` with nothing to check.
func (l *ScanListener) Run(ctx context.Context) {
	stdin := l.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	log.Printf("scan listener: reading scans from standard input")

	mode := newModeState()
	scanner := bufio.NewScanner(stdin)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		l.handleLine(ctx, scanner.Text(), mode)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("scan listener: standard input read error, stopping: %v", err)
		return
	}
	log.Printf("scan listener: standard input closed, stopping")
}

func (l *ScanListener) handleLine(ctx context.Context, barcode string, mode *modeState) {
	if barcode == "" {
		return // empty line discarded, no entry created
	}

	if newMode, isControl := classify(barcode, l.StockInBarcode, l.StockOutBarcode); isControl {
		mode.set(newMode)
		return // no scan entry for a control barcode
	}

	l.createEntry(ctx, barcode, mode.get())
}

func (l *ScanListener) createEntry(ctx context.Context, barcode string, direction scan.ScanDirection) {
	userID := l.HeadlessUserID
	if userID == "" {
		userID = defaultHeadlessUserID
	}

	lookup, err := l.LookupService.Lookup(ctx, barcode, userID)
	if err != nil {
		log.Printf("scan listener: product lookup for %q failed: %v", barcode, err)
		return
	}

	entry := scan.NewEntryFromLookup(userID, barcode, lookup, &direction, l.now())
	if _, err := l.Queue.CreateScanEntry(ctx, entry); err != nil {
		log.Printf("scan listener: create scan entry for %q failed: %v", barcode, err)
		return
	}
}

func (l *ScanListener) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}
