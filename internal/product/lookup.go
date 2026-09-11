package product

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// LookupResult represents the outcome of a three-tier product lookup.
type LookupResult struct {
	// Product is the single matching product, or nil if not found.
	Product *ProductSummary `json:"product"`

	// Source indicates where the result came from: "user_override", "global", "external", or empty when not found.
	Source string `json:"source"`
}

// IsFound returns true if the lookup found a product.
func (r LookupResult) IsFound() bool {
	return r.Product != nil
}

// LookupService orchestrates three-tier product lookup: user overrides first,
// global DB second, Open Food Facts API third.
type LookupService struct {
	Catalog interface {
		LookupByBarcode(ctx context.Context, barcode, userID string) (*ProductSummary, error)
		GetProductByID(ctx context.Context, id string) (*Product, error)
		CreateProduct(ctx context.Context, product Product) error
		UpsertBarcodeMapping(ctx context.Context, barcode, productID, source, userID string) error
	}
	OpenFoodFacts interface {
		LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error)
	}

	// Refresher schedules background revalidation of stale cached rows.
	// Nil disables revalidation, so every existing LookupService literal
	// keeps compiling and behaving as it does today.
	Refresher interface {
		ScheduleRefresh(ctx context.Context, productID string)
	}

	// Now supplies the current time. Defaults to time.Now when nil.
	Now func() time.Time
}

// now returns the current time, defaulting to time.Now so existing
// construction sites that leave Now nil behave unchanged.
func (s *LookupService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Lookup performs a three-tier product lookup for the given barcode and user.
//
// Priority order:
//  1. User overrides for this specific user
//  2. Global database entries
//  3. Open Food Facts external API
//
// Returns:
//   - LookupResult with Product set if a match is found
//   - LookupResult with IsFound() == false if no product is found anywhere
//   - error if a non-recoverable error occurs
//
// Note: The UNIQUE (barcode, source, user_id) constraint in the schema prevents
// multiple products mapping to the same barcode within a source. Barcode conflicts
// are resolved via the flagged entry workflow where users can create overrides.
//
// Requirements: 1.15, 1.16, 1.17
func (s *LookupService) Lookup(ctx context.Context, barcode, userID string) (LookupResult, error) {
	if barcode == "" {
		return LookupResult{}, fmt.Errorf("barcode cannot be empty")
	}
	if userID == "" {
		return LookupResult{}, fmt.Errorf("userID cannot be empty")
	}

	// Tier 1 & 2: Check user overrides and global DB (handled by the catalog in priority order).
	product, err := s.Catalog.LookupByBarcode(ctx, barcode, userID)
	if err != nil {
		return LookupResult{}, fmt.Errorf("database lookup failed: %w", err)
	}

	// If we found exactly one product in the database, return it.
	if product != nil {
		result := LookupResult{
			Product: product,
			Source:  "global", // Conservative default; the catalog prioritizes overrides internally
		}
		if s.Refresher != nil {
			s.Refresher.ScheduleRefresh(ctx, product.ID)
		}
		return result, nil
	}

	// Tier 3: Fall back to external API (Open Food Facts).
	product, err = s.OpenFoodFacts.LookupBarcode(ctx, barcode)
	if err != nil {
		// If the external API says "not found", treat as legitimate not-found result.
		if errors.Is(err, ErrProductNotFound) {
			return LookupResult{}, nil
		}
		// Other errors (network, timeout, etc.) are treated as not-found.
		// The UI handles this via the flagged entry workflow.
		return LookupResult{}, nil
	}

	// External API returned a product. Persist it so the returned ID references a
	// real products row and a subsequent scan resolves at Tier 2.
	if err := s.persistExternalProduct(ctx, product); err != nil {
		return LookupResult{}, err
	}

	return LookupResult{
		Product: product,
		Source:  "external",
	}, nil
}

// persistExternalProduct idempotently stores an externally-resolved product and
// its barcode mapping. For external results product.ID equals the barcode, so
// the products row and the barcodes mapping share that value. Repeated calls for
// the same barcode create no duplicate rows.
func (s *LookupService) persistExternalProduct(ctx context.Context, product *ProductSummary) error {
	existing, err := s.Catalog.GetProductByID(ctx, product.ID)
	if err != nil {
		return fmt.Errorf("could not check for existing product: %w", err)
	}
	if existing == nil {
		now := s.now()
		err = s.Catalog.CreateProduct(ctx, Product{
			ID:            product.ID,
			Name:          product.Name,
			Category:      product.Category,
			UnitOfMeasure: product.UnitOfMeasure,
			ImageURL:      product.ImageURL,
			Source:        SourceExternal,
			RefreshedAt:   &now,
		})
		if err != nil {
			return fmt.Errorf("could not save product: %w", err)
		}
	}

	// External products are stored as global catalog entries; the barcodes.source
	// CHECK permits only "global" and "user_override".
	if err := s.Catalog.UpsertBarcodeMapping(ctx, product.ID, product.ID, "global", ""); err != nil {
		return fmt.Errorf("could not save barcode mapping: %w", err)
	}
	return nil
}
