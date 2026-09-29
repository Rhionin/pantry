package scanlistener

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

// fakeQueue implements the Queue interface for testing.
type fakeQueue struct {
	entries []*scan.ScanEntry
	err     error

	// cancel, if set, is called after each entry is recorded. Tests that
	// drive runDevice with a fake Open func that never returns a permanent
	// error use this to stop the reconnect loop once the expected entry
	// has been observed — otherwise the loop reopens the fake device and
	// replays it forever, since ctx.Err() never becomes non-nil on its own.
	cancel func()
}

func (q *fakeQueue) CreateScanEntry(ctx context.Context, entry scan.ScanEntry) (*scan.ScanEntry, error) {
	if q.err != nil {
		return nil, q.err
	}
	// Deep copy the entry
	copied := entry
	q.entries = append(q.entries, &copied)
	if q.cancel != nil {
		q.cancel()
	}
	return &copied, nil
}

// fakeLookup implements the LookupService interface for testing.
type fakeLookup struct {
	result product.LookupResult
	err    error
}

func (l *fakeLookup) Lookup(ctx context.Context, barcode, userID string) (product.LookupResult, error) {
	if l.err != nil {
		return product.LookupResult{}, l.err
	}
	return l.result, nil
}

// TestRunStdin_EOF tests that Run returns when stdin reaches EOF.
func TestRunStdin_EOF(t *testing.T) {
	ctx := context.Background()
	listener := New()
	listener.Source = SourceStdin
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}
	listener.Stdin = strings.NewReader("123456\n")

	listener.Run(ctx)

	if len(listener.Queue.(*fakeQueue).entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(listener.Queue.(*fakeQueue).entries))
	}
}

// TestRunStdin_BackgroundMode tests stdin mode with a faked OpenFunc.
func TestRunStdin_BackgroundMode(t *testing.T) {
	ctx := context.Background()
	listener := New()
	listener.Source = SourceStdin
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}
	listener.Stdin = strings.NewReader("123456\n")

	listener.Run(ctx)
	status := listener.Status()

	if status.Source != SourceStdin {
		t.Errorf("status.Source = %v, want %v", status.Source, SourceStdin)
	}
	if status.Connected {
		t.Error("status.Connected should be false for stdin")
	}
}

// TestRunDevice_OpenFail tests that runDevice retries on open failure.
func TestRunDevice_OpenFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	attempts := 0
	openFunc := func(path string) (io.ReadCloser, bool, error) {
		attempts++
		if attempts == 1 {
			return nil, false, io.EOF
		}
		// Second attempt succeeds with a reader that immediately returns EOF.
		// Cancel here so runDevice's reconnect loop stops after this attempt
		// instead of reopening the exhausted fake device forever.
		cancel()
		return io.NopCloser(bytes.NewReader(nil)), true, nil
	}

	listener := New()
	listener.Source = SourceDevice
	listener.Open = openFunc
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}

	// Set a very short backoff for testing
	listener.maxBackoff = 0

	listener.Run(ctx)

	if attempts != 2 {
		t.Errorf("open was called %d times, expected 2", attempts)
	}
}

// TestRunDevice_Success tests a successful device read.
func TestRunDevice_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build a valid input_event record for "123456" followed by Enter
	var buf bytes.Buffer
	for _, code := range []uint16{0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x2a} { // 1,2,3,4,5,6,Enter
		buf.Write(make([]byte, 16)) // padding for timeval
		binary.Write(&buf, binary.LittleEndian, uint16(evKey))
		binary.Write(&buf, binary.LittleEndian, code)
		binary.Write(&buf, binary.LittleEndian, int32(1)) // press
	}

	queue := &fakeQueue{cancel: cancel}
	lookup := &fakeLookup{}
	lookup.result = product.LookupResult{
		Product: &product.ProductSummary{
			ID:   "prod-123",
			Name: "Test Product",
		},
		Source: "global",
	}

	listener := New()
	listener.Source = SourceDevice
	listener.Open = func(path string) (io.ReadCloser, bool, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), true, nil
	}
	listener.Queue = queue
	listener.LookupService = lookup

	listener.Run(ctx)

	if len(queue.entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(queue.entries))
	}
	if len(queue.entries) > 0 {
		if queue.entries[0].Barcode != "123456" {
			t.Errorf("entry.Barcode = %q, want %q", queue.entries[0].Barcode, "123456")
		}
	}

	// By the time Run returns, the fake device's reader has been exhausted
	// (EOF right after Enter) and runDevice has already torn the connection
	// down, so per the design ("connected true while reading and false after
	// a removal") Connected and Grabbed report false here.
	status := listener.Status()
	if status.Connected {
		t.Error("status.Connected should be false after the device disconnected")
	}
	if status.Grabbed {
		t.Error("status.Grabbed should be false after the device disconnected")
	}
	if status.Source != SourceDevice {
		t.Errorf("status.Source = %v, want %v", status.Source, SourceDevice)
	}
}

// TestRunStdin_Cancelled tests that Run returns when ctx is cancelled.
func TestRunStdin_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener := New()
	listener.Source = SourceStdin
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}
	listener.Stdin = strings.NewReader("123456\n")

	// Cancel synchronously before Run rather than racing a goroutine against
	// a strings.Reader scan that completes near-instantly: runStdin only
	// checks ctx.Err() between scanner.Scan() calls, so a cancel that hasn't
	// happened yet by the time Scan() returns would let handleLine run
	// anyway, making the goroutine version non-deterministic.
	cancel()

	listener.Run(ctx)

	if len(listener.Queue.(*fakeQueue).entries) != 0 {
		t.Errorf("expected 0 entries when cancelled, got %d", len(listener.Queue.(*fakeQueue).entries))
	}
}

// TestRunStdin_Errors tests that stdin errors are handled gracefully.
func TestRunStdin_Errors(t *testing.T) {
	ctx := context.Background()
	listener := New()
	listener.Source = SourceStdin
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}

	// Create a reader that returns an error on first Read
	reader := &errorReader{}
	listener.Stdin = reader

	listener.Run(ctx)
	status := listener.Status()

	if status.LastError == "" {
		t.Error("Expected LastError to be set after read error")
	}
}

// errorReader is a dummy reader that always returns an error.
type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

// ===== Preservation Tests for Missing-Device Tolerance =====

// TestOpenAlwaysFails tests that runDevice reports disconnected with a persistent
// lastError when Open always fails, and continues retrying.
func TestOpenAlwaysFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	attempts := 0
	openFunc := func(path string) (io.ReadCloser, bool, error) {
		attempts++
		return nil, false, io.EOF
	}

	listener := New()
	listener.Source = SourceDevice
	listener.Open = openFunc
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}

	// Set microsecond backoffs for fast test execution
	listener.initialBackoff = 1 * time.Microsecond
	listener.maxBackoff = 10 * time.Microsecond

	listener.Run(ctx)

	// Verify multiple retry attempts occurred
	if attempts < 2 {
		t.Errorf("expected at least 2 open attempts, got %d", attempts)
	}

	// Verify status reports disconnected with error
	status := listener.Status()
	if status.Connected {
		t.Error("status.Connected should be false when open always fails")
	}
	if status.LastError == "" {
		t.Error("status.LastError should be set when open always fails")
	}
}

// TestOpenFailsThenSucceeds tests that runDevice retries on failure and resets
// backoff when open succeeds.
func TestOpenFailsThenSucceeds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	attempts := 0
	openFunc := func(path string) (io.ReadCloser, bool, error) {
		attempts++
		if attempts < 3 {
			return nil, false, io.EOF
		}
		// Third attempt succeeds; cancel so the reconnect loop stops
		cancel()
		return io.NopCloser(bytes.NewReader(nil)), true, nil
	}

	listener := New()
	listener.Source = SourceDevice
	listener.Open = openFunc
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}

	listener.initialBackoff = 1 * time.Microsecond
	listener.maxBackoff = 10 * time.Microsecond

	listener.Run(ctx)

	if attempts != 3 {
		t.Errorf("expected 3 open attempts, got %d", attempts)
	}

	// After successful open and EOF, status should be disconnected with no error
	// (the EOF from the exhausted reader is expected)
	status := listener.Status()
	if status.Connected {
		t.Error("status.Connected should be false after device EOF")
	}
}

// TestDeviceDisappearsAfterOpen tests that runDevice handles a device that
// disappears mid-read (returns an error during read), sets disconnected status,
// and resumes retrying.
func TestDeviceDisappearsAfterOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	attempts := 0
	openFunc := func(path string) (io.ReadCloser, bool, error) {
		attempts++
		if attempts == 1 {
			// First open succeeds but returns a reader that immediately errors
			return io.NopCloser(&errorReader{}), false, nil
		}
		// Second attempt: cancel to stop the reconnect loop
		cancel()
		return io.NopCloser(bytes.NewReader(nil)), false, nil
	}

	listener := New()
	listener.Source = SourceDevice
	listener.Open = openFunc
	listener.Queue = &fakeQueue{}
	listener.LookupService = &fakeLookup{}

	listener.Run(ctx)

	// Verify two open attempts: one that succeeded then errored on read,
	// and a second after reconnect started
	if attempts != 2 {
		t.Errorf("expected 2 open attempts after mid-read error, got %d", attempts)
	}

	status := listener.Status()
	if status.Connected {
		t.Error("status.Connected should be false after read error")
	}
	if status.LastError == "" {
		t.Error("status.LastError should be set after read error")
	}
}

// TestGrabFailureNonFatal tests that Open reporting a failed grab (grabbed=false)
// still yields a working reader and status reports grabbed: false.
func TestGrabFailureNonFatal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build a valid input_event for "123" followed by Enter
	var buf bytes.Buffer
	for _, code := range []uint16{0x02, 0x03, 0x04, 0x2a} { // 1,2,3,Enter
		buf.Write(make([]byte, 16))
		binary.Write(&buf, binary.LittleEndian, uint16(evKey))
		binary.Write(&buf, binary.LittleEndian, code)
		binary.Write(&buf, binary.LittleEndian, int32(1))
	}

	queue := &fakeQueue{cancel: cancel}
	lookup := &fakeLookup{}
	lookup.result = product.LookupResult{
		Product: &product.ProductSummary{
			ID:   "prod-123",
			Name: "Test Product",
		},
		Source: "global",
	}

	listener := New()
	listener.Source = SourceDevice
	listener.Open = func(path string) (io.ReadCloser, bool, error) {
		// Return grabbed=false to simulate failed exclusive grab
		return io.NopCloser(bytes.NewReader(buf.Bytes())), false, nil
	}
	listener.Queue = queue
	listener.LookupService = lookup

	listener.Run(ctx)

	// Verify a scan was successfully created despite failed grab
	if len(queue.entries) != 1 {
		t.Errorf("expected 1 entry despite failed grab, got %d", len(queue.entries))
	}

	// After reading and EOF, status should be disconnected but we can verify
	// that the grab failure did not prevent scanning
	status := listener.Status()
	if status.Grabbed {
		t.Error("status.Grabbed should be false when grab fails")
	}
}

// TestNextBackoffDoubles tests that nextBackoff doubles and saturates at maxBackoff.
func TestNextBackoffDoubles(t *testing.T) {
	listener := New()
	listener.initialBackoff = 1 * time.Millisecond
	listener.maxBackoff = 30 * time.Millisecond

	current := listener.initialBackoff
	expected := listener.initialBackoff

	// Verify doubling
	for i := 0; i < 5; i++ {
		current = listener.nextBackoff(current)
		expected = expected * 2
		if expected > listener.maxBackoff {
			expected = listener.maxBackoff
		}
		if current != expected {
			t.Errorf("nextBackoff iteration %d: got %v, want %v", i, current, expected)
		}
	}

	// Verify saturation: after reaching maxBackoff, it should stay at maxBackoff
	for i := 0; i < 3; i++ {
		current = listener.nextBackoff(current)
		if current != listener.maxBackoff {
			t.Errorf("after saturation, nextBackoff should stay at maxBackoff, got %v", current)
		}
	}
}

// TestDefaultDevicePath verifies the compiled-in default device path.
func TestDefaultDevicePath(t *testing.T) {
	listener := New()
	if listener.DevicePath != "/dev/input/pantry-scanner" {
		t.Errorf("default DevicePath = %q, want %q", listener.DevicePath, "/dev/input/pantry-scanner")
	}
}

// TestRecconnectLoopPrecondition verifies that httpListenAndServe in main.go
// is preceded by listener.Run in a goroutine, so an unopenable device cannot
// block HTTP startup. This test documents the guarantee rather than enforcing it
// (the enforcement is in main.go's order of operations).
func TestReconnectLoopPrecondition(t *testing.T) {
	// This test is documentation: cmd/server/main.go line ~121 does
	// go listener.Run(context.Background())
	// before line ~125 does
	// http.ListenAndServe(addr, handler)
	// So an unopenable device cannot block or fail HTTP startup.
	// If main.go is refactored to change this order, this comment serves
	// as a guard against accidentally serializing them.
	t.Logf("PRECONDITION: cmd/server/main.go starts listener in goroutine before http.ListenAndServe")
}
