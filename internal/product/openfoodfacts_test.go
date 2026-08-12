package product

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/justinrixx/retryhttp"
)

// mockTransport wraps a function as an http.RoundTripper for testing.
type mockTransport struct {
	fn func(*http.Request) (*http.Response, error)
}

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.fn(req)
}

func TestLookupBarcode(t *testing.T) {
	tests := []struct {
		name         string
		barcode      string
		statusCode   int
		responseBody string
		wantErr      error
		checkProduct func(*testing.T, *ProductSummary)
	}{
		{
			name:       "success",
			barcode:    "012345678905",
			statusCode: 200,
			responseBody: `{
				"status": 1,
				"product": {
					"product_name": "Coca-Cola",
					"categories": "Beverages",
					"code": "012345678905"
				}
			}`,
			checkProduct: func(t *testing.T, ps *ProductSummary) {
				if ps == nil {
					t.Fatal("expected ProductSummary, got nil")
				}
				if ps.Name != "Coca-Cola" {
					t.Errorf("expected name 'Coca-Cola', got %q", ps.Name)
				}
				if ps.Category != "Beverages" {
					t.Errorf("expected category 'Beverages', got %q", ps.Category)
				}
				if ps.ID != "012345678905" {
					t.Errorf("expected ID '012345678905', got %q", ps.ID)
				}
			},
		},
		{
			name:       "not found - 404",
			barcode:    "999999999999",
			statusCode: http.StatusNotFound,
			wantErr:    ErrProductNotFound,
		},
		{
			name:       "not found - invalid status",
			barcode:    "012345678905",
			statusCode: 200,
			responseBody: `{
				"status": 0,
				"product": {}
			}`,
			wantErr: ErrProductNotFound,
		},
		{
			name:       "not found - empty name",
			barcode:    "012345678905",
			statusCode: 200,
			responseBody: `{
				"status": 1,
				"product": {
					"product_name": "",
					"categories": "Test"
				}
			}`,
			wantErr: ErrProductNotFound,
		},
		{
			name:    "empty barcode",
			barcode: "",
			wantErr: errors.New("barcode cannot be empty"),
		},
		{
			name:         "malformed JSON",
			barcode:      "012345678905",
			statusCode:   200,
			responseBody: "invalid json",
			wantErr:      errors.New("failed to decode"),
		},
		{
			name:       "server error",
			barcode:    "012345678905",
			statusCode: http.StatusInternalServerError,
			wantErr:    errors.New("OpenFoodFacts returned status 500"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip HTTP setup for validation-only tests
			if tt.barcode == "" {
				client := NewOpenFoodFactsClient()
				_, err := client.LookupBarcode(context.Background(), tt.barcode)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}

			mock := mockTransport{
				fn: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: tt.statusCode,
						Body:       io.NopCloser(bytes.NewReader([]byte(tt.responseBody))),
						Header:     make(http.Header),
					}, nil
				},
			}

			client := NewOpenFoodFactsClientWithClient(&http.Client{
				Timeout:   10 * time.Second,
				Transport: mock,
			})

			ps, err := client.LookupBarcode(context.Background(), tt.barcode)

			// Check error
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
				if ps != nil {
					t.Errorf("expected nil ProductSummary on error, got %v", ps)
				}
				return
			}

			// Check success
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkProduct != nil {
				tt.checkProduct(t, ps)
			}
		})
	}
}

func TestShouldRetryIncluding429(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		method     string
		want       bool
	}{
		{
			name:       "retry on 429",
			statusCode: http.StatusTooManyRequests,
			method:     http.MethodGet,
			want:       true,
		},
		{
			name:       "retry on 503 for GET (delegates to default)",
			statusCode: http.StatusServiceUnavailable,
			method:     http.MethodGet,
			want:       true,
		},
		{
			name:       "do not retry on 404",
			statusCode: http.StatusNotFound,
			method:     http.MethodGet,
			want:       false,
		},
		{
			name:       "do not retry on 400",
			statusCode: http.StatusBadRequest,
			method:     http.MethodGet,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt := retryhttp.Attempt{
				Req: &http.Request{
					Method: tt.method,
				},
				Res: &http.Response{
					StatusCode: tt.statusCode,
				},
			}

			got := shouldRetryIncluding429(attempt)
			if got != tt.want {
				t.Errorf("shouldRetryIncluding429() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProductLookupClientInterface(t *testing.T) {
	var client ProductLookupClient = NewOpenFoodFactsClient()
	if client == nil {
		t.Fatal("OpenFoodFactsClient should implement ProductLookupClient")
	}
}
