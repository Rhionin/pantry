package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestScannerModeHandler_ValidModes verifies POST /api/scanner/mode accepts
// both scan directions and echoes the selected mode back.
func TestScannerModeHandler_ValidModes(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "stock_in is accepted and echoed",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scanner/mode",
				body:           `{"mode":"stock_in"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.mode", value: "stock_in"},
				},
			},
		},
		{
			name: "stock_out is accepted and echoed",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scanner/mode",
				body:           `{"mode":"stock_out"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.mode", value: "stock_out"},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}

// TestScannerModeHandler_InvalidMode verifies POST /api/scanner/mode rejects
// anything that is not a known scan direction with 400 Bad Request.
func TestScannerModeHandler_InvalidMode(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "unknown mode string is rejected",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scanner/mode",
				body:           `{"mode":"sideways"}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
		{
			name: "empty mode is rejected",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scanner/mode",
				body:           `{"mode":""}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
	}

	runHandlerTests(t, tests)
}

// TestScannerConfigHandler_Defaults verifies GET /api/scanner/config reports
// the STOCK_IN/STOCK_OUT defaults when NewHandler is built without
// WithScannerConfig (the setupTestWithDB case).
func TestScannerConfigHandler_Defaults(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "config returns the default control-barcode strings",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/scanner/config",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.stockInBarcode", value: "STOCK_IN"},
					{path: "$.stockOutBarcode", value: "STOCK_OUT"},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}

// TestScannerConfigHandler_ConfiguredStrings verifies WithScannerConfig
// threads custom control-barcode strings through to GET /api/scanner/config.
func TestScannerConfigHandler_ConfiguredStrings(t *testing.T) {
	handler, _ := NewHandler(nil, nil, nil, nil, WithScannerConfig(ScannerConfig{
		StockInBarcode:  "IN-42",
		StockOutBarcode: "OUT-42",
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/scanner/config", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/scanner/config status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		StockInBarcode  string `json:"stockInBarcode"`
		StockOutBarcode string `json:"stockOutBarcode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode config response: %v (%s)", err, w.Body.String())
	}
	if body.StockInBarcode != "IN-42" {
		t.Errorf("stockInBarcode = %q, want %q", body.StockInBarcode, "IN-42")
	}
	if body.StockOutBarcode != "OUT-42" {
		t.Errorf("stockOutBarcode = %q, want %q", body.StockOutBarcode, "OUT-42")
	}
}

// TestScannerModeHandler_BroadcastsSSEEvent verifies that POSTing to
// /api/scanner/mode publishes a scanner_mode event through the same
// Broadcaster GET /api/events subscribes to, so a browser-initiated mode
// switch reaches every connected client exactly like a headless control scan.
// It reuses the SSE frame reader from handler_events_test.go.
func TestScannerModeHandler_BroadcastsSSEEvent(t *testing.T) {
	handler, _ := setupTestWithDB(t)

	testServer := httptest.NewServer(handler)
	t.Cleanup(testServer.Close)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, testServer.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("build GET /api/events request: %v", err)
	}

	client := &http.Client{} // no timeout: the stream stays open for its lifetime
	streamRes, err := client.Do(streamReq)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	if streamRes.StatusCode != http.StatusOK {
		streamRes.Body.Close()
		t.Fatalf("GET /api/events: want %d, got %d", http.StatusOK, streamRes.StatusCode)
	}

	frames := make(chan sseFrame, 16)
	done := make(chan struct{})
	go readSSEFrames(streamRes.Body, frames, done)

	t.Cleanup(func() {
		cancelStream()
		streamRes.Body.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("readSSEFrames goroutine did not exit after cancelling the stream")
		}
	})

	modeRes, err := http.Post(testServer.URL+"/api/scanner/mode", "application/json", strings.NewReader(`{"mode":"stock_out"}`))
	if err != nil {
		t.Fatalf("POST /api/scanner/mode: %v", err)
	}
	defer modeRes.Body.Close()
	if modeRes.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/scanner/mode: want %d, got %d", http.StatusOK, modeRes.StatusCode)
	}

	select {
	case frame := <-frames:
		if frame.event != "scanner_mode" {
			t.Fatalf("SSE event type: want %q, got %q", "scanner_mode", frame.event)
		}
		// PublishScannerModeEvent marshals the raw ScanDirection string, so the
		// data field is a JSON string literal, e.g. "stock_out".
		var mode string
		if err := json.Unmarshal([]byte(frame.data), &mode); err != nil {
			t.Fatalf("decode SSE data: %v (data: %s)", err, frame.data)
		}
		if mode != "stock_out" {
			t.Fatalf("scanner_mode event payload: want %q, got %q", "stock_out", mode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a scanner_mode event on the /api/events stream")
	}
}
