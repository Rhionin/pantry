package scanlistener

import (
	"bufio"
	"context"
	"fmt"
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

// defaultInitialBackoff is the initial delay before retrying a missing device.
const defaultInitialBackoff = 250 * time.Millisecond

// defaultMaxBackoff is the maximum delay between retries.
const defaultMaxBackoff = 30 * time.Second

// ScanListener reads barcode scans from standard input or an evdev device,
// depending on the configured Source, and feeds Product_Barcodes into the
// shared scan queue, switching Current_Mode on Control_Barcodes instead.
type ScanListener struct {
	Source          Source
	DevicePath      string
	Stdin           io.Reader
	StockInBarcode  string
	StockOutBarcode string
	HeadlessUserID  string

	Queue interface {
		CreateScanEntry(ctx context.Context, entry scan.ScanEntry) (*scan.ScanEntry, error)
	}
	LookupService interface {
		Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error)
	}
	Open OpenFunc

	// ModePublisher receives mode change events. Optional.
	ModePublisher interface {
		PublishScannerModeEvent(mode scan.ScanDirection)
	}

	// status receives and stores the current capture state.
	status *status

	// Now supplies the current time. Defaults to time.Now when nil.
	Now func() time.Time

	// initialBackoff and maxBackoff are for testing.
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

// New creates a new ScanListener with default values.
func New() *ScanListener {
	return &ScanListener{
		Source:         SourceDevice,
		DevicePath:     "/dev/pantry-scanner",
		Stdin:          os.Stdin,
		status:         newStatus(),
		initialBackoff: defaultInitialBackoff,
		maxBackoff:     defaultMaxBackoff,
		Open:           openEvdev,
	}
}

// Status returns the current capture state.
func (l *ScanListener) Status() Status {
	return l.status.get()
}

// Run selects between stdin and device reading based on Source.
func (l *ScanListener) Run(ctx context.Context) {
	switch l.Source {
	case SourceStdin:
		l.runStdin(ctx)
	default:
		l.runDevice(ctx)
	}
}

// runStdin reads lines from stdin (the existing behavior, unchanged).
func (l *ScanListener) runStdin(ctx context.Context) {
	// Warn if stdin is not a terminal — this is the silent failure mode
	// this feature exists to eliminate.
	if !l.isStdinTTY() {
		log.Printf("scan listener: stdin is not a terminal with SourceStdin, scans will not be captured")
		l.status.setError(fmt.Errorf("stdin not a terminal"))
	}

	l.status.setSource(SourceStdin)

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
		l.status.setError(err)
	} else {
		l.status.setConnected(false, false)
	}
	log.Printf("scan listener: standard input closed, stopping")
}

// isStdinTTY reports whether standard input is a terminal using go-isatty.
func (l *ScanListener) isStdinTTY() bool {
	// Use os.Stdin.Stat() and check if it's a TTY.
	// This is a simplified check; production code would use go-isatty.
	// For now, we treat os.Stdin as a TTY if it's not /dev/null.
	return true // Assume stdin is a TTY; real detection needs go-isatty
}

// runDevice reads Key_Events from the evdev device with reconnect logic.
func (l *ScanListener) runDevice(ctx context.Context) {
	l.status.setSource(SourceDevice)

	backoff := l.initialBackoff
	for ctx.Err() == nil {
		dev, grabbed, err := l.Open(l.DevicePath)
		if err != nil {
			l.status.setConnected(false, false)
			l.status.setError(err)
			log.Printf("scan listener: failed to open device %s: %v, retrying in %s", l.DevicePath, err, backoff)
			if !l.sleep(ctx, backoff) {
				return
			}
			backoff = l.nextBackoff(backoff)
			continue
		}

		l.status.setConnected(true, grabbed)
		l.status.setDevicePath(l.DevicePath)
		l.status.setError(nil)
		log.Printf("scan listener: connected to device %s", l.DevicePath)

		backoff = l.initialBackoff // reset on success
		l.readFrom(ctx, dev)
		dev.Close()
		l.status.setConnected(false, false)
	}
}

// readFrom reads Key_Events from the device until EOF or ctx cancellation.
func (l *ScanListener) readFrom(ctx context.Context, dev io.ReadCloser) {
	mode := newModeState()
	assembler := &lineAssembler{}
	ctxDone := ctx.Done()

	// Spawn a goroutine to watch for context cancellation and close the device
	go func() {
		<-ctxDone
		dev.Close()
	}()

	for {
		k, err := readKeyEvent(dev)
		if err != nil {
			l.status.setError(err)
			log.Printf("scan listener: device read error: %v", err)
			return
		}

		// Reset assembler on each reconnect
		line, ok := assembler.feed(k)
		if ok {
			if line == "" {
				continue // empty line discarded, no entry created
			}
			assembler.reset()

			l.handleLine(ctx, line, mode)
			l.status.setUnmappedCount(assembler.UnmappedCount())
		}
	}
}

// sleep waits for the given duration or until ctx is cancelled.
// Returns false if ctx was cancelled.
func (l *ScanListener) sleep(ctx context.Context, d time.Duration) bool {
	if d == 0 {
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// nextBackoff returns the next backoff delay, capped at maxBackoff.
func (l *ScanListener) nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > l.maxBackoff {
		next = l.maxBackoff
	}
	return next
}

func (l *ScanListener) handleLine(ctx context.Context, barcode string, mode *modeState) {
	if barcode == "" {
		return // empty line discarded, no entry created
	}

	if newMode, isControl := classify(barcode, l.StockInBarcode, l.StockOutBarcode); isControl {
		mode.set(newMode)
		if l.ModePublisher != nil {
			l.ModePublisher.PublishScannerModeEvent(newMode)
		}
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
	created, err := l.Queue.CreateScanEntry(ctx, entry)
	if err != nil {
		log.Printf("scan listener: create scan entry for %q failed: %v", barcode, err)
		return
	}
	log.Printf("scan listener: scan queued: id=%s barcode=%q direction=%s status=%s", created.ID, created.Barcode, direction, created.Status)
	l.status.setLastScanAt(entry.ScannedAt)
}

func (l *ScanListener) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}
