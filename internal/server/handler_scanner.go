package server

import (
	"sync"

	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/scan"
)

// scannerMode holds the last direction selected through the HTTP mode-switch
// endpoint. The mode-switch endpoint updates it and publishes a scanner_mode
// event through the same Broadcaster GET /api/events uses, so every connected
// browser converges on the same direction over SSE. GET /api/scanner/config
// reads it back so a browser that connects after a switch starts on the
// current direction rather than a hardcoded guess. It defaults to stock_in,
// matching the headless listener's newModeState.
//
// The headless listener keeps its own modeState because it stamps the direction
// onto entries it creates at scan time and must not depend on an HTTP handler
// being reachable. The two are reconciled for display through the shared
// scanner_mode SSE event, not through shared memory: this is deliberate
// eventual consistency of what browsers show, not a single authoritative
// register. Each capture path (HTTP/browser vs. headless device) owns the
// direction it stamps on the entries it creates.
type scannerMode struct {
	mu      sync.Mutex
	current scan.ScanDirection
}

func newScannerMode() *scannerMode {
	return &scannerMode{current: scan.StockIn}
}

func (m *scannerMode) set(d scan.ScanDirection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = d
}

func (m *scannerMode) get() scan.ScanDirection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// ScannerConfig carries the reserved control-barcode strings the backend
// classifies against, so the browser can recognize the exact same values.
type ScannerConfig struct {
	StockInBarcode  string
	StockOutBarcode string
}

// ScannerModeHandler implements POST /api/scanner/mode. It switches the shared
// scanner mode and broadcasts a scanner_mode SSE event so browser subscribers
// react the same way they do when the headless listener scans a control
// barcode.
type ScannerModeHandler struct {
	Mode        *scannerMode
	Broadcaster *events.Broadcaster
}

type scannerModeRequest struct {
	Mode scan.ScanDirection `json:"mode"`
}

type scannerModeResponse struct {
	Mode scan.ScanDirection `json:"mode"`
}

func (h *ScannerModeHandler) Handle(req Request[scannerModeRequest, struct{}]) (scannerModeResponse, error) {
	switch req.Body.Mode {
	case scan.StockIn, scan.StockOut:
	default:
		return scannerModeResponse{}, BadRequest("mode must be stock_in or stock_out")
	}

	h.Mode.set(req.Body.Mode)
	if h.Broadcaster != nil {
		h.Broadcaster.PublishScannerModeEvent(req.Body.Mode)
	}

	return scannerModeResponse{Mode: req.Body.Mode}, nil
}

// ScannerConfigHandler implements GET /api/scanner/config, exposing the
// reserved control-barcode strings so the browser classifies the exact values
// the backend does, plus the current scanner mode so a browser that connects
// after a mode switch starts on the right direction.
type ScannerConfigHandler struct {
	Config ScannerConfig
	Mode   *scannerMode
}

type scannerConfigResponse struct {
	StockInBarcode  string             `json:"stockInBarcode"`
	StockOutBarcode string             `json:"stockOutBarcode"`
	CurrentMode     scan.ScanDirection `json:"currentMode"`
}

func (h *ScannerConfigHandler) Handle(req Request[struct{}, struct{}]) (scannerConfigResponse, error) {
	currentMode := scan.StockIn
	if h.Mode != nil {
		currentMode = h.Mode.get()
	}
	return scannerConfigResponse{
		StockInBarcode:  h.Config.StockInBarcode,
		StockOutBarcode: h.Config.StockOutBarcode,
		CurrentMode:     currentMode,
	}, nil
}
