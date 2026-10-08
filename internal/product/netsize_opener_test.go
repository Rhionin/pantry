package product

import (
	"bytes"
	"context"
	"io"
	"math"
	"net/http"
	"testing"
	"time"
)

func TestLookupBarcodeReadsOpenFoodFactsQuantity(t *testing.T) {
	body := `{
		"status": 1,
		"product": {
			"product_name": "Campbell's Condensed Cream of Mushroom Soup",
			"categories": "Soups",
			"quantity": "10.5 oz (298 g)",
			"product_quantity": 297.67,
			"product_quantity_unit": "g"
		}
	}`
	client := NewProductOpenerClientWithHTTPClient(ExternalSourceOpenFoodFacts, "https://world.openfoodfacts.org/api/v2/product", &http.Client{
		Timeout: 10 * time.Second,
		Transport: mockTransport{fn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(body))),
				Header:     make(http.Header),
			}, nil
		}},
	})
	got, err := client.LookupBarcode(context.Background(), "051000012349")
	if err != nil {
		t.Fatal(err)
	}
	if got.NetDimension != DimensionMass || got.NetBaseValue == nil || math.Abs(*got.NetBaseValue-297.67) > 0.0001 {
		t.Fatalf("size = %+v", got)
	}
	if got.PackCount != nil {
		t.Fatalf("single can should not store a pack count, got %d", *got.PackCount)
	}

	multipack := `{
		"status": 1,
		"product": {
			"product_name": "Seltzer",
			"quantity": "6 x 12 fl oz",
			"product_quantity": "2130",
			"product_quantity_unit": "ml"
		}
	}`
	client = NewProductOpenerClientWithHTTPClient(ExternalSourceOpenFoodFacts, "https://world.openfoodfacts.org/api/v2/product", &http.Client{
		Timeout: 10 * time.Second,
		Transport: mockTransport{fn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(multipack))),
				Header:     make(http.Header),
			}, nil
		}},
	})
	got, err = client.LookupBarcode(context.Background(), "051000012350")
	if err != nil {
		t.Fatal(err)
	}
	if got.PackCount == nil || *got.PackCount != 6 || got.NetDimension != DimensionVolume {
		t.Fatalf("multipack = %+v pack %v", got, got.PackCount)
	}
}
