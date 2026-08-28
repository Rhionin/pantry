package server

import (
	"context"
	"errors"

	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListItemDeleteHandler handles DELETE /api/shopping-list/items/{id}.
type ShoppingListItemDeleteHandler struct {
	ShoppingList interface {
		RemoveItem(ctx context.Context, id string) error
	}
}

func (h *ShoppingListItemDeleteHandler) Handle(req Request[struct{}, shoppingListItemPathParams]) (struct{}, error) {
	err := h.ShoppingList.RemoveItem(req.Context, req.PathParams.ID)
	if err != nil {
		if errors.Is(err, shopping.ErrItemNotFound) {
			return struct{}{}, NotFound("shopping list item not found")
		}
		return struct{}{}, InternalError(err)
	}
	return struct{}{}, nil
}
