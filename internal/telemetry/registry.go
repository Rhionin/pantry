// Package telemetry keeps an in-process picture of how the pantry server and
// the browser are behaving. Nothing here is shipped off the machine: callers
// read a snapshot over HTTP, and the counters never include barcodes, product
// names, or user ids.
package telemetry

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	slowRequest       = time.Second
	maxRecentReports  = 40
	maxRecentProblems = 20
	logInterval       = 30 * time.Second
	maxMessageLen     = 180
	maxPathLen        = 128
	maxClientDuration = time.Hour
	maxClientSkipped  = 10000
	maxSlowRoutes     = 5
	maxClientRoutes   = 64
	maxResourceCount  = 500
	maxResourceBytes  = 64 << 20
)

// latencyEdgesMs are inclusive upper bounds. A sample of 3ms lands in the 5ms
// bucket, so reported percentiles are bucket ceilings, not exact quantiles.
var latencyEdgesMs = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

var (
	errUnknownKind      = errors.New("unknown report kind")
	errNegativeDuration = errors.New("duration must not be negative")
	errBadStatus        = errors.New("status is invalid")
	errBadSkipped       = errors.New("skipped count is invalid")
	errBadResources     = errors.New("resource counts are invalid")
)

var (
	queryInText = regexp.MustCompile(`\?[^\s]*`)
	sessionRE   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// clientKinds are the only browser report types the server stores.
var clientKinds = map[string]struct{}{
	"page_load":           {},
	"route":               {},
	"js_error":            {},
	"unhandled_rejection": {},
	"api":                 {},
	"sse_error":           {},
	"sse_gap":             {},
	"sse_open":            {},
}

// Registry collects telemetry for one server process.
type Registry struct {
	mu      sync.Mutex
	started time.Time

	routes           map[string]*routeStats
	httpRequests     int64
	httpClientErrors int64
	httpServerErrors int64
	httpLatency      latencyHist
	problems         []Problem
	httpLog          map[string]*logGate

	streamsOpened     int64
	streamsClosed     int64
	streamOpenLatency latencyHist

	subscribers func() int

	published      int64
	enqueued       int64
	dropped        int64
	marshalErrors  int64
	written        int64
	writeErrors    int64
	noSubscribers  int64
	delivery       latencyHist
	ingestDelivery latencyHist
	byEvent        map[string]*eventStats
	noSubLog       map[string]*logGate

	client    clientStats
	recent    []ClientReport
	clientLog map[string]*logGate

	pageDuration latencyHist
	pageTTFB     latencyHist
	pageDCL      latencyHist
	latestPage   *ClientReport
	jsAssets     assetMax
	cssAssets    assetMax

	clientAPI     map[string]*apiRouteStats
	firstPaintAPI map[string]*apiRouteStats
}

type routeStats struct {
	requests     int64
	clientErrors int64
	serverErrors int64
	latency      latencyHist
}

type eventStats struct {
	published      int64
	enqueued       int64
	dropped        int64
	marshalErrors  int64
	written        int64
	writeErrors    int64
	noSubscribers  int64
	delivery       latencyHist
	ingestDelivery latencyHist
}

type clientStats struct {
	reports   int64
	pageLoads int64
	routes    int64
	errors    int64
	apiCalls  int64
	apiErrors int64
	sseOpens  int64
	sseErrors int64
	sseGaps   int64
}

type apiRouteStats struct {
	calls   int64
	errors  int64
	latency latencyHist
}

type assetMax struct {
	resources int
	transfer  int64
	encoded   int64
}

type latencyHist struct {
	counts []int64
	count  int64
	sum    float64
	max    float64
}

type logGate struct {
	last       time.Time
	suppressed int
}

// Problem is one slow or failed HTTP request, keyed by the route pattern
// rather than the raw URL so ids and query strings stay out of the snapshot.
type Problem struct {
	At         string  `json:"at"`
	Route      string  `json:"route"`
	Status     int     `json:"status"`
	DurationMs float64 `json:"durationMs"`
}

// InboundReport is the JSON body posted by the browser. Fields that could
// carry a barcode or other query data are stripped before storage.
type InboundReport struct {
	Kind               string  `json:"kind"`
	Page               string  `json:"page"`
	Route              string  `json:"route"`
	Status             int     `json:"status"`
	DurationMs         float64 `json:"durationMs"`
	TTFBMs             float64 `json:"ttfbMs"`
	DOMContentLoadedMs float64 `json:"domContentLoadedMs"`
	Message            string  `json:"message"`
	Session            string  `json:"session"`
	Skipped            int     `json:"skipped"`
	JSResources        int     `json:"jsResources"`
	JSTransferBytes    int64   `json:"jsTransferBytes"`
	JSEncodedBytes     int64   `json:"jsEncodedBytes"`
	CSSResources       int     `json:"cssResources"`
	CSSTransferBytes   int64   `json:"cssTransferBytes"`
	CSSEncodedBytes    int64   `json:"cssEncodedBytes"`
	FirstPaint         bool    `json:"firstPaint"`
}

// ClientReport is one stored browser report. At is when the server received
// it, so it lines up with server logs on the same clock.
type ClientReport struct {
	At                 string  `json:"at"`
	Kind               string  `json:"kind"`
	Page               string  `json:"page,omitempty"`
	Route              string  `json:"route,omitempty"`
	Status             int     `json:"status,omitempty"`
	DurationMs         float64 `json:"durationMs,omitempty"`
	TTFBMs             float64 `json:"ttfbMs,omitempty"`
	DOMContentLoadedMs float64 `json:"domContentLoadedMs,omitempty"`
	Message            string  `json:"message,omitempty"`
	Session            string  `json:"session,omitempty"`
	Skipped            int     `json:"skipped,omitempty"`
	JSResources        int     `json:"jsResources,omitempty"`
	JSTransferBytes    int64   `json:"jsTransferBytes,omitempty"`
	JSEncodedBytes     int64   `json:"jsEncodedBytes,omitempty"`
	CSSResources       int     `json:"cssResources,omitempty"`
	CSSTransferBytes   int64   `json:"cssTransferBytes,omitempty"`
	CSSEncodedBytes    int64   `json:"cssEncodedBytes,omitempty"`
	FirstPaint         bool    `json:"firstPaint,omitempty"`
}

// LatencySnapshot summarizes a duration histogram. Percentiles are upper
// bounds of the fixed buckets listed in the package comment, in milliseconds.
type LatencySnapshot struct {
	Count int64   `json:"count"`
	SumMs float64 `json:"sumMs"`
	MaxMs float64 `json:"maxMs"`
	P50Ms float64 `json:"p50Ms"`
	P95Ms float64 `json:"p95Ms"`
	P99Ms float64 `json:"p99Ms"`
}

// RouteSnapshot is the request count and latency for one method+pattern.
type RouteSnapshot struct {
	Route        string          `json:"route"`
	Requests     int64           `json:"requests"`
	ClientErrors int64           `json:"clientErrors"`
	ServerErrors int64           `json:"serverErrors"`
	Latency      LatencySnapshot `json:"latency"`
}

// HTTPSnapshot covers ordinary request/response routes. Long-lived event
// streams are reported separately so one open page cannot dominate latency.
type HTTPSnapshot struct {
	Requests       int64           `json:"requests"`
	ClientErrors   int64           `json:"clientErrors"`
	ServerErrors   int64           `json:"serverErrors"`
	Latency        LatencySnapshot `json:"latency"`
	Routes         []RouteSnapshot `json:"routes"`
	RecentProblems []Problem       `json:"recentProblems"`
}

// StreamSnapshot counts GET /api/events connections. Active is connections
// that have sent response headers and not yet finished.
type StreamSnapshot struct {
	Opened      int64           `json:"opened"`
	Closed      int64           `json:"closed"`
	Active      int64           `json:"active"`
	OpenLatency LatencySnapshot `json:"openLatency"`
}

// EventSnapshot is the publish-to-browser path for one event type.
// PublishedWithNoSubscribers counts events that were saved but had nobody
// listening, which is why a scan can be missing until the page is refreshed.
// DroppedSlowClients counts subscribers whose buffer filled; those browsers
// are disconnected and do not receive the event they missed.
type EventSnapshot struct {
	Published                  int64           `json:"published"`
	Enqueued                   int64           `json:"enqueued"`
	DroppedSlowClients         int64           `json:"droppedSlowClients"`
	MarshalErrors              int64           `json:"marshalErrors"`
	Written                    int64           `json:"written"`
	WriteErrors                int64           `json:"writeErrors"`
	PublishedWithNoSubscribers int64           `json:"publishedWithNoSubscribers"`
	Delivery                   LatencySnapshot `json:"delivery"`
	IngestToDelivery           LatencySnapshot `json:"ingestToDelivery"`
}

// ScanSyncSnapshot is the scan-ingest to connected-browser path.
// Subscribers is the live count of browsers that will receive the next event.
// Delivery is the delay from publish to a successful server-sent write.
// IngestToDelivery is the delay from the scan's scannedAt timestamp to that write.
type ScanSyncSnapshot struct {
	Subscribers                int                      `json:"subscribers"`
	Published                  int64                    `json:"published"`
	Enqueued                   int64                    `json:"enqueued"`
	DroppedSlowClients         int64                    `json:"droppedSlowClients"`
	MarshalErrors              int64                    `json:"marshalErrors"`
	Written                    int64                    `json:"written"`
	WriteErrors                int64                    `json:"writeErrors"`
	PublishedWithNoSubscribers int64                    `json:"publishedWithNoSubscribers"`
	Delivery                   LatencySnapshot          `json:"delivery"`
	IngestToDelivery           LatencySnapshot          `json:"ingestToDelivery"`
	ByEvent                    map[string]EventSnapshot `json:"byEvent"`
}

// ClientSnapshot summarizes browser reports posted to /api/telemetry/client.
type ClientSnapshot struct {
	Reports      int64          `json:"reports"`
	PageLoads    int64          `json:"pageLoads"`
	RouteChanges int64          `json:"routeChanges"`
	Errors       int64          `json:"errors"`
	APICalls     int64          `json:"apiCalls"`
	APIErrors    int64          `json:"apiErrors"`
	SSEOpens     int64          `json:"sseOpens"`
	SSEErrors    int64          `json:"sseErrors"`
	SSEGaps      int64          `json:"sseGaps"`
	Recent       []ClientReport `json:"recent"`
}

// AssetSnapshot is the largest JS or CSS payload seen on a page_load report.
// Counts and sizes only: resource URLs are not stored.
type AssetSnapshot struct {
	MaxResources     int   `json:"maxResources"`
	MaxTransferBytes int64 `json:"maxTransferBytes"`
	MaxEncodedBytes  int64 `json:"maxEncodedBytes"`
}

// PageLoadLatest is the most recent page_load with the gaps already subtracted.
// DocumentMs is DOM ready minus time to first byte. AfterDomMs is full load
// minus DOM ready, and stays 0 when the browser left duration at 0.
type PageLoadLatest struct {
	At                 string  `json:"at"`
	Page               string  `json:"page,omitempty"`
	DurationMs         float64 `json:"durationMs"`
	TTFBMs             float64 `json:"ttfbMs"`
	DOMContentLoadedMs float64 `json:"domContentLoadedMs"`
	DocumentMs         float64 `json:"documentMs"`
	AfterDomMs         float64 `json:"afterDomMs"`
	JSResources        int     `json:"jsResources,omitempty"`
	JSTransferBytes    int64   `json:"jsTransferBytes,omitempty"`
	JSEncodedBytes     int64   `json:"jsEncodedBytes,omitempty"`
	CSSResources       int     `json:"cssResources,omitempty"`
	CSSTransferBytes   int64   `json:"cssTransferBytes,omitempty"`
	CSSEncodedBytes    int64   `json:"cssEncodedBytes,omitempty"`
}

// RouteTiming is one route's client or server latency, for the short list at
// the top of the snapshot.
type RouteTiming struct {
	Route   string          `json:"route"`
	Calls   int64           `json:"calls"`
	Errors  int64           `json:"errors,omitempty"`
	Latency LatencySnapshot `json:"latency"`
}

// PageLoadSnapshot is the short reading of "why is the page slow?".
// Duration, TTFB, and DOMContentLoaded percentiles use the same buckets as
// other latency fields. Latest holds the exact milliseconds from the most
// recent page_load. Dominant is ttfb, document, after_dom, api, or unknown.
type PageLoadSnapshot struct {
	Samples          int64           `json:"samples"`
	Duration         LatencySnapshot `json:"duration"`
	TTFB             LatencySnapshot `json:"ttfb"`
	DOMContentLoaded LatencySnapshot `json:"domContentLoaded"`
	Latest           *PageLoadLatest `json:"latest,omitempty"`
	JS               AssetSnapshot   `json:"js"`
	CSS              AssetSnapshot   `json:"css"`
	FirstPaintAPI    []RouteTiming   `json:"firstPaintApi"`
	SlowestClientAPI []RouteTiming   `json:"slowestClientApi"`
	SlowestHTTP      []RouteTiming   `json:"slowestHttp"`
	Dominant         string          `json:"dominant"`
	Note             string          `json:"note"`
}

// RuntimeSnapshot is a point-in-time view of the Go process.
type RuntimeSnapshot struct {
	Goroutines  int    `json:"goroutines"`
	AllocBytes  uint64 `json:"allocBytes"`
	SysBytes    uint64 `json:"sysBytes"`
	HeapObjects uint64 `json:"heapObjects"`
	NumGC       uint32 `json:"numGC"`
}

// Snapshot is the document returned by GET /api/telemetry. The request that
// fetched it is not included; that request is recorded after the handler returns.
type Snapshot struct {
	Status        string           `json:"status"`
	StartedAt     string           `json:"startedAt"`
	UptimeSeconds float64          `json:"uptimeSeconds"`
	Runtime       RuntimeSnapshot  `json:"runtime"`
	HTTP          HTTPSnapshot     `json:"http"`
	Streams       StreamSnapshot   `json:"streams"`
	ScanSync      ScanSyncSnapshot `json:"scanSync"`
	Client        ClientSnapshot   `json:"client"`
	PageLoad      PageLoadSnapshot `json:"pageLoad"`
}

// NewRegistry returns an empty registry stamped with the current time.
func NewRegistry() *Registry {
	r := &Registry{
		started:       time.Now(),
		routes:        map[string]*routeStats{},
		httpLog:       map[string]*logGate{},
		byEvent:       map[string]*eventStats{},
		noSubLog:      map[string]*logGate{},
		clientLog:     map[string]*logGate{},
		clientAPI:     map[string]*apiRouteStats{},
		firstPaintAPI: map[string]*apiRouteStats{},
	}
	for _, kind := range []string{"scan", "scan_processing", "scan_processing_failed", "inventory", "scanner_mode"} {
		r.byEvent[kind] = &eventStats{}
	}
	return r
}

// SetSubscriberCount supplies the live browser-connection count. The function
// is invoked when a snapshot is taken and must not call back into the registry.
func (r *Registry) SetSubscriberCount(fn func() int) {
	r.mu.Lock()
	r.subscribers = fn
	r.mu.Unlock()
}

// ObserveHTTP records one finished request that was not an open event stream.
func (r *Registry) ObserveHTTP(route string, status int, d time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	ms := durationMs(d)
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	stats := r.route(route)
	stats.requests++
	stats.latency.observe(ms)
	r.httpRequests++
	r.httpLatency.observe(ms)

	switch {
	case status >= 500:
		stats.serverErrors++
		r.httpServerErrors++
	case status >= 400:
		stats.clientErrors++
		r.httpClientErrors++
	}

	if status >= 500 || d >= slowRequest {
		r.problems = append(r.problems, Problem{
			At:         now.UTC().Format(time.RFC3339),
			Route:      route,
			Status:     status,
			DurationMs: round1(ms),
		})
		if len(r.problems) > maxRecentProblems {
			r.problems = r.problems[len(r.problems)-maxRecentProblems:]
		}
		if emit, suppressed := r.gate(r.httpLog, route).allow(now); emit {
			log.Printf("telemetry component=http route=%q status=%d duration_ms=%.1f suppressed=%d", route, status, ms, suppressed)
		}
	}
}

// ObserveStreamOpen records that an event stream sent its response headers.
// open is the time from request start to those headers.
func (r *Registry) ObserveStreamOpen(open time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streamsOpened++
	r.streamOpenLatency.observe(durationMs(open))
}

// ObserveStreamClose records that an event stream handler returned.
func (r *Registry) ObserveStreamClose() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streamsClosed++
}

// ObservePublish records one fan-out attempt.
// subscribers is the number of browsers connected when the event was published,
// enqueued is how many accepted it, and dropped is how many were disconnected
// because their buffer was full.
func (r *Registry) ObservePublish(eventType string, subscribers, enqueued, dropped int) {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	ev := r.event(eventType)
	ev.published++
	ev.enqueued += int64(enqueued)
	ev.dropped += int64(dropped)
	r.published++
	r.enqueued += int64(enqueued)
	r.dropped += int64(dropped)

	if subscribers == 0 {
		ev.noSubscribers++
		r.noSubscribers++
		// Only log once a browser has actually connected during this process.
		// Cold unit tests publish with nobody listening; the counter still moves,
		// and the log is reserved for "a page was here and then it wasn't".
		if r.streamsOpened > 0 {
			if emit, suppressed := r.gate(r.noSubLog, eventType).allow(now); emit {
				log.Printf("telemetry component=scan_sync event=%s subscribers=0 suppressed=%d", eventType, suppressed)
			}
		}
	}
	if dropped > 0 {
		log.Printf("telemetry component=scan_sync event=%s dropped_slow_clients=%d subscribers=%d", eventType, dropped, subscribers)
	}
}

// ObserveMarshalFailure records that an event could not be encoded for delivery.
func (r *Registry) ObserveMarshalFailure(eventType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.event(eventType).marshalErrors++
	r.marshalErrors++
	log.Printf("telemetry component=scan_sync event=%s marshal_error=1", eventType)
}

// ObserveDelivery records a successful write to one browser.
// fanout is the delay from publish to the write. When hasIngest is set, ingest
// is the delay from the scan timestamp to the write.
func (r *Registry) ObserveDelivery(eventType string, fanout, ingest time.Duration, hasIngest bool) {
	fanoutMs := durationMs(fanout)
	r.mu.Lock()
	defer r.mu.Unlock()
	ev := r.event(eventType)
	ev.written++
	ev.delivery.observe(fanoutMs)
	r.written++
	r.delivery.observe(fanoutMs)
	if hasIngest {
		ingestMs := durationMs(ingest)
		ev.ingestDelivery.observe(ingestMs)
		r.ingestDelivery.observe(ingestMs)
	}
}

// ObserveWriteError records a failed write to a browser. The connection is
// about to close; events published while it is gone are not replayed.
func (r *Registry) ObserveWriteError(eventType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.event(eventType).writeErrors++
	r.writeErrors++
	log.Printf("telemetry component=scan_sync event=%s write_error=1", eventType)
}

// AcceptClientReport validates and stores one browser report.
func (r *Registry) AcceptClientReport(in InboundReport) error {
	if _, ok := clientKinds[in.Kind]; !ok {
		return errUnknownKind
	}
	if badFloat(in.DurationMs) || badFloat(in.TTFBMs) || badFloat(in.DOMContentLoadedMs) {
		return errNegativeDuration
	}
	if in.Status != 0 && (in.Status < 100 || in.Status > 599) {
		return errBadStatus
	}
	if in.Skipped < 0 || in.Skipped > maxClientSkipped {
		return errBadSkipped
	}
	if badResourceCount(in.JSResources) || badResourceCount(in.CSSResources) ||
		badResourceBytes(in.JSTransferBytes) || badResourceBytes(in.JSEncodedBytes) ||
		badResourceBytes(in.CSSTransferBytes) || badResourceBytes(in.CSSEncodedBytes) {
		return errBadResources
	}

	rep := ClientReport{
		At:                 time.Now().UTC().Format(time.RFC3339),
		Kind:               in.Kind,
		Page:               sanitizePath(in.Page),
		Route:              sanitizePath(in.Route),
		Status:             in.Status,
		DurationMs:         round1(clampMs(in.DurationMs)),
		TTFBMs:             round1(clampMs(in.TTFBMs)),
		DOMContentLoadedMs: round1(clampMs(in.DOMContentLoadedMs)),
		Message:            sanitizeMessage(in.Message),
		Skipped:            in.Skipped,
		JSResources:        in.JSResources,
		JSTransferBytes:    in.JSTransferBytes,
		JSEncodedBytes:     in.JSEncodedBytes,
		CSSResources:       in.CSSResources,
		CSSTransferBytes:   in.CSSTransferBytes,
		CSSEncodedBytes:    in.CSSEncodedBytes,
		FirstPaint:         in.Kind == "api" && in.FirstPaint,
	}
	if sessionRE.MatchString(in.Session) {
		rep.Session = in.Session
	}

	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	r.client.reports++
	remember := false
	logIt := false
	switch in.Kind {
	case "page_load":
		r.client.pageLoads++
		r.pageDuration.observe(rep.DurationMs)
		r.pageTTFB.observe(rep.TTFBMs)
		r.pageDCL.observe(rep.DOMContentLoadedMs)
		r.jsAssets.fold(rep.JSResources, rep.JSTransferBytes, rep.JSEncodedBytes)
		r.cssAssets.fold(rep.CSSResources, rep.CSSTransferBytes, rep.CSSEncodedBytes)
		copied := rep
		r.latestPage = &copied
		remember = true
	case "route":
		r.client.routes++
		remember = true
	case "js_error", "unhandled_rejection":
		r.client.errors++
		remember = true
		logIt = true
	case "api":
		r.client.apiCalls++
		failed := in.Status == 0 || in.Status >= 400
		if rep.Route != "" {
			r.observeClientAPI(r.clientAPI, rep.Route, rep.DurationMs, failed)
			if rep.FirstPaint {
				r.observeClientAPI(r.firstPaintAPI, rep.Route, rep.DurationMs, failed)
			}
		}
		if failed {
			r.client.apiErrors++
			remember = true
			logIt = true
		} else if in.DurationMs >= slowRequest.Seconds()*1000 {
			remember = true
		}
	case "sse_open":
		r.client.sseOpens++
	case "sse_error":
		r.client.sseErrors++
		remember = true
		logIt = true
	case "sse_gap":
		r.client.sseGaps++
		remember = true
		logIt = true
	}
	if remember {
		r.recent = append(r.recent, rep)
		if len(r.recent) > maxRecentReports {
			r.recent = r.recent[len(r.recent)-maxRecentReports:]
		}
	}
	if logIt {
		if emit, suppressed := r.gate(r.clientLog, in.Kind).allow(now); emit {
			log.Printf("telemetry component=client kind=%s page=%q route=%q status=%d duration_ms=%.1f skipped=%d suppressed=%d message=%q",
				rep.Kind, rep.Page, rep.Route, rep.Status, rep.DurationMs, rep.Skipped, suppressed, rep.Message)
		}
	}
	return nil
}

// Snapshot returns the current counters. The subscriber function, if set, is
// called without holding the registry lock.
func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	fn := r.subscribers
	r.mu.Unlock()

	subscribers := 0
	if fn != nil {
		subscribers = fn()
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	r.mu.Lock()
	defer r.mu.Unlock()

	routes := make([]RouteSnapshot, 0, len(r.routes))
	for name, stats := range r.routes {
		routes = append(routes, RouteSnapshot{
			Route:        name,
			Requests:     stats.requests,
			ClientErrors: stats.clientErrors,
			ServerErrors: stats.serverErrors,
			Latency:      stats.latency.snapshot(),
		})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Route < routes[j].Route })

	byEvent := make(map[string]EventSnapshot, len(r.byEvent))
	for name, stats := range r.byEvent {
		byEvent[name] = eventSnapshotOf(stats)
	}

	problems := r.problems
	if problems == nil {
		problems = []Problem{}
	} else {
		problems = append([]Problem(nil), problems...)
	}
	recent := r.recent
	if recent == nil {
		recent = []ClientReport{}
	} else {
		recent = append([]ClientReport(nil), recent...)
	}

	active := r.streamsOpened - r.streamsClosed
	if active < 0 {
		active = 0
	}

	return Snapshot{
		Status:        "ok",
		StartedAt:     r.started.UTC().Format(time.RFC3339),
		UptimeSeconds: round1(time.Since(r.started).Seconds()),
		Runtime: RuntimeSnapshot{
			Goroutines:  runtime.NumGoroutine(),
			AllocBytes:  mem.Alloc,
			SysBytes:    mem.Sys,
			HeapObjects: mem.HeapObjects,
			NumGC:       mem.NumGC,
		},
		HTTP: HTTPSnapshot{
			Requests:       r.httpRequests,
			ClientErrors:   r.httpClientErrors,
			ServerErrors:   r.httpServerErrors,
			Latency:        r.httpLatency.snapshot(),
			Routes:         routes,
			RecentProblems: problems,
		},
		Streams: StreamSnapshot{
			Opened:      r.streamsOpened,
			Closed:      r.streamsClosed,
			Active:      active,
			OpenLatency: r.streamOpenLatency.snapshot(),
		},
		ScanSync: ScanSyncSnapshot{
			Subscribers:                subscribers,
			Published:                  r.published,
			Enqueued:                   r.enqueued,
			DroppedSlowClients:         r.dropped,
			MarshalErrors:              r.marshalErrors,
			Written:                    r.written,
			WriteErrors:                r.writeErrors,
			PublishedWithNoSubscribers: r.noSubscribers,
			Delivery:                   r.delivery.snapshot(),
			IngestToDelivery:           r.ingestDelivery.snapshot(),
			ByEvent:                    byEvent,
		},
		Client: ClientSnapshot{
			Reports:      r.client.reports,
			PageLoads:    r.client.pageLoads,
			RouteChanges: r.client.routes,
			Errors:       r.client.errors,
			APICalls:     r.client.apiCalls,
			APIErrors:    r.client.apiErrors,
			SSEOpens:     r.client.sseOpens,
			SSEErrors:    r.client.sseErrors,
			SSEGaps:      r.client.sseGaps,
			Recent:       recent,
		},
		PageLoad: r.pageLoadSnapshot(routes),
	}
}

func (r *Registry) pageLoadSnapshot(httpRoutes []RouteSnapshot) PageLoadSnapshot {
	first := topClientRoutes(r.firstPaintAPI)
	slowAPI := topClientRoutes(r.clientAPI)
	slowHTTP := topHTTPRoutes(httpRoutes)
	var latest *PageLoadLatest
	if r.latestPage != nil {
		built := pageLoadLatest(*r.latestPage)
		latest = &built
	}
	dominant := dominantWait(latest, first)
	return PageLoadSnapshot{
		Samples:          r.client.pageLoads,
		Duration:         r.pageDuration.snapshot(),
		TTFB:             r.pageTTFB.snapshot(),
		DOMContentLoaded: r.pageDCL.snapshot(),
		Latest:           latest,
		JS:               r.jsAssets.snapshot(),
		CSS:              r.cssAssets.snapshot(),
		FirstPaintAPI:    first,
		SlowestClientAPI: slowAPI,
		SlowestHTTP:      slowHTTP,
		Dominant:         dominant,
		Note:             pageLoadNote(latest, first, dominant),
	}
}

func (r *Registry) observeClientAPI(m map[string]*apiRouteStats, route string, ms float64, failed bool) {
	stats := m[route]
	if stats == nil {
		// POST /api/telemetry/client is reachable without the household
		// password, so a flood of distinct routes must not grow this map.
		if len(m) >= maxClientRoutes {
			return
		}
		stats = &apiRouteStats{}
		m[route] = stats
	}
	stats.calls++
	stats.latency.observe(ms)
	if failed {
		stats.errors++
	}
}

func (a *assetMax) fold(resources int, transfer, encoded int64) {
	if resources > a.resources {
		a.resources = resources
	}
	if transfer > a.transfer {
		a.transfer = transfer
	}
	if encoded > a.encoded {
		a.encoded = encoded
	}
}

func (a assetMax) snapshot() AssetSnapshot {
	return AssetSnapshot{
		MaxResources:     a.resources,
		MaxTransferBytes: a.transfer,
		MaxEncodedBytes:  a.encoded,
	}
}

func (r *Registry) route(name string) *routeStats {
	stats := r.routes[name]
	if stats == nil {
		stats = &routeStats{}
		r.routes[name] = stats
	}
	return stats
}

func (r *Registry) event(eventType string) *eventStats {
	stats := r.byEvent[eventType]
	if stats == nil {
		stats = &eventStats{}
		r.byEvent[eventType] = stats
	}
	return stats
}

func (r *Registry) gate(m map[string]*logGate, key string) *logGate {
	g := m[key]
	if g == nil {
		g = &logGate{}
		m[key] = g
	}
	return g
}

func eventSnapshotOf(stats *eventStats) EventSnapshot {
	return EventSnapshot{
		Published:                  stats.published,
		Enqueued:                   stats.enqueued,
		DroppedSlowClients:         stats.dropped,
		MarshalErrors:              stats.marshalErrors,
		Written:                    stats.written,
		WriteErrors:                stats.writeErrors,
		PublishedWithNoSubscribers: stats.noSubscribers,
		Delivery:                   stats.delivery.snapshot(),
		IngestToDelivery:           stats.ingestDelivery.snapshot(),
	}
}

func (h *latencyHist) observe(ms float64) {
	if h.counts == nil {
		h.counts = make([]int64, len(latencyEdgesMs)+1)
	}
	h.count++
	h.sum += ms
	if ms > h.max {
		h.max = ms
	}
	idx := len(latencyEdgesMs)
	for i, edge := range latencyEdgesMs {
		if ms <= edge {
			idx = i
			break
		}
	}
	h.counts[idx]++
}

func (h latencyHist) snapshot() LatencySnapshot {
	return LatencySnapshot{
		Count: h.count,
		SumMs: round1(h.sum),
		MaxMs: round1(h.max),
		P50Ms: h.percentile(0.50),
		P95Ms: h.percentile(0.95),
		P99Ms: h.percentile(0.99),
	}
}

func (h latencyHist) percentile(p float64) float64 {
	if h.count == 0 || len(h.counts) == 0 {
		return 0
	}
	target := int64(math.Ceil(p * float64(h.count)))
	if target < 1 {
		target = 1
	}
	var cum int64
	for i, n := range h.counts {
		cum += n
		if cum >= target {
			if i >= len(latencyEdgesMs) {
				return round1(h.max)
			}
			return latencyEdgesMs[i]
		}
	}
	return round1(h.max)
}

func (g *logGate) allow(now time.Time) (bool, int) {
	if g.last.IsZero() || now.Sub(g.last) >= logInterval {
		suppressed := g.suppressed
		g.suppressed = 0
		g.last = now
		return true, suppressed
	}
	g.suppressed++
	return false, 0
}

func durationMs(d time.Duration) float64 {
	if d < 0 {
		return 0
	}
	return float64(d) / float64(time.Millisecond)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func clampMs(v float64) float64 {
	max := maxClientDuration.Seconds() * 1000
	if v > max {
		return max
	}
	return v
}

func badFloat(v float64) bool {
	return math.IsNaN(v) || math.IsInf(v, 0) || v < 0
}

func badResourceCount(n int) bool {
	return n < 0 || n > maxResourceCount
}

func badResourceBytes(n int64) bool {
	return n < 0 || n > maxResourceBytes
}

func pageLoadLatest(rep ClientReport) PageLoadLatest {
	latest := PageLoadLatest{
		At:                 rep.At,
		Page:               rep.Page,
		DurationMs:         rep.DurationMs,
		TTFBMs:             rep.TTFBMs,
		DOMContentLoadedMs: rep.DOMContentLoadedMs,
		JSResources:        rep.JSResources,
		JSTransferBytes:    rep.JSTransferBytes,
		JSEncodedBytes:     rep.JSEncodedBytes,
		CSSResources:       rep.CSSResources,
		CSSTransferBytes:   rep.CSSTransferBytes,
		CSSEncodedBytes:    rep.CSSEncodedBytes,
	}
	if latest.DOMContentLoadedMs >= latest.TTFBMs {
		latest.DocumentMs = round1(latest.DOMContentLoadedMs - latest.TTFBMs)
	}
	if latest.DurationMs > 0 && latest.DurationMs >= latest.DOMContentLoadedMs {
		latest.AfterDomMs = round1(latest.DurationMs - latest.DOMContentLoadedMs)
	}
	return latest
}

func dominantWait(latest *PageLoadLatest, first []RouteTiming) string {
	if latest == nil {
		return "unknown"
	}
	apiMs := 0.0
	for _, row := range first {
		if row.Latency.MaxMs > apiMs {
			apiMs = row.Latency.MaxMs
		}
	}
	bestName := "unknown"
	bestMs := 0.0
	for _, candidate := range []struct {
		name string
		ms   float64
	}{
		{"ttfb", latest.TTFBMs},
		{"document", latest.DocumentMs},
		{"after_dom", latest.AfterDomMs},
		{"api", apiMs},
	} {
		if candidate.ms > bestMs {
			bestName = candidate.name
			bestMs = candidate.ms
		}
	}
	return bestName
}

func pageLoadNote(latest *PageLoadLatest, first []RouteTiming, dominant string) string {
	if latest == nil {
		return "No page load has been reported yet."
	}
	note := fmt.Sprintf("Latest page load: first byte %.0fms, DOM ready %.0fms", latest.TTFBMs, latest.DOMContentLoadedMs)
	if latest.DurationMs > 0 {
		note += fmt.Sprintf(", full load %.0fms", latest.DurationMs)
	} else if latest.DOMContentLoadedMs > 0 {
		note += ", full load was not recorded"
	}
	switch dominant {
	case "document":
		note += fmt.Sprintf(". Most of the wait is the document after the first byte (%.0fms)", latest.DocumentMs)
	case "ttfb":
		note += ". Most of the wait is time to first byte"
	case "after_dom":
		note += fmt.Sprintf(". Most of the wait is after DOM ready (%.0fms)", latest.AfterDomMs)
	case "api":
		route, ms := slowestFirstPaint(first)
		if route != "" {
			note += fmt.Sprintf(". Most of the wait is the first API call %s (%.0fms)", route, ms)
		}
	}
	return note + "."
}

func slowestFirstPaint(first []RouteTiming) (string, float64) {
	var route string
	var ms float64
	for _, row := range first {
		if row.Latency.MaxMs > ms {
			route = row.Route
			ms = row.Latency.MaxMs
		}
	}
	return route, ms
}

func topClientRoutes(m map[string]*apiRouteStats) []RouteTiming {
	rows := make([]RouteTiming, 0, len(m))
	for route, stats := range m {
		rows = append(rows, RouteTiming{
			Route:   route,
			Calls:   stats.calls,
			Errors:  stats.errors,
			Latency: stats.latency.snapshot(),
		})
	}
	sortRouteTimings(rows)
	if len(rows) > maxSlowRoutes {
		rows = rows[:maxSlowRoutes]
	}
	return rows
}

func topHTTPRoutes(routes []RouteSnapshot) []RouteTiming {
	rows := make([]RouteTiming, 0, len(routes))
	for _, route := range routes {
		if route.Requests == 0 {
			continue
		}
		rows = append(rows, RouteTiming{
			Route:   route.Route,
			Calls:   route.Requests,
			Errors:  route.ClientErrors + route.ServerErrors,
			Latency: route.Latency,
		})
	}
	sortRouteTimings(rows)
	if len(rows) > maxSlowRoutes {
		rows = rows[:maxSlowRoutes]
	}
	return rows
}

func sortRouteTimings(rows []RouteTiming) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Latency.P95Ms != rows[j].Latency.P95Ms {
			return rows[i].Latency.P95Ms > rows[j].Latency.P95Ms
		}
		if rows[i].Latency.MaxMs != rows[j].Latency.MaxMs {
			return rows[i].Latency.MaxMs > rows[j].Latency.MaxMs
		}
		return rows[i].Route < rows[j].Route
	})
}

func sanitizePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		raw = u.Path
	}
	if len(raw) > maxPathLen {
		raw = raw[:maxPathLen]
	}
	return raw
}

func sanitizeMessage(s string) string {
	s = queryInText.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxMessageLen {
		s = s[:maxMessageLen]
	}
	return s
}
