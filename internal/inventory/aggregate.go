package inventory

import (
	"context"
	"strings"
	"time"
)

// InventoryItem represents an aggregated inventory item with instance counts
// and attention flags derived from expiry status.
type InventoryItem struct {
	Item            Item `json:"item"`
	InstanceCount   int  `json:"instanceCount"`
	NearExpiryCount int  `json:"nearExpiryCount"`
	ExpiredCount    int  `json:"expiredCount"`
	NeedsAttention  bool `json:"needsAttention"`
}

// DefaultWarningDays is the near-expiry window GetInventoryList and
// GetInventoryItem use when no caller-specific value is required.
const DefaultWarningDays = 7

// GetInventoryList returns all items for the given userID with aggregated
// instance information including counts and expiry-based attention flags.
// Items are ordered by product name. If query is non-empty, only items
// whose name or category contains the query string (case-insensitive) are returned.
//
// Validates Requirements 2.2, 2.3, 2.4, 2.10, 2.11
func (r *Pantry) GetInventoryList(ctx context.Context, userID string, now time.Time, warningDays int, query string) ([]InventoryItem, error) {
	// Get all items for the user
	items, err := r.ListItems(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]InventoryItem, 0, len(items))

	for _, item := range items {
		// Apply search filter if query is provided
		if query != "" && !matchesQuery(item, query) {
			continue
		}

		invItem, err := r.aggregateItem(ctx, item, now, warningDays)
		if err != nil {
			return nil, err
		}

		result = append(result, invItem)
	}

	return result, nil
}

// GetInventoryItem returns the aggregated InventoryItem for a single item,
// or nil if no such item exists.
func (r *Pantry) GetInventoryItem(ctx context.Context, itemID string, now time.Time, warningDays int) (*InventoryItem, error) {
	item, err := r.getItemByID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}

	invItem, err := r.aggregateItem(ctx, *item, now, warningDays)
	if err != nil {
		return nil, err
	}
	return &invItem, nil
}

// aggregateItem computes the InstanceCount, NearExpiryCount, ExpiredCount,
// and NeedsAttention fields for a single item, shared by GetInventoryList
// and GetInventoryItem so their behavior can never drift apart.
func (r *Pantry) aggregateItem(ctx context.Context, item Item, now time.Time, warningDays int) (InventoryItem, error) {
	// Get all instances for this item
	instances, err := r.ListItemInstances(ctx, item.ID)
	if err != nil {
		return InventoryItem{}, err
	}

	invItem := InventoryItem{
		Item:          item,
		InstanceCount: len(instances),
	}

	// Compute expiry status for each instance
	for _, instance := range instances {
		status := ComputeExpiryStatus(instance.ExpiresAt, now, warningDays)
		switch status {
		case ExpiryStatusNearExpiry:
			invItem.NearExpiryCount++
		case ExpiryStatusExpired:
			invItem.ExpiredCount++
		}
	}

	// Set needs attention flag if any instance is near expiry or expired
	invItem.NeedsAttention = invItem.NearExpiryCount > 0 || invItem.ExpiredCount > 0

	return invItem, nil
}

// matchesQuery returns true if the item's product name or category contains
// the query string (case-insensitive).
func matchesQuery(item Item, query string) bool {
	lowerQuery := strings.ToLower(query)
	lowerName := strings.ToLower(item.Product.Name)
	lowerCategory := strings.ToLower(item.Product.Category)

	return strings.Contains(lowerName, lowerQuery) || strings.Contains(lowerCategory, lowerQuery)
}
