package server

import (
	"context"
	"errors"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListExportHandler handles POST /api/shopping-list/export.
// It submits the replenishment list, including store-brand pooling, to the cart exporter.
type ShoppingListExportHandler struct {
	ShoppingList interface {
		ListManualItems(ctx context.Context, userID string) ([]shopping.ShoppingListItem, error)
	}
	Pantry interface {
		ListItems(ctx context.Context, userID string) ([]inventory.Item, error)
		ListItemInstances(ctx context.Context, itemID string) ([]inventory.ItemInstance, error)
	}
	Exporter shopping.CartExporter
}

type shoppingListExportResponse struct {
	Exported int `json:"exported"`
}

func (h *ShoppingListExportHandler) Handle(req Request[struct{}, struct{}]) (*shoppingListExportResponse, error) {
	const userID = "user-1"

	items, err := h.Pantry.ListItems(req.Context, userID)
	if err != nil {
		return nil, InternalError(err)
	}

	manualItems, err := h.ShoppingList.ListManualItems(req.Context, userID)
	if err != nil {
		return nil, InternalError(err)
	}

	counts, err := loadInstanceCounts(req.Context, h.Pantry.ListItemInstances, items)
	if err != nil {
		return nil, InternalError(err)
	}

	derived := replenishmentEntries(items, counts, manualItems)

	manualEntries := make([]shopping.ManualEntry, len(manualItems))
	for i, m := range manualItems {
		manualEntries[i] = shopping.ManualEntry{ItemID: m.ItemID, Quantity: m.Quantity}
	}
	merged := shopping.MergeEntries(derived, manualEntries)

	exportItems := make([]shopping.ExportItem, len(merged))
	for i, e := range merged {
		exportItems[i] = shopping.ExportItem{
			ItemID:   e.ItemID,
			Quantity: e.Quantity,
		}
	}

	if err := h.Exporter.Export(req.Context, exportItems); err != nil {
		var exportErr *shopping.ExportError
		if errors.As(err, &exportErr) {
			return nil, &HTTPError{Code: 500, Message: exportErr.Error()}
		}
		return nil, InternalError(err)
	}

	return &shoppingListExportResponse{Exported: len(exportItems)}, nil
}
