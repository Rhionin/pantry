package telemetry

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLatencyPercentile_UsesBucketUpperBound(t *testing.T) {
	reg := NewRegistry()
	for i := 0; i < 9; i++ {
		reg.ObserveHTTP("GET /api/products", 200, 3*time.Millisecond)
	}
	reg.ObserveHTTP("GET /api/products", 200, 2000*time.Millisecond)

	snap := reg.Snapshot()
	if snap.HTTP.Requests != 10 {
		t.Fatalf("requests = %d, want 10", snap.HTTP.Requests)
	}
	if snap.HTTP.Latency.P50Ms != 5 {
		t.Fatalf("p50 = %v, want 5", snap.HTTP.Latency.P50Ms)
	}
	if snap.HTTP.Latency.P95Ms != 2500 {
		t.Fatalf("p95 = %v, want 2500", snap.HTTP.Latency.P95Ms)
	}
	if snap.HTTP.Latency.MaxMs != 2000 {
		t.Fatalf("max = %v, want 2000", snap.HTTP.Latency.MaxMs)
	}
	if len(snap.HTTP.RecentProblems) != 1 {
		t.Fatalf("recent problems = %d, want 1", len(snap.HTTP.RecentProblems))
	}
	if snap.HTTP.RecentProblems[0].Route != "GET /api/products" || snap.HTTP.RecentProblems[0].Status != 200 {
		t.Fatalf("problem = %+v", snap.HTTP.RecentProblems[0])
	}
}

func TestObserveHTTP_CountsClientAndServerErrors(t *testing.T) {
	reg := NewRegistry()
	reg.ObserveHTTP("GET /api/products", 200, time.Millisecond)
	reg.ObserveHTTP("POST /api/scans", 400, time.Millisecond)
	reg.ObserveHTTP("POST /api/scans", 500, 2*time.Millisecond)

	snap := reg.Snapshot()
	if snap.HTTP.ClientErrors != 1 || snap.HTTP.ServerErrors != 1 {
		t.Fatalf("errors = client %d server %d", snap.HTTP.ClientErrors, snap.HTTP.ServerErrors)
	}
	var scans RouteSnapshot
	found := false
	for _, route := range snap.HTTP.Routes {
		if route.Route == "POST /api/scans" {
			scans = route
			found = true
		}
	}
	if !found {
		t.Fatal("missing POST /api/scans route")
	}
	if scans.ClientErrors != 1 || scans.ServerErrors != 1 || scans.Requests != 2 {
		t.Fatalf("scan route = %+v", scans)
	}
	if len(snap.HTTP.RecentProblems) != 1 || snap.HTTP.RecentProblems[0].Status != 500 {
		t.Fatalf("problems = %+v", snap.HTTP.RecentProblems)
	}
}

func TestObservePublish_NoSubscriberAndDelivery(t *testing.T) {
	reg := NewRegistry()
	reg.ObservePublish("scan", 0, 0, 0)
	reg.ObserveDelivery("scan", 4*time.Millisecond, 12*time.Millisecond, true)
	reg.ObservePublish("scan", 1, 1, 0)

	snap := reg.Snapshot()
	scan := snap.ScanSync.ByEvent["scan"]
	if scan.Published != 2 || scan.PublishedWithNoSubscribers != 1 || scan.Written != 1 {
		t.Fatalf("scan stats = %+v", scan)
	}
	if scan.IngestToDelivery.Count != 1 {
		t.Fatalf("ingest count = %d, want 1", scan.IngestToDelivery.Count)
	}
	if snap.ScanSync.PublishedWithNoSubscribers != 1 || snap.ScanSync.Written != 1 {
		t.Fatalf("totals = %+v", snap.ScanSync)
	}
	if _, ok := snap.ScanSync.ByEvent["inventory"]; !ok {
		t.Fatal("inventory event stats missing before any inventory publish")
	}
}

func TestObservePublish_DroppedSlowClient(t *testing.T) {
	reg := NewRegistry()
	reg.ObservePublish("inventory", 2, 1, 1)
	snap := reg.Snapshot()
	if snap.ScanSync.DroppedSlowClients != 1 {
		t.Fatalf("dropped = %d, want 1", snap.ScanSync.DroppedSlowClients)
	}
	if snap.ScanSync.ByEvent["inventory"].Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1", snap.ScanSync.ByEvent["inventory"].Enqueued)
	}
}

func TestAcceptClientReport_StripsQueryAndRecordsError(t *testing.T) {
	reg := NewRegistry()
	err := reg.AcceptClientReport(InboundReport{
		Kind:    "js_error",
		Page:    "/inventory?barcode=123456789012",
		Message: "load failed ?barcode=123456789012",
		Session: "abc_DEF-1",
	})
	if err != nil {
		t.Fatalf("AcceptClientReport: %v", err)
	}

	snap := reg.Snapshot()
	if snap.Client.Errors != 1 || snap.Client.Reports != 1 {
		t.Fatalf("client = %+v", snap.Client)
	}
	if len(snap.Client.Recent) != 1 {
		t.Fatalf("recent = %d, want 1", len(snap.Client.Recent))
	}
	got := snap.Client.Recent[0]
	if got.Page != "/inventory" {
		t.Fatalf("page = %q", got.Page)
	}
	if strings.Contains(got.Message, "123456789012") || strings.Contains(got.Message, "?") {
		t.Fatalf("message kept query data: %q", got.Message)
	}
	if got.Session != "abc_DEF-1" {
		t.Fatalf("session = %q", got.Session)
	}
	if got.At == "" {
		t.Fatal("missing receipt timestamp")
	}
}

func TestAcceptClientReport_APISuccessDoesNotCrowdOutErrors(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AcceptClientReport(InboundReport{Kind: "api", Route: "/api/products/lookup?barcode=999", Status: 200, DurationMs: 12}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AcceptClientReport(InboundReport{Kind: "api", Route: "/api/scans", Status: 0, DurationMs: 40}); err != nil {
		t.Fatal(err)
	}

	snap := reg.Snapshot()
	if snap.Client.APICalls != 2 || snap.Client.APIErrors != 1 {
		t.Fatalf("api calls = %d errors = %d", snap.Client.APICalls, snap.Client.APIErrors)
	}
	if len(snap.Client.Recent) != 1 || snap.Client.Recent[0].Status != 0 {
		t.Fatalf("recent = %+v", snap.Client.Recent)
	}
	if snap.Client.Recent[0].Route != "/api/scans" {
		t.Fatalf("route = %q", snap.Client.Recent[0].Route)
	}
	if strings.Contains(snap.Client.Recent[0].Route, "barcode") {
		t.Fatalf("route kept barcode: %q", snap.Client.Recent[0].Route)
	}
}

func TestAcceptClientReport_RejectsUnknownKindAndBadNumbers(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AcceptClientReport(InboundReport{Kind: "password"}); err != errUnknownKind {
		t.Fatalf("kind error = %v", err)
	}
	if err := reg.AcceptClientReport(InboundReport{Kind: "page_load", DurationMs: -1}); err != errNegativeDuration {
		t.Fatalf("duration error = %v", err)
	}
	if err := reg.AcceptClientReport(InboundReport{Kind: "api", Status: 99}); err != errBadStatus {
		t.Fatalf("status error = %v", err)
	}
	if snap := reg.Snapshot(); snap.Client.Reports != 0 {
		t.Fatalf("reports = %d, want 0", snap.Client.Reports)
	}
}

func TestAcceptClientReport_StripsAbsoluteURLAndBadSession(t *testing.T) {
	reg := NewRegistry()
	err := reg.AcceptClientReport(InboundReport{
		Kind:    "route",
		Page:    "https://pantry.local/shopping?q=milk",
		Session: "not a session",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := reg.Snapshot().Client.Recent[0]
	if got.Page != "/shopping" {
		t.Fatalf("page = %q", got.Page)
	}
	if got.Session != "" {
		t.Fatalf("session = %q", got.Session)
	}
}

func TestSnapshot_SubscriberCountAndStreamActive(t *testing.T) {
	reg := NewRegistry()
	reg.SetSubscriberCount(func() int { return 2 })
	reg.ObserveStreamOpen(15 * time.Millisecond)
	reg.ObserveStreamOpen(20 * time.Millisecond)
	reg.ObserveStreamClose()

	snap := reg.Snapshot()
	if snap.ScanSync.Subscribers != 2 {
		t.Fatalf("subscribers = %d", snap.ScanSync.Subscribers)
	}
	if snap.Streams.Opened != 2 || snap.Streams.Closed != 1 || snap.Streams.Active != 1 {
		t.Fatalf("streams = %+v", snap.Streams)
	}
	if snap.Runtime.Goroutines < 1 {
		t.Fatal("expected a goroutine count")
	}
	if snap.Status != "ok" || snap.StartedAt == "" {
		t.Fatalf("status snapshot = %+v", snap)
	}
	if snap.PageLoad.Dominant != "unknown" || snap.PageLoad.Note == "" {
		t.Fatalf("empty page load = %+v", snap.PageLoad)
	}
	if snap.PageLoad.FirstPaintAPI == nil || snap.PageLoad.SlowestClientAPI == nil || snap.PageLoad.SlowestHTTP == nil {
		t.Fatal("page load lists must be empty arrays, not null")
	}
}

func TestPageLoadSummary_DocumentWaitAndStrippedRoutes(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "page_load", DurationMs: 800, TTFBMs: 100, DOMContentLoadedMs: 400,
	}); err != nil {
		t.Fatal(err)
	}
	err := reg.AcceptClientReport(InboundReport{
		Kind:               "page_load",
		Page:               "/?barcode=123456789012",
		DurationMs:         1400,
		TTFBMs:             202,
		DOMContentLoadedMs: 1297,
		JSResources:        2,
		JSTransferBytes:    840000,
		JSEncodedBytes:     1100000,
		CSSResources:       1,
		CSSTransferBytes:   120000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "api", Route: "/api/inventory?q=milk", Status: 200, DurationMs: 34, FirstPaint: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "api", Route: "/api/scans?userId=user-1&status=pending", Status: 200, DurationMs: 40, FirstPaint: true,
	}); err != nil {
		t.Fatal(err)
	}
	// A later poll is not a first-paint call, but it still counts as client API time.
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "api", Route: "/api/scanner/config", Status: 200, DurationMs: 12,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "api", Route: "/api/products/lookup?barcode=999", Status: 500, DurationMs: 80,
	}); err != nil {
		t.Fatal(err)
	}

	reg.ObserveHTTP("GET /", 200, 565*time.Millisecond)
	reg.ObserveHTTP("GET /api/inventory", 200, 34*time.Millisecond)

	snap := reg.Snapshot()
	if snap.PageLoad.Samples != 2 {
		t.Fatalf("samples = %d", snap.PageLoad.Samples)
	}
	if snap.PageLoad.Dominant != "document" {
		t.Fatalf("dominant = %q note %q", snap.PageLoad.Dominant, snap.PageLoad.Note)
	}
	if snap.PageLoad.Latest == nil || snap.PageLoad.Latest.Page != "/" {
		t.Fatalf("latest = %+v", snap.PageLoad.Latest)
	}
	if snap.PageLoad.Latest.TTFBMs != 202 || snap.PageLoad.Latest.DocumentMs != 1095 || snap.PageLoad.Latest.AfterDomMs != 103 {
		t.Fatalf("latest gaps = %+v", snap.PageLoad.Latest)
	}
	if snap.PageLoad.JS.MaxTransferBytes != 840000 || snap.PageLoad.JS.MaxResources != 2 {
		t.Fatalf("js = %+v", snap.PageLoad.JS)
	}
	if snap.PageLoad.Duration.Count != 2 || snap.PageLoad.TTFB.Count != 2 {
		t.Fatalf("histograms = duration %+v ttfb %+v", snap.PageLoad.Duration, snap.PageLoad.TTFB)
	}
	if strings.Contains(snap.PageLoad.Note, "123456789012") || strings.Contains(snap.PageLoad.Note, "barcode") {
		t.Fatalf("note kept private data: %q", snap.PageLoad.Note)
	}

	var sawInventory, sawScans bool
	for _, row := range snap.PageLoad.FirstPaintAPI {
		if strings.Contains(row.Route, "barcode") || strings.Contains(row.Route, "userId") || strings.Contains(row.Route, "?") {
			t.Fatalf("first paint route kept a query: %q", row.Route)
		}
		switch row.Route {
		case "/api/inventory":
			sawInventory = row.Latency.MaxMs == 34
		case "/api/scans":
			sawScans = row.Latency.MaxMs == 40
		default:
			t.Fatalf("unexpected first paint route %q", row.Route)
		}
	}
	if !sawInventory || !sawScans {
		t.Fatalf("first paint = %+v", snap.PageLoad.FirstPaintAPI)
	}

	if len(snap.PageLoad.SlowestClientAPI) == 0 || snap.PageLoad.SlowestClientAPI[0].Route != "/api/products/lookup" {
		t.Fatalf("slowest client api = %+v", snap.PageLoad.SlowestClientAPI)
	}
	for _, row := range snap.PageLoad.SlowestClientAPI {
		if strings.Contains(row.Route, "999") || strings.Contains(row.Route, "barcode") {
			t.Fatalf("client api route kept a barcode: %q", row.Route)
		}
	}
	var sawFailed bool
	for _, rep := range snap.Client.Recent {
		if rep.Kind == "api" && rep.Status == 200 {
			t.Fatalf("fast success crowded recent: %+v", snap.Client.Recent)
		}
		if rep.Kind == "api" && rep.Route == "/api/products/lookup" && rep.Status == 500 {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Fatalf("recent = %+v", snap.Client.Recent)
	}

	if len(snap.PageLoad.SlowestHTTP) == 0 || snap.PageLoad.SlowestHTTP[0].Route != "GET /" {
		t.Fatalf("slowest http = %+v", snap.PageLoad.SlowestHTTP)
	}
	if snap.PageLoad.SlowestHTTP[0].Latency.MaxMs != 565 {
		t.Fatalf("GET / max = %v", snap.PageLoad.SlowestHTTP[0].Latency.MaxMs)
	}
}

func TestPageLoadSummary_ZeroDurationDoesNotInventAfterDom(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "page_load", Page: "/", DurationMs: 0, TTFBMs: 202, DOMContentLoadedMs: 1297,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "api", Route: "/api/inventory", Status: 200, DurationMs: 34, FirstPaint: true,
	}); err != nil {
		t.Fatal(err)
	}
	snap := reg.Snapshot()
	if snap.PageLoad.Dominant != "document" {
		t.Fatalf("dominant = %q note %q", snap.PageLoad.Dominant, snap.PageLoad.Note)
	}
	if snap.PageLoad.Latest.AfterDomMs != 0 || snap.PageLoad.Latest.DocumentMs != 1095 {
		t.Fatalf("gaps = %+v", snap.PageLoad.Latest)
	}
	if !strings.Contains(snap.PageLoad.Note, "full load was not recorded") {
		t.Fatalf("note = %q", snap.PageLoad.Note)
	}
}

func TestPageLoadSummary_APIWaitCanDominate(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "page_load", DurationMs: 300, TTFBMs: 40, DOMContentLoadedMs: 80,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AcceptClientReport(InboundReport{
		Kind: "api", Route: "/api/products", Status: 200, DurationMs: 1800, FirstPaint: true,
	}); err != nil {
		t.Fatal(err)
	}
	snap := reg.Snapshot()
	if snap.PageLoad.Dominant != "api" {
		t.Fatalf("dominant = %q note %q", snap.PageLoad.Dominant, snap.PageLoad.Note)
	}
	if !strings.Contains(snap.PageLoad.Note, "/api/products") {
		t.Fatalf("note = %q", snap.PageLoad.Note)
	}
}

func TestClientAPIRoutes_StopGrowingAfterTheCap(t *testing.T) {
	reg := NewRegistry()
	for i := 0; i < maxClientRoutes; i++ {
		route := fmt.Sprintf("/api/n%d", i)
		if err := reg.AcceptClientReport(InboundReport{Kind: "api", Route: route, Status: 200, DurationMs: 10}); err != nil {
			t.Fatal(err)
		}
	}
	if err := reg.AcceptClientReport(InboundReport{Kind: "api", Route: "/api/overflow", Status: 200, DurationMs: 9000}); err != nil {
		t.Fatal(err)
	}
	snap := reg.Snapshot()
	if snap.Client.APICalls != int64(maxClientRoutes+1) {
		t.Fatalf("api calls = %d", snap.Client.APICalls)
	}
	for _, row := range snap.PageLoad.SlowestClientAPI {
		if row.Route == "/api/overflow" {
			t.Fatal("route past the cap was stored")
		}
	}
	if snap.PageLoad.SlowestClientAPI[0].Latency.MaxMs > 100 {
		t.Fatalf("slowest = %+v", snap.PageLoad.SlowestClientAPI[0])
	}
}

func TestAcceptClientReport_RejectsResourceCounts(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AcceptClientReport(InboundReport{Kind: "page_load", JSResources: -1}); err != errBadResources {
		t.Fatalf("count error = %v", err)
	}
	if err := reg.AcceptClientReport(InboundReport{Kind: "page_load", CSSTransferBytes: maxResourceBytes + 1}); err != errBadResources {
		t.Fatalf("bytes error = %v", err)
	}
	if snap := reg.Snapshot(); snap.PageLoad.Samples != 0 {
		t.Fatalf("samples = %d", snap.PageLoad.Samples)
	}
}

func TestPageLoadSummary_CapsSlowRouteList(t *testing.T) {
	reg := NewRegistry()
	for i, ms := range []float64{10, 20, 30, 40, 50, 60} {
		route := "/api/r" + string(rune('a'+i))
		if err := reg.AcceptClientReport(InboundReport{Kind: "api", Route: route, Status: 200, DurationMs: ms}); err != nil {
			t.Fatal(err)
		}
	}
	snap := reg.Snapshot()
	if len(snap.PageLoad.SlowestClientAPI) != maxSlowRoutes {
		t.Fatalf("slow routes = %d, want %d (%+v)", len(snap.PageLoad.SlowestClientAPI), maxSlowRoutes, snap.PageLoad.SlowestClientAPI)
	}
	if snap.PageLoad.SlowestClientAPI[0].Route != "/api/rf" {
		t.Fatalf("slowest = %q", snap.PageLoad.SlowestClientAPI[0].Route)
	}
}
