package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/Rhionin/pantry/internal/telemetry"
)

// publishObserver adapts the broadcaster's callbacks onto the telemetry registry.
type publishObserver struct {
	reg *telemetry.Registry
}

func (o publishObserver) Published(eventType string, subscribers, enqueued, dropped int) {
	if o.reg == nil {
		return
	}
	o.reg.ObservePublish(eventType, subscribers, enqueued, dropped)
}

func (o publishObserver) MarshalFailed(eventType string) {
	if o.reg == nil {
		return
	}
	o.reg.ObserveMarshalFailure(eventType)
}

// observeHTTP records latency and status for finished requests without
// changing the response. Event streams are tracked separately: their lifetime
// is the connection, not a page load.
func observeHTTP(reg *telemetry.Registry, next http.Handler) http.Handler {
	if reg == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		rec.onHeader = func(status int, at time.Time) {
			if r.Pattern == "GET /api/events" && status == http.StatusOK {
				reg.ObserveStreamOpen(at.Sub(start))
			}
		}
		next.ServeHTTP(rec, r)
		if r.Pattern == "GET /api/events" && rec.status == http.StatusOK {
			reg.ObserveStreamClose()
			return
		}
		reg.ObserveHTTP(routeKey(r), rec.status, time.Since(start))
	})
}

func routeKey(r *http.Request) string {
	if r.Pattern != "" {
		if strings.Contains(r.Pattern, " ") {
			return r.Pattern
		}
		return r.Method + " " + r.Pattern
	}
	return r.Method + " unmatched"
}

// recordingResponseWriter captures the status code and the moment headers are
// sent. It forwards Flush and Unwrap so event streams and http.ResponseController
// keep working through the wrapper.
type recordingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	onHeader    func(status int, at time.Time)
}

func (w *recordingResponseWriter) WriteHeader(code int) {
	w.note(code)
	w.ResponseWriter.WriteHeader(code)
}

func (w *recordingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.note(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *recordingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *recordingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *recordingResponseWriter) note(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	if w.onHeader != nil {
		w.onHeader(status, time.Now())
	}
}
