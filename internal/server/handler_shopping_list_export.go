package server

import (
	"context"
	"errors"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListExportHandler handles POST /api/shopping-list/export.
// It submits the replenishment list, including store-brand pooling and any
// saved brand preference. useItemIds swaps a line for another brand of the
// same product when the shopper accepts a sale. The swap is this export only.
type ShoppingListExportHandler struct {
	ShoppingList interface {
		ListManualItems(ctx context.Context, userID string) ([]shopping.ShoppingListItem, error)
		ListPreferences(ctx context.Context, userID string) ([]shopping.Preference, error)
		ListDeals(ctx context.Context, userID string) ([]shopping.Deal, error)
	}
	Pantry interface {
		ListItems(ctx context.Context, userID string) ([]inventory.Item, error)
		ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
	}
	Exporter shopping.CartExporter
}

type shoppingListExportRequest struct {
	UseItemIDs map[string]string `json:"useItemIds"`
}

type exportedItemResponse struct {
	ItemID   string `json:"itemId"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type shoppingListExportResponse struct {
	Exported int                    `json:"exported"`
	Items    []exportedItemResponse `json:"items"`
}

func (h *ShoppingListExportHandler) Handle(req Request[shoppingListExportRequest, struct{}]) (*shoppingListExportResponse, error) {
	const userID = "user-1"

	provision, err := loadShoppingProvision(req.Context, userID, h.Pantry, h.ShoppingList)
	if err != nil {
		return nil, InternalError(err)
	}

	exportItems := make([]shopping.ExportItem, len(provision.Merged))
	for i, entry := range provision.Merged {
		itemID, subErr := shopping.SubstituteBrand(entry.ItemID, req.Body.UseItemIDs[entry.ItemID], provision.Needs)
		if subErr != nil {
			if errors.Is(subErr, shopping.ErrDifferentProduct) {
				return nil, &HTTPError{Code: 422, Message: "Choose a brand of the same product"}
			}
			return nil, InternalError(subErr)
		}
		exportItems[i] = shopping.ExportItem{
			ItemID:   itemID,
			Name:     itemName(provision.Needs, itemID),
			Quantity: entry.Quantity,
		}
	}

	if err := h.Exporter.Export(req.Context, exportItems); err != nil {
		var exportErr *shopping.ExportError
		if errors.As(err, &exportErr) {
			return nil, &HTTPError{Code: 500, Message: exportErr.Error()}
		}
		return nil, InternalError(err)
	}

	items := make([]exportedItemResponse, len(exportItems))
	for i, item := range exportItems {
		items[i] = exportedItemResponse{ItemID: item.ItemID, Name: item.Name, Quantity: item.Quantity}
	}
	return &shoppingListExportResponse{Exported: len(exportItems), Items: items}, nil
}
