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
