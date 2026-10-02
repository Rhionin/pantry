package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/telemetry"
)

// Telemetry lives on the handler that served the request. exchanges() builds
// a fresh handler, so these tests call the same handler twice instead.

func TestTelemetry_RecordsHTTPErrorsAndLatency(t *testing.T) {
	handler, _ := setupTestWithDB(t)

	post := httptest.NewRequest(http.MethodPost, "/api/scans", strings.NewReader("not-json"))
	post.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, post)
	if postRec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/scans status = %d, want 400", postRec.Code)
	}

	missing := httptest.NewRequest(http.MethodGet, "/api/no-such-route", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missing)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("GET missing status = %d, want 404", missingRec.Code)
	}

	snap := getTelemetry(t, handler)
	if snap.HTTP.ClientErrors < 2 {
		t.Fatalf("clientErrors = %d, want at least 2", snap.HTTP.ClientErrors)
	}
	if snap.Runtime.Goroutines < 1 || snap.UptimeSeconds < 0 || snap.Status != "ok" {
		t.Fatalf("runtime snapshot = %+v", snap)
	}

	var sawScan, sawUnmatched bool
	for _, route := range snap.HTTP.Routes {
		switch route.Route {
		case "POST /api/scans":
			sawScan = route.ClientErrors >= 1 && route.Requests >= 1
		case "GET unmatched":
			sawUnmatched = route.ClientErrors >= 1
		}
	}
	if !sawScan || !sawUnmatched {
		t.Fatalf("routes = %+v", snap.HTTP.Routes)
	}
}

func TestTelemetry_ScanWithNoSubscriber(t *testing.T) {
	handler, env := setupTestWithDB(t)
	setupProductWithBarcode("prod-tel", "Telemetry Product", "Test", "111222333444")(env)

	body := `{"barcode":"111222333444","userId":"user-telemetry"}`
	req := httptest.NewRequest(http.MethodPost, "/api/scans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/scans status = %d, body %s", rec.Code, rec.Body.String())
	}

	snap := getTelemetry(t, handler)
	scan := snap.ScanSync.ByEvent["scan"]
	if scan.Published < 1 || scan.PublishedWithNoSubscribers < 1 {
		t.Fatalf("scan publish stats = %+v", scan)
	}
	if scan.Written != 0 {
		t.Fatalf("written = %d, want 0 with no subscriber", scan.Written)
	}
	if snap.ScanSync.Subscribers != 0 {
		t.Fatalf("subscribers = %d, want 0", snap.ScanSync.Subscribers)
	}
}

func TestTelemetry_ScanReachesSubscriber(t *testing.T) {
	handler, env := setupTestWithDB(t)
	setupProductWithBarcode("prod-tel-live", "Telemetry Live", "Test", "999888777666")(env)

	testServer := httptest.NewServer(handler)
	t.Cleanup(testServer.Close)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, testServer.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{}
	streamRes, err := client.Do(streamReq)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancelStream()
		streamRes.Body.Close()
	})
	if streamRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events status = %d", streamRes.StatusCode)
	}

	createBody := `{"barcode":"999888777666","direction":"stock_in","userId":"events-user"}`
	postRes, err := http.Post(testServer.URL+"/api/scans", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	defer postRes.Body.Close()
	if postRes.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/scans status = %d", postRes.StatusCode)
	}

	frames := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		var raw strings.Builder
		for {
			n, readErr := streamRes.Body.Read(buf)
			if n > 0 {
				raw.Write(buf[:n])
				if strings.Contains(raw.String(), "event: scan") {
					frames <- raw.String()
					return
				}
			}
			if readErr != nil {
				frames <- raw.String()
				return
			}
		}
	}()
	var frame string
	select {
	case frame = <-frames:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a scan frame")
	}
	if !strings.Contains(frame, "event: scan") || !strings.Contains(frame, "id: ") {
		t.Fatalf("SSE frame = %q", frame)
	}

	snapRes, err := http.Get(testServer.URL + "/api/telemetry")
	if err != nil {
		t.Fatal(err)
	}
	defer snapRes.Body.Close()
	snap := decodeTelemetry(t, snapRes)
	scan := snap.ScanSync.ByEvent["scan"]
	if scan.Published < 1 || scan.Written < 1 {
		t.Fatalf("scan delivery = %+v", scan)
	}
	if scan.PublishedWithNoSubscribers != 0 {
		t.Fatalf("published with no subscribers = %d, want 0", scan.PublishedWithNoSubscribers)
	}
	if scan.IngestToDelivery.Count < 1 || scan.Delivery.Count < 1 {
		t.Fatalf("latency = delivery %+v ingest %+v", scan.Delivery, scan.IngestToDelivery)
	}
	if snap.ScanSync.Subscribers < 1 {
		t.Fatalf("subscribers = %d, want at least 1", snap.ScanSync.Subscribers)
	}
	if snap.Streams.Opened < 1 || snap.Streams.Active < 1 {
		t.Fatalf("streams = %+v", snap.Streams)
	}
}

func TestTelemetry_ClientReportRoundTripStripsBarcode(t *testing.T) {
	handler, _ := setupTestWithDB(t)

	body := `{"kind":"js_error","page":"/inventory?barcode=123456789012","message":"failed ?barcode=123456789012","session":"sess-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/telemetry/client", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, body %s", rec.Code, rec.Body.String())
	}

	snap := getTelemetry(t, handler)
	if snap.Client.Errors != 1 || len(snap.Client.Recent) != 1 {
		t.Fatalf("client = %+v", snap.Client)
	}
	got := snap.Client.Recent[0]
	if got.Page != "/inventory" || strings.Contains(got.Message, "123456789012") {
		t.Fatalf("stored report = %+v", got)
	}
}

func TestTelemetry_PageLoadSummaryRoundTrip(t *testing.T) {
	handler, _ := setupTestWithDB(t)

	body := `{
		"kind":"page_load",
		"page":"/inventory?barcode=123456789012",
		"durationMs":1400,
		"ttfbMs":202,
		"domContentLoadedMs":1297,
		"jsResources":2,
		"jsTransferBytes":840000,
		"jsEncodedBytes":1100000,
		"cssResources":1,
		"cssTransferBytes":40000
	}`
	post := httptest.NewRequest(http.MethodPost, "/api/telemetry/client", strings.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, post)
	if postRec.Code != http.StatusNoContent {
		t.Fatalf("POST page_load status = %d, body %s", postRec.Code, postRec.Body.String())
	}

	apiBody := `{"kind":"api","route":"/api/inventory?q=milk","status":200,"durationMs":34,"firstPaint":true}`
	apiReq := httptest.NewRequest(http.MethodPost, "/api/telemetry/client", strings.NewReader(apiBody))
	apiReq.Header.Set("Content-Type", "application/json")
	apiRec := httptest.NewRecorder()
	handler.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusNoContent {
		t.Fatalf("POST api status = %d, body %s", apiRec.Code, apiRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/telemetry", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "123456789012") || strings.Contains(raw, "milk") {
		t.Fatalf("snapshot kept private data: %s", raw)
	}
	for _, want := range []string{
		`"pageLoad"`,
		`"dominant":"document"`,
		`"documentMs":1095`,
		`"firstPaintApi":[`,
		`"/api/inventory"`,
		`"jsTransferBytes":840000`,
		`"slowestClientApi":[`,
		`"slowestHttp":[`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("snapshot missing %q in %s", want, raw)
		}
	}

	snap := getTelemetry(t, handler)
	if snap.PageLoad.Latest == nil || snap.PageLoad.Latest.Page != "/inventory" {
		t.Fatalf("latest = %+v", snap.PageLoad.Latest)
	}
	if snap.PageLoad.Latest.DurationMs != 1400 || snap.PageLoad.JS.MaxResources != 2 {
		t.Fatalf("page load = %+v", snap.PageLoad)
	}
	if len(snap.PageLoad.FirstPaintAPI) != 1 || snap.PageLoad.FirstPaintAPI[0].Route != "/api/inventory" {
		t.Fatalf("first paint = %+v", snap.PageLoad.FirstPaintAPI)
	}
}

func TestTelemetry_ClientReportRejectsBadInput(t *testing.T) {
	handler, _ := setupTestWithDB(t)

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "unknown kind", body: `{"kind":"password"}`, want: "unknown report kind"},
		{name: "not json", body: `{"kind"`, want: "invalid report"},
		{name: "too large", body: `{"kind":"page_load","message":"` + strings.Repeat("a", maxClientReportBytes) + `"}`, want: "report is too large"},
		{name: "bad resources", body: `{"kind":"page_load","jsResources":-1}`, want: "resource counts are invalid"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/telemetry/client", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("body = %s, want substring %q", rec.Body.String(), tt.want)
			}
		})
	}
}

func getTelemetry(t *testing.T, handler http.Handler) telemetry.Snapshot {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/telemetry", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/telemetry status = %d, body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content type = %q", ct)
	}
	res := rec.Result()
	defer res.Body.Close()
	return decodeTelemetry(t, res)
}

func decodeTelemetry(t *testing.T, res *http.Response) telemetry.Snapshot {
	t.Helper()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read telemetry: %v", err)
	}
	var snap telemetry.Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("decode telemetry: %v body %s", err, body)
	}
	return snap
}
