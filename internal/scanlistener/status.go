package scanlistener

import (
	"sync"
	"time"

	"github.com/Rhionin/pantry/internal/scan"
)

// Status is the externally observable state of the capture path, surfaced
// through GET /health because an appliance with no monitor has no other way
// to distinguish a dead scanner from an idle one.
type Status struct {
	Source       Source     `json:"source"`       // "device" or "stdin"
	DevicePath   string     `json:"devicePath,omitempty"`
	Connected    bool       `json:"connected"`
	Grabbed      bool       `json:"grabbed"`
	Mode         string     `json:"mode"`
	UnmappedKeys int        `json:"unmappedKeys"`
	LastScanAt   *time.Time `json:"lastScanAt"`
	LastError    string     `json:"lastError,omitempty"`
}

// status holds the external state with mutex protection.
type status struct {
	mu         sync.Mutex
	source     Source
	devicePath string
	connected  bool
	grabbed    bool
	mode       scan.ScanDirection
	unmapped   int
	lastScanAt *time.Time
	lastError  string
}

// newStatus creates a status with initial values.
func newStatus() *status {
	return &status{
		source: SourceDevice, // default
		mode:   scan.StockIn, // default from modeState
	}
}

// setSource sets the active source.
func (s *status) setSource(src Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.source = src
}

// setDevicePath sets the device path.
func (s *status) setDevicePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devicePath = path
}

// setConnected sets connection status and optionally the grabbed flag.
func (s *status) setConnected(connected, grabbed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = connected
	s.grabbed = grabbed
	if connected {
		s.lastError = ""
	}
}

// setMode sets the current scan direction.
func (s *status) setMode(d scan.ScanDirection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = d
}

// setUnmappedCount sets the count of unmapped keycodes.
func (s *status) setUnmappedCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unmapped = count
}

// setLastScanAt sets the timestamp of the last successful scan.
func (s *status) setLastScanAt(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScanAt = &t
}

// setError sets or clears the last error message.
func (s *status) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.lastError = err.Error()
	} else {
		s.lastError = ""
	}
}

// get returns the current status.
func (s *status) get() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{
		Source:       s.source,
		DevicePath:   s.devicePath,
		Connected:    s.connected,
		Grabbed:      s.grabbed,
		Mode:         string(s.mode),
		UnmappedKeys: s.unmapped,
		LastScanAt:   s.lastScanAt,
		LastError:    s.lastError,
	}
}
