// Package product provides product and barcode management functionality.
package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

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

	// 404 means product not found; return the sentinel error.
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrProductNotFound
	}

	// Other non-2xx status codes are not retryable.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("OpenFoodFacts returned status %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("OpenFoodFacts returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the JSON response.
	var data offResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode OpenFoodFacts response: %w", err)
	}

	// Check if product exists in response.
	if data.Status != 1 || data.Product.Name == "" {
		return nil, ErrProductNotFound
	}

	// Convert the response to ProductSummary.
	// Product ID is the barcode itself as a placeholder.
	ps := &ProductSummary{
		ID:            barcode,
		Name:          data.Product.Name,
		Category:      data.Product.Category,
		UnitOfMeasure: "", // OFF doesn't provide this; will be set by caller if needed.
	}
	return ps, nil
}

// shouldRetryIncluding429 wraps the default retry logic and adds 429 (rate-limit) handling.
// This ensures we retry on rate-limit errors in addition to the default retryable conditions.
func shouldRetryIncluding429(attempt retryhttp.Attempt) bool {
	// Always retry on 429 (Too Many Requests / rate-limit).
	if attempt.Res != nil && attempt.Res.StatusCode == http.StatusTooManyRequests {
		return true
	}
	// For everything else, use the library's default logic which handles:
	// - DNS errors
	// - Timeout errors (for idempotent methods)
	// - 502 Bad Gateway and 503 Service Unavailable (for idempotent methods)
	// - Retry-After header presence
	return retryhttp.DefaultShouldRetryFn(attempt)
}
