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
