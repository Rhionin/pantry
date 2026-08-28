package server

import (
	"context"
	"errors"

	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListItemUpdateHandler handles PATCH /api/shopping-list/items/{id}.
// The only supported update is marking an item as purchased.
type ShoppingListItemUpdateHandler struct {
	ShoppingList interface {
		MarkPurchased(ctx context.Context, id string) error
		GetItemByID(ctx context.Context, id string) (*shopping.ShoppingListItem, error)
	}
}

type shoppingListItemUpdateRequest struct {
	Purchased bool `json:"purchased"`
}

func (h *ShoppingListItemUpdateHandler) Handle(req Request[shoppingListItemUpdateRequest, shoppingListItemPathParams]) (*ShoppingListEntryResponse, error) {
	id := req.PathParams.ID

	if req.Body.Purchased {
		err := h.ShoppingList.MarkPurchased(req.Context, id)
		if err != nil {
			if errors.Is(err, shopping.ErrItemNotFound) {
				return nil, NotFound("shopping list item not found")
			}
			return nil, InternalError(err)
		}
	}

	item, err := h.ShoppingList.GetItemByID(req.Context, id)
	if err != nil {
		return nil, InternalError(err)
	}
	if item == nil {
		return nil, NotFound("shopping list item not found")
	}

	resp := shoppingListItemToResponse(item)
	return &resp, nil
}
