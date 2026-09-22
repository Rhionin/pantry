package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rhionin/pantry/internal/scanlistener"
)

// TestHealthWithScannerStatus verifies that GET /health includes scanner
// status when WithScannerStatus is configured.
func TestHealthWithScannerStatus(t *testing.T) {
	// Create a handler with a scanner status function
	statusFn := func() scanlistener.Status {
		return scanlistener.Status{
			Source:       scanlistener.SourceDevice,
			DevicePath:   "/dev/pantry-scanner",
			Connected:    true,
			Grabbed:      true,
			Mode:         "stock_in",
			UnmappedKeys: 0,
		}
	}

	handler, _ := NewHandler(nil, nil, nil, nil, WithScannerStatus(statusFn))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /health status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse JSON response: %v", w.Body.String())
	}

	if status, ok := response["status"]; !ok || status != "ok" {
		t.Errorf("response[status] = %v, want %v", status, "ok")
	}

	scanner, ok := response["scanner"]
	if !ok {
		t.Error("response missing scanner field")
		return
	}

	scannerMap, ok := scanner.(map[string]any)
	if !ok {
		t.Fatalf("scanner is not an object: %T", scanner)
	}

	expectedScanner := map[string]any{
		"source":       "device",
		"devicePath":   "/dev/pantry-scanner",
		"connected":    true,
		"grabbed":      true,
		"mode":         "stock_in",
		"unmappedKeys": float64(0),
	}

	for k, v := range expectedScanner {
		if scannerMap[k] != v {
			t.Errorf("scanner[%s] = %v, want %v", k, scannerMap[k], v)
		}
	}
}

// TestHealthWithoutScannerStatus verifies that GET /health works without
// scanner status (existing behavior preserved).
func TestHealthWithoutScannerStatus(t *testing.T) {
	handler, _ := NewHandler(nil, nil, nil, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /health status = %d, want %d", w.Code, http.StatusOK)
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse JSON response: %v", w.Body.String())
	}

	if status, ok := response["status"]; !ok || status != "ok" {
		t.Errorf("response[status] = %v, want %v", status, "ok")
	}

	if _, ok := response["scanner"]; ok {
		t.Error("response includes scanner field when none configured")
	}
}


