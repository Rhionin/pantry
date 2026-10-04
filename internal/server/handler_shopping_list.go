package server

import (
	"time"

	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListEntryResponse is the API representation of a shopping list entry.
type ShoppingListEntryResponse struct {
	ID                string             `json:"id"`
	ItemID            string             `json:"itemId"`
	Quantity          int                `json:"quantity"`
	Source            string             `json:"source"` // "auto" or "manual"
	Note              string             `json:"note"`
	PurchasedAt       *string            `json:"purchasedAt"` // ISO 8601 or null
	ReplenishmentMode string             `json:"replenishmentMode,omitempty"`
	Provider          string             `json:"provider,omitempty"`
	ComputedQuantity  int                `json:"computedQuantity,omitempty"`
	Adjustment        *int               `json:"adjustment,omitempty"`
	Basis             *ShoppingListBasis `json:"basis,omitempty"`
}

// ShoppingListBasis is the arithmetic behind one entry's computed quantity.
type ShoppingListBasis struct {
	Mode           string `json:"mode"`
	TargetQuantity *int   `json:"targetQuantity,omitempty"`
	InstanceCount  int    `json:"instanceCount"`
	ConsumedUnits  int    `json:"consumedUnits"`
	Requested      int    `json:"requested"`
}

// shoppingListItemPathParams holds path parameters for item-specific endpoints.
type shoppingListItemPathParams struct {
	ID string `json:"id"`
}

// shoppingListItemToResponse converts a ShoppingListItem to an API response.
func shoppingListItemToResponse(item *shopping.ShoppingListItem) ShoppingListEntryResponse {
	r := ShoppingListEntryResponse{
		ID:       item.ID,
		ItemID:   item.ItemID,
		Quantity: item.Quantity,
		Source:   item.Source,
	}
	if item.PurchasedAt != nil {
		ts := item.PurchasedAt.Format(time.RFC3339)
		r.PurchasedAt = &ts
	}
	return r
}
