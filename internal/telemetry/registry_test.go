package telemetry

import (
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
}
