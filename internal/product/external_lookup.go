package product

import (
	"context"
	"errors"
	"log"
	"sync"
)

// FanOutOutcome classifies a completed Tier-3 fan-out.
type FanOutOutcome string

const (
	// FanOutHit means at least one database supplied a usable record.
	FanOutHit FanOutOutcome = "hit"

	// FanOutConfirmedMiss means every database reported the barcode unknown
	// and none failed. This is the only outcome that may be cached.
	FanOutConfirmedMiss FanOutOutcome = "confirmed_miss"

	// FanOutUnresolved means no database supplied a record and at least one
	// failed, so the barcode's true status is unknown and must be retried.
	FanOutUnresolved FanOutOutcome = "unresolved"
)

// FanOutResult is the single value ExternalLookup reports to LookupService.
type FanOutResult struct {
	Outcome FanOutOutcome

	// Product and Source are set if and only if Outcome is FanOutHit.
	Product *ProductSummary
	Source  ExternalSource
}

// BarcodeLookup is the interface ExternalLookup uses to query a single database.
type BarcodeLookup interface {
	LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error)
}

// barcodeLookup is an alias for compatibility
type barcodeLookup = BarcodeLookup

// upstreamReply holds the result of one database query in a fan-out.
type upstreamReply struct {
	source  ExternalSource
	product *ProductSummary
	err     error
}

// errNoUpstreamClient indicates that a database was expected but is absent from
// the Clients map. This is an error, never a miss, so a misconfigured deployment
// cannot manufacture a confirmed miss.
var errNoUpstreamClient = errors.New("upstream client not configured")

// ExternalLookup queries the Product Opener databases. It is the fan-out for
// Tier 3 and the per-database addressable lookup for Refresher.
type ExternalLookup struct {
	// Clients holds one client per database. A database absent from this map
	// is treated as an upstream error, never as an upstream miss: a
	// misconfigured deployment must not be able to manufacture a
	// confirmed miss.
	Clients map[ExternalSource]BarcodeLookup
}

// NewExternalLookup builds an ExternalLookup from a map of clients, one per database.
func NewExternalLookup(clients map[ExternalSource]BarcodeLookup) *ExternalLookup {
	return &ExternalLookup{
		Clients: clients,
	}
}

// Lookup queries every database concurrently and reports one outcome.
// It returns the first hit in database precedence order, or a confirmed miss if
// all databases report unknown with no errors, or unresolved if any error occurs
// without a hit.
func (l *ExternalLookup) Lookup(ctx context.Context, barcode string) FanOutResult {
	// One slot per database, indexed by precedence position. Each goroutine
	// writes only its own slot, so there is no shared mutable state and no
	// mutex — and because the slots are ordered by precedence, selecting the
	// winner is a scan for the first hit rather than a comparison.
	replies := make([]upstreamReply, len(databasePrecedence))

	var wg sync.WaitGroup
	for i, source := range databasePrecedence {
		client, ok := l.Clients[source]
		if !ok {
			replies[i] = upstreamReply{source: source, err: errNoUpstreamClient}
			continue
		}
		wg.Add(1)
		go func(i int, source ExternalSource, client BarcodeLookup) {
			defer wg.Done()
			p, err := client.LookupBarcode(ctx, barcode)
			replies[i] = upstreamReply{source: source, product: p, err: err}
		}(i, source, client)
	}

	// Every goroutine this call started has returned before Lookup does.
	// No detached work, no leak, and no need for a Wait() API on the fan-out.
	wg.Wait()

	return classifyFanOut(barcode, replies)
}

// LookupIn queries exactly one database, for a revalidation that already
// knows where a row's data came from.
func (l *ExternalLookup) LookupIn(ctx context.Context, source ExternalSource, barcode string) (*ProductSummary, error) {
	client, ok := l.Clients[source]
	if !ok {
		return nil, errNoUpstreamClient
	}
	return client.LookupBarcode(ctx, barcode)
}

// classifyFanOut scans replies in precedence order and returns on the first hit.
// A hit outranks a later miss and a concurrent error. An error anywhere with no
// hit downgrades a would-be confirmed miss to unresolved.
func classifyFanOut(barcode string, replies []upstreamReply) FanOutResult {
	sawError := false
	for _, r := range replies {
		switch {
		case r.err == nil && r.product != nil:
			// First hit in precedence order wins; remaining replies are
			// discarded, including errors. A usable answer is a usable answer.
			return FanOutResult{Outcome: FanOutHit, Product: r.product, Source: r.source}
		case errors.Is(r.err, ErrProductNotFound):
			// Upstream miss: this database is confident it does not know.
		default:
			// Upstream error: one log line naming barcode, database, reason.
			log.Printf("barcode %s lookup against %s failed: %v", barcode, r.source, r.err)
			sawError = true
		}
	}
	if sawError {
		return FanOutResult{Outcome: FanOutUnresolved}
	}
	return FanOutResult{Outcome: FanOutConfirmedMiss}
}

// UpstreamDatabases is the union both LookupService and Refresher are
// satisfied by, so one value serves both and the DISABLE switch cannot
// disable one without the other.
type UpstreamDatabases interface {
	Lookup(ctx context.Context, barcode string) FanOutResult
	LookupIn(ctx context.Context, source ExternalSource, barcode string) (*ProductSummary, error)
}
