package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
)

// TestHeadlessScanEntryShape locks in Requirement 3.4: a scan entry created
// through the headless path (scan.NewEntryFromLookup + Queue.CreateScanEntry,
// with no HTTP request or terminal input involved — the same construction
// ScanListener uses) is returned by GET /api/scans with the same field set
// and value shapes as a browser-created entry, differing only in userId.
func TestHeadlessScanEntryShape(t *testing.T) {
	const headlessUserID = "headless-user"
	const browserUserID = "browser-user"
	const headlessBarcode = "222333444555"
	const browserBarcode = "333444555666"

	tests := []handlerTestCase{
		{
			name: "headless and browser created entries have identical field sets",
			setup: func(env testEnv) {
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-headless", Name: "Headless Product", Category: "Test", UnitOfMeasure: "unit",
				}); err != nil {
					env.T.Fatalf("CreateProduct (headless): %v", err)
				}
				if err := env.ProductStore.CreateProduct(context.Background(), product.Product{
					ID: "prod-browser", Name: "Browser Product", Category: "Test", UnitOfMeasure: "unit",
				}); err != nil {
					env.T.Fatalf("CreateProduct (browser): %v", err)
				}
				if err := env.ProductStore.UpsertBarcodeMapping(context.Background(), browserBarcode, "prod-browser", "global", ""); err != nil {
					env.T.Fatalf("UpsertBarcodeMapping (browser): %v", err)
				}

				// Headless-created entry: built directly through
				// scan.NewEntryFromLookup and a Queue backed by env.DB, standing in
				// for ScanListener's construction path with no real terminal input.
				lookup := product.LookupResult{
					Product: &product.ProductSummary{ID: "prod-headless", Name: "Headless Product", Category: "Test", UnitOfMeasure: "unit"},
					Source:  "global",
				}
				direction := scan.StockIn
				entry := scan.NewEntryFromLookup(headlessUserID, headlessBarcode, lookup, &direction, time.Now())
				scanQueue := scan.NewQueue(env.DB)
				if _, err := scanQueue.CreateScanEntry(context.Background(), entry); err != nil {
					env.T.Fatalf("CreateScanEntry (headless): %v", err)
				}
			},
			// Browser-created entry: the normal POST /api/scans path.
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"` + browserBarcode + `","direction":"stock_in","userId":"` + browserUserID + `"}`,
				expectedStatus: http.StatusCreated,
			},
			afterRequest: func(env testEnv) {
				headless := getScanEntry(env, headlessUserID)
				browser := getScanEntry(env, browserUserID)

				assertSameShape(env.T, headless, browser)
			},
		},
	}

	runHandlerTests(t, tests)
}

// getScanEntry issues GET /api/scans?userId=... against a fresh handler bound
// to the same DB/catalog the primary request used, and returns the single
// matching entry decoded as a generic JSON object.
func getScanEntry(env testEnv, userID string) map[string]any {
	env.T.Helper()

	var now func() time.Time
	if env.Clock != nil {
		now = env.Clock.Now
	}
	handler, _ := NewHandler(env.ProductStore, &product.LookupService{
		Catalog:       env.ProductStore,
		OpenFoodFacts: env.OpenFoodFacts,
		Refresher:     env.Refresher,
		Now:           now,
	}, env.Refresher, env.DB)

	server := httptest.NewServer(handler)
	defer server.Close()

	res, err := http.Get(server.URL + "/api/scans?userId=" + userID)
	if err != nil {
		env.T.Fatalf("GET /api/scans?userId=%s: %v", userID, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		env.T.Fatalf("GET /api/scans?userId=%s: want 200, got %d", userID, res.StatusCode)
	}

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		env.T.Fatalf("read body for userId=%s: %v", userID, err)
	}

	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		env.T.Fatalf("decode body for userId=%s: %v (body: %s)", userID, err, raw)
	}
	if len(entries) != 1 {
		env.T.Fatalf("GET /api/scans?userId=%s: want 1 entry, got %d", userID, len(entries))
	}
	return entries[0]
}

// assertSameShape asserts two decoded scan entries have identical field sets
// and, per field, identical value shapes (the same Go type once decoded from
// JSON) — except userId, which is expected to differ by construction.
func assertSameShape(t *testing.T, a, b map[string]any) {
	t.Helper()

	if len(a) != len(b) {
		t.Fatalf("field count differs: headless has %d fields %v, browser has %d fields %v", len(a), fieldNames(a), len(b), fieldNames(b))
	}

	for field, aVal := range a {
		bVal, ok := b[field]
		if !ok {
			t.Errorf("field %q present in headless entry but missing from browser entry", field)
			continue
		}
		if field == "userId" {
			continue
		}
		if aType, bType := reflect.TypeOf(aVal), reflect.TypeOf(bVal); aType != bType {
			t.Errorf("field %q shape differs: headless %v (%T), browser %v (%T)", field, aVal, aVal, bVal, bVal)
		}
	}

	for field := range b {
		if _, ok := a[field]; !ok {
			t.Errorf("field %q present in browser entry but missing from headless entry", field)
		}
	}
}

func fieldNames(m map[string]any) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return names
}
