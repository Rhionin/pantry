package server

import (
	"context"
	"errors"
	"time"

	"github.com/Rhionin/pantry/internal/events"
	"github.com/Rhionin/pantry/internal/history"
	"github.com/Rhionin/pantry/internal/inventory"
)

// HistoryHandler serves pace and the recent trail, and the corrections on a move.
type HistoryHandler struct {
	Journal *history.Journal
	Pantry  *inventory.Pantry
	Events  *events.Broadcaster
}

type historyItemParams struct {
	ItemID string `json:"itemId"`
}

type historyGroupParams struct {
	GroupID string `json:"groupId"`
}

type historyMoveParams struct {
	ID string `json:"id"`
}

type historyQuantityBody struct {
	Quantity int `json:"quantity"`
}

func (h *HistoryHandler) Product(req Request[struct{}, historyItemParams]) (history.View, error) {
	view, err := h.Journal.Product(req.Context, req.PathParams.ItemID, time.Now())
	if err != nil {
		return history.View{}, historyHTTP(err)
	}
	return view, nil
}

func (h *HistoryHandler) Group(req Request[struct{}, historyGroupParams]) (history.View, error) {
	view, err := h.Journal.Group(req.Context, req.PathParams.GroupID, time.Now())
	if err != nil {
		return history.View{}, historyHTTP(err)
	}
	return view, nil
}

func (h *HistoryHandler) SetQuantity(req Request[historyQuantityBody, historyMoveParams]) (history.View, error) {
	view, err := h.Journal.SetQuantity(req.Context, req.PathParams.ID, req.Body.Quantity, time.Now())
	if err != nil {
		return history.View{}, historyHTTP(err)
	}
	h.publish(req.Context, view.ID)
	return view, nil
}

func (h *HistoryHandler) Undo(req Request[struct{}, historyMoveParams]) (history.View, error) {
	view, err := h.Journal.Undo(req.Context, req.PathParams.ID, time.Now())
	if err != nil {
		return history.View{}, historyHTTP(err)
	}
	h.publish(req.Context, view.ID)
	return view, nil
}

func (h *HistoryHandler) publish(ctx context.Context, itemID string) {
	if h.Events == nil || h.Pantry == nil || itemID == "" {
		return
	}
	item, err := h.Pantry.GetInventoryItem(ctx, itemID, time.Now(), inventory.DefaultWarningDays)
	if err != nil || item == nil {
		return
	}
	h.Events.PublishInventoryEvent(*item)
}

func historyHTTP(err error) error {
	var missing *history.NotFoundError
	if errors.As(err, &missing) {
		return NotFound(missing.Error())
	}
	var input *history.InputError
	if errors.As(err, &input) {
		return BadRequest(input.Error())
	}
	var conflict *history.ConflictError
	if errors.As(err, &conflict) {
		return Conflict(conflict.Error())
	}
	return InternalError(err)
}
