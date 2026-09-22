package server

import (
	"sync"

	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/scan"
)

// scannerMode holds the browser-selected scan direction. It is the HTTP side's
// share of Current_Mode: the mode-switch endpoint updates it and publishes a
// scanner_mode event through the same Broadcaster GET /api/events uses, so
// every connected browser converges on the same direction. It defaults to
// stock_in, matching the headless listener's newModeState.
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
// the backend does.
type ScannerConfigHandler struct {
	Config ScannerConfig
}

type scannerConfigResponse struct {
	StockInBarcode  string `json:"stockInBarcode"`
	StockOutBarcode string `json:"stockOutBarcode"`
}

func (h *ScannerConfigHandler) Handle(req Request[struct{}, struct{}]) (scannerConfigResponse, error) {
	return scannerConfigResponse{
		StockInBarcode:  h.Config.StockInBarcode,
		StockOutBarcode: h.Config.StockOutBarcode,
	}, nil
}
