package server

import (
	"net/http"
	"testing"
)

// TestScanCreateHandlerLookupOutcomes locks in the found/not-found behavior of
// ScanCreateHandler.Handle after its task 6.3 refactor to call
// scan.NewEntryFromLookup: a found product still yields status "pending" with
// the matched productId, and a lookup miss still yields status "flagged" with
// a nil productId.
func TestScanCreateHandlerLookupOutcomes(t *testing.T) {
	tests := []handlerTestCase{
		{
			name:  "found product yields pending status with productId",
			setup: setupProductWithBarcode("prod-scan-found", "Found Product", "Test", "111222333444"),
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"111222333444","userId":"user-scan-found"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.status", value: "pending"},
					{path: "$.productId", value: "prod-scan-found"},
				},
			},
		},
		{
			name: "no matching product yields flagged status with nil productId",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"000111222333","userId":"user-scan-notfound"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.status", value: "flagged"},
					{path: "$.productId", value: nil},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}

// TestScanCreateHandler_ControlBarcodeCurrentlyFlagged is a FEAT-001
// characterization test documenting the backend side of the browser
// control-barcode gap: POST /api/scans with the reserved STOCK_IN control
// barcode currently creates an ordinary flagged scan entry (no product match)
// and performs NO mode switch, because ScanCreateHandler does not classify
// control barcodes the way the headless scanlistener does. This asserts the
// present (buggy) behavior on purpose and MUST be updated in FEAT-002 once the
// browser path gains a mode-switch endpoint that classifies STOCK_IN/STOCK_OUT
// instead of enqueuing them as product scans.
func TestScanCreateHandler_ControlBarcodeCurrentlyFlagged(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "STOCK_IN control barcode is currently enqueued as a flagged scan (no mode switch) - FEAT-002 will change this",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"STOCK_IN","userId":"user-scan-control"}`,
				expectedStatus: http.StatusCreated,
				assertions: []assertion{
					{path: "$.status", value: "flagged"},
					{path: "$.productId", value: nil},
					{path: "$.barcode", value: "STOCK_IN"},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}

// TestScanCreateHandler_MergeBehavior locks in that a repeat POST /api/scans
// for the same barcode/userId/direction while the first entry is still
// pending or flagged merges into the existing entry (unitCount incremented)
// instead of creating a duplicate row, through the full HTTP path.
func TestScanCreateHandler_MergeBehavior(t *testing.T) {
	tests := []handlerTestCase{
		{
			name:  "repeat scan while pending merges into existing entry with unitCount 2",
			setup: setupProductWithBarcode("prod-scan-merge-pending", "Merge Pending Product", "Test", "222333444555"),
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"222333444555","userId":"user-scan-merge-pending","direction":"stock_in"}`,
				expectedStatus: http.StatusCreated,
			},
			afterRequest: exchanges(
				httpExchange{
					method:         "POST",
					path:           "/api/scans",
					body:           `{"barcode":"222333444555","userId":"user-scan-merge-pending","direction":"stock_in"}`,
					expectedStatus: http.StatusCreated,
				},
				httpExchange{
					method:         "GET",
					path:           "/api/scans",
					query:          map[string]string{"userId": "user-scan-merge-pending"},
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].status", value: "pending"},
						{path: "$[0].unitCount", value: float64(2)},
					},
				},
			),
		},
		{
			name: "repeat scan while flagged merges into existing entry with unitCount 2",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/scans",
				body:           `{"barcode":"333444555666","userId":"user-scan-merge-flagged","direction":"stock_in"}`,
				expectedStatus: http.StatusCreated,
			},
			afterRequest: exchanges(
				httpExchange{
					method:         "POST",
					path:           "/api/scans",
					body:           `{"barcode":"333444555666","userId":"user-scan-merge-flagged","direction":"stock_in"}`,
					expectedStatus: http.StatusCreated,
				},
				httpExchange{
					method:         "GET",
					path:           "/api/scans",
					query:          map[string]string{"userId": "user-scan-merge-flagged"},
					expectedStatus: http.StatusOK,
					assertions: []assertion{
						{path: "$[0].status", value: "flagged"},
						{path: "$[0].unitCount", value: float64(2)},
					},
				},
			),
		},
	}

	runHandlerTests(t, tests)
}
