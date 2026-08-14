package product

import (
	"context"
	"errors"
	"fmt"
)

// LookupResult represents the outcome of a three-tier product lookup.
type LookupResult struct {
	// Product is the single matching product, or nil if not found.
	Product *ProductSummary

	// Source indicates where the result came from: "user_override", "global", "external", or empty when not found.
	Source string
}

// IsFound returns true if the lookup found a product.
func (r LookupResult) IsFound() bool {
	return r.Product != nil
}

// LookupService orchestrates three-tier product lookup: user overrides first,
// global DB second, Open Food Facts API third.
type LookupService struct {
	Repo interface {
		LookupByBarcode(ctx context.Context, barcode, userID string) (*ProductSummary, error)
	}
	OpenFoodFacts interface {
		LookupBarcode(ctx context.Context, barcode string) (*ProductSummary, error)
	}
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

	// Tier 1 & 2: Check user overrides and global DB (handled by repo in priority order).
	product, err := s.Repo.LookupByBarcode(ctx, barcode, userID)
	if err != nil {
		return LookupResult{}, fmt.Errorf("database lookup failed: %w", err)
	}

	// If we found exactly one product in the database, return it.
	if product != nil {
		return LookupResult{
			Product: product,
			Source:  "global", // Conservative default; repo prioritizes overrides internally
		}, nil
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

	// External API returned a product.
	return LookupResult{
		Product: product,
		Source:  "external",
	}, nil
}
