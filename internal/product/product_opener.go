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

// ExternalSource identifies which Product Opener database supplied a product's
// field values. The empty value means "not externally sourced, or sourced
// before provenance was recorded".
type ExternalSource string

const (
	ExternalSourceOpenFoodFacts     ExternalSource = "openfoodfacts"
	ExternalSourceOpenProductsFacts ExternalSource = "openproductsfacts"
	ExternalSourceOpenBeautyFacts   ExternalSource = "openbeautyfacts"
	ExternalSourceOpenPetFoodFacts  ExternalSource = "openpetfoodfacts"
)

// Valid reports whether s is storable. The empty value is valid and means
// "no external provenance". SQLite cannot enforce this set with a CHECK
// constraint added via ALTER TABLE, so CreateProduct enforces it in Go,
// exactly as it does for products.source.
func (s ExternalSource) Valid() bool {
	switch s {
	case "", ExternalSourceOpenFoodFacts, ExternalSourceOpenProductsFacts,
		ExternalSourceOpenBeautyFacts, ExternalSourceOpenPetFoodFacts:
		return true
	}
	return false
}

// productOpenerBaseURLs maps each database to its API root. Adding a fifth
// Product Opener database is an entry here plus an entry in databasePrecedence.
var productOpenerBaseURLs = map[ExternalSource]string{
	ExternalSourceOpenFoodFacts:     "https://world.openfoodfacts.org/api/v2/product",
	ExternalSourceOpenProductsFacts: "https://world.openproductsfacts.org/api/v2/product",
	ExternalSourceOpenBeautyFacts:   "https://world.openbeautyfacts.org/api/v2/product",
	ExternalSourceOpenPetFoodFacts:  "https://world.openpetfoodfacts.org/api/v2/product",
}

// databasePrecedence resolves a barcode present in more than one upstream
// database. The fan-out iterates this slice and returns the first hit, so
// changing the ordering is changing this literal — there is no branching
// logic to find. Ordered by expected pantry-scan frequency.
var databasePrecedence = []ExternalSource{
	ExternalSourceOpenFoodFacts,
	ExternalSourceOpenProductsFacts,
	ExternalSourceOpenBeautyFacts,
	ExternalSourceOpenPetFoodFacts,
}

// ProductOpenerClient queries one Product Opener database. All four databases
// serve the same read API shape and differ only in host, so one type with a
// construction-time base URL covers all of them.
type ProductOpenerClient struct {
	source     ExternalSource
	baseURL    string
	httpClient *http.Client
}

// NewProductOpenerClient builds a client for one database with the standard
// retry transport and a 10 second timeout.
func NewProductOpenerClient(source ExternalSource, baseURL string) *ProductOpenerClient {
	transport := retryhttp.New(
		retryhttp.WithShouldRetryFn(shouldRetryIncluding429),
	)

	return &ProductOpenerClient{
		source:  source,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// NewProductOpenerClientWithHTTPClient builds a client with a caller-supplied
// HTTP client, for tests that stand a fake server in front of a database.
func NewProductOpenerClientWithHTTPClient(source ExternalSource, baseURL string, hc *http.Client) *ProductOpenerClient {
	return &ProductOpenerClient{
		source:     source,
		baseURL:    baseURL,
		httpClient: hc,
	}
}

// Source reports which database this client queries.
func (c *ProductOpenerClient) Source() ExternalSource {
	return c.source
}

// BaseURL reports the URL this client queries, so a test can confirm a fake
// server stands in for the intended database.
func (c *ProductOpenerClient) BaseURL() string {
	return c.baseURL
}

// DefaultProductOpenerClients builds one client per database, each with the
// same retry transport and timeout.
func DefaultProductOpenerClients() map[ExternalSource]BarcodeLookup {
	clients := make(map[ExternalSource]BarcodeLookup)
	for source, baseURL := range productOpenerBaseURLs {
		clients[source] = NewProductOpenerClient(source, baseURL)
	}
	return clients
}

// productOpenerResponse represents the JSON response structure from Product Opener API.
type productOpenerResponse struct {
	Product struct {
		Name          string `json:"product_name"`
		Category      string `json:"categories"`
		Code          string `json:"code"`
		ImageThumbURL string `json:"image_front_small_url"`
	} `json:"product"`
	Status int `json:"status"`
}

// LookupBarcode looks up a product by barcode from a Product Opener database.
// Uses retryhttp with exponential backoff for rate-limit (429) and transient errors.
//
// Returns:
//   - (*ProductSummary, nil) if product is found
//   - (nil, ErrProductNotFound) if product is not found (404 or invalid response)
//   - (nil, error) if an error occurs
//
// ProductSummary.ID is set from the requested barcode, never from the response's
// code field. This preserves the invariant that an external row's product ID is
// its barcode.
func (c *ProductOpenerClient) LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error) {
	if barcode == "" {
		return nil, fmt.Errorf("barcode cannot be empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s.json", c.baseURL, barcode), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
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
			return nil, fmt.Errorf("Product Opener database returned status %d (failed to read body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("Product Opener database returned status %d: %s", resp.StatusCode, string(body))
	}

	var data productOpenerResponse
	if err := json.UnmarshalRead(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("failed to decode Product Opener response: %w", err)
	}

	if data.Status != 1 || data.Product.Name == "" {
		return nil, ErrProductNotFound
	}

	ps := &ProductSummary{
		ID:            barcode,
		Name:          data.Product.Name,
		Category:      data.Product.Category,
		UnitOfMeasure: "",
		ImageURL:      data.Product.ImageThumbURL,
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
