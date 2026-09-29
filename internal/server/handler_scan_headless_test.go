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
			DevicePath:   "/dev/input/pantry-scanner",
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
		"devicePath":   "/dev/input/pantry-scanner",
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

// TestHealthWithMissingDevice tests that GET /health returns ok and
// reports scanner.connected: false when the device path does not exist,
// preserving the tolerance this spec depends on.
func TestHealthWithMissingDevice(t *testing.T) {
	statusFn := func() scanlistener.Status {
		return scanlistener.Status{
			Source:       scanlistener.SourceDevice,
			DevicePath:   "/dev/nonexistent-scanner",
			Connected:    false,
			Grabbed:      false,
			Mode:         "stock_in",
			UnmappedKeys: 0,
			LastError:    "device not found",
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

	// Verify the scanner reports disconnected but includes the device path
	if connected, ok := scannerMap["connected"].(bool); !ok || connected {
		t.Errorf("scanner.connected = %v, want false", scannerMap["connected"])
	}

	if devicePath, ok := scannerMap["devicePath"].(string); !ok || devicePath == "" {
		t.Errorf("scanner.devicePath should be set to non-empty string, got %v", scannerMap["devicePath"])
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


