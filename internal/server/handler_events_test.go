package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
)

// sseFrame is one decoded "event: <type>\ndata: <json>\n\n" server-sent
// event frame.
type sseFrame struct {
	event string
	data  string
}

// readSSEFrames parses SSE frames from body line-by-line, sending each
// completed frame on frames, until a read fails (connection closed or
// context cancelled) or the body reaches EOF. It closes done when it returns,
// so callers can wait for the goroutine to exit during cleanup.
func readSSEFrames(body io.Reader, frames chan<- sseFrame, done chan<- struct{}) {
	defer close(done)

	scanner := bufio.NewScanner(body)
	var current sseFrame
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			current.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if current.event != "" {
				frames <- current
				current = sseFrame{}
			}
		}
	}
}

// TestEventsStream_ScanEventOnCreate verifies GET /api/events streams a
// "scan" server-sent event carrying the entry created by a normal
// POST /api/scans call against the same handler/DB the stream is subscribed
// to (Requirements 1.1, 1.2, 1.5, 2.1).
func TestEventsStream_ScanEventOnCreate(t *testing.T) {
	handler, catalog, _, _, _, _ := setupTestWithDB(t)

	const barcode = "999888777666"
	if err := catalog.CreateProduct(context.Background(), product.Product{
		ID: "prod-events", Name: "Events Product", Category: "Test", UnitOfMeasure: "unit",
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if err := catalog.UpsertBarcodeMapping(context.Background(), barcode, "prod-events", "global", ""); err != nil {
		t.Fatalf("UpsertBarcodeMapping: %v", err)
	}

	testServer := httptest.NewServer(handler)
	// Registered before the stream's own cleanup below, so LIFO ordering
	// closes the stream (and lets EventsHandler.Handle return) before
	// Close() blocks waiting for that connection to go idle.
	t.Cleanup(testServer.Close)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, testServer.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("build GET /api/events request: %v", err)
	}

	client := &http.Client{} // no timeout: the connection is meant to stay open for the stream's lifetime
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

	// Trigger a mutation through the normal POST /api/scans path, on the same
	// handler/DB the stream above is subscribed to.
	createBody := `{"barcode":"` + barcode + `","direction":"stock_in","userId":"events-user"}`
	postRes, err := http.Post(testServer.URL+"/api/scans", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/scans: %v", err)
	}
	defer postRes.Body.Close()
	if postRes.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/scans: want %d, got %d", http.StatusCreated, postRes.StatusCode)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(postRes.Body).Decode(&created); err != nil {
		t.Fatalf("decode POST /api/scans response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created scan entry has empty id")
	}

	select {
	case frame := <-frames:
		if frame.event != "scan" {
			t.Fatalf("SSE event type: want %q, got %q", "scan", frame.event)
		}
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(frame.data), &payload); err != nil {
			t.Fatalf("decode SSE data: %v (data: %s)", err, frame.data)
		}
		if payload.ID != created.ID {
			t.Fatalf("SSE event id: want %q, got %q", created.ID, payload.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a scan event on the /api/events stream")
	}
}
