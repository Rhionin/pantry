// Package product provides product and barcode management functionality.
package product

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-json-experiment/json"
	"github.com/justinrixx/retryhttp"
)

var ErrProductNotFound = errors.New("product not found")

// OpenFoodFactsClient queries the Open Food Facts API for product information.
type OpenFoodFactsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewOpenFoodFactsClient creates a new Open Food Facts API client.
// Uses retryhttp with default settings plus 429 (rate-limit) retry support.
func NewOpenFoodFactsClient() *OpenFoodFactsClient {
	transport := retryhttp.New(
		retryhttp.WithShouldRetryFn(shouldRetryIncluding429),
	)

	return &OpenFoodFactsClient{
		baseURL: "https://world.openfoodfacts.org/api/v2/product",
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// NewOpenFoodFactsClientWithClient creates a new Open Food Facts API client
// with a custom HTTP client. Useful for testing.
func NewOpenFoodFactsClientWithClient(client *http.Client) *OpenFoodFactsClient {
	return &OpenFoodFactsClient{
		baseURL:    "https://world.openfoodfacts.org/api/v2/product",
		httpClient: client,
	}
}

// offResponse represents the JSON response structure from Open Food Facts API.
type offResponse struct {
	Product struct {
		Name     string `json:"product_name"`
		Category string `json:"categories"`
		Code     string `json:"code"`
	} `json:"product"`
	Status int `json:"status"`
}

// LookupBarcode looks up a product by barcode from the Open Food Facts API.
// Uses retryhttp with exponential backoff for rate-limit (429) and transient errors.
//
// Returns:
//   - (*ProductSummary, nil) if product is found
//   - (nil, ErrProductNotFound) if product is not found (404 or invalid response)
//   - (nil, error) if an error occurs
func (c *OpenFoodFactsClient) LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error) {
	if barcode == "" {
		return nil, fmt.Errorf("barcode cannot be empty")
	}

	resp, err := c.httpClient.Get(fmt.Sprintf("%s/%s.json", c.baseURL, barcode))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrProductNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("OpenFoodFacts returned status %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("OpenFoodFacts returned status %d: %s", resp.StatusCode, string(body))
	}

	var data offResponse
	if err := json.UnmarshalRead(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("failed to decode OpenFoodFacts response: %w", err)
	}

	if data.Status != 1 || data.Product.Name == "" {
		return nil, ErrProductNotFound
	}

	ps := &ProductSummary{
		ID:            barcode,
		Name:          data.Product.Name,
		Category:      data.Product.Category,
		UnitOfMeasure: "",
	}
	return ps, nil
}

// shouldRetryIncluding429 wraps the default retry logic and adds 429 (rate-limit) handling.
func shouldRetryIncluding429(attempt retryhttp.Attempt) bool {
	if attempt.Res != nil && attempt.Res.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return retryhttp.DefaultShouldRetryFn(attempt)
}
