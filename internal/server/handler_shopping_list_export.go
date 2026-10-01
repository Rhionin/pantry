package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/cart/connection"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
)

// ShoppingListExportHandler handles POST /api/shopping-list/export.
// It submits the replenishment list, including store-brand pooling and any
// saved brand preference. useItemIds swaps a line for another brand of the
// same product when the shopper accepts a sale. The swap is this export only.
// With no configured provider the call still returns the planned lines and
// confirms nothing.
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
	Provisioner cart.Provisioner
	Ledger      *cart.Ledger
	Registry    *cart.Registry
	Connections *connection.Directory
}

type shoppingListExportRequest struct {
	Provider   string            `json:"provider"`
	UseItemIDs map[string]string `json:"useItemIds"`
}

type exportedItemResponse struct {
	ItemID   string `json:"itemId"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type ProvisionEntry struct {
	EntryID       string `json:"entryId"`
	ItemID        string `json:"itemId"`
	Name          string `json:"name"`
	Quantity      int    `json:"quantity"`
	Outcome       string `json:"outcome"`
	OutcomeReason string `json:"outcomeReason,omitempty"`
}

type shoppingListExportResponse struct {
	Provider     string                 `json:"provider,omitempty"`
	Exported     int                    `json:"exported"`
	FailedItems  []string               `json:"failedItems,omitempty"`
	UnknownItems []string               `json:"unknownItems,omitempty"`
	Entries      []ProvisionEntry       `json:"entries,omitempty"`
	Handoff      *HandoffResponse       `json:"handoff,omitempty"`
	Items        []exportedItemResponse `json:"items,omitempty"`
}

type HandoffResponse struct {
	URL     string `json:"url,omitempty"`
	Expires string `json:"expires,omitempty"`
}

func (h *ShoppingListExportHandler) Handle(req Request[shoppingListExportRequest, struct{}]) (*shoppingListExportResponse, error) {
	const userID = "user-1"

	provision, err := loadShoppingProvision(req.Context, userID, h.Pantry, h.ShoppingList)
	if err != nil {
		return nil, InternalError(err)
	}
	planned, swaps, err := planExportLines(provision, req.Body.UseItemIDs)
	if err != nil {
		return nil, err
	}

	providerID := cart.ProviderID(req.Body.Provider)
	if providerID == "" && h.Registry != nil {
		if id, ok := h.Registry.SoleConfigured(); ok {
			providerID = id
		}
	}

	// No configured provider: the historical no-op report. Naming an unknown
	// or unconfigured provider is still an error, so a client can tell those
	// apart from "nothing was sent". The planned lines stay on the response
	// so a sale swap can be checked before a retailer is connected.
	if h.Registry == nil || !h.Registry.AnyCredentialsConfigured() {
		if req.Body.Provider != "" {
			if h.Registry == nil {
				return nil, BadRequest(fmt.Sprintf("provider %q is not registered", req.Body.Provider))
			}
			provider, ok := h.Registry.Get(providerID)
			if !ok {
				return nil, BadRequest(fmt.Sprintf("provider %q is not registered", providerID))
			}
			if !h.Registry.CredentialsConfigured(providerID) {
				return nil, Conflict(fmt.Sprintf("%s is not configured", provider.DisplayName()))
			}
		}
		return &shoppingListExportResponse{Provider: string(providerID), Exported: 0, Items: planned}, nil
	}

	provider, ok := h.Registry.Get(providerID)
	if !ok || providerID == "" {
		named := string(providerID)
		if named == "" {
			named = req.Body.Provider
		}
		return nil, BadRequest(fmt.Sprintf("provider %q is not registered", named))
	}
	if !h.Registry.CredentialsConfigured(providerID) {
		return nil, Conflict(fmt.Sprintf("%s is not configured", provider.DisplayName()))
	}
	caps := provider.Capabilities()
	if caps.Auth != cart.AuthNone {
		if h.Connections == nil {
			return nil, Conflict(fmt.Sprintf("%s is not connected", provider.DisplayName()))
		}
		conn, err := h.Connections.Read(req.Context, string(providerID))
		if err != nil {
			return nil, InternalError(err)
		}
		if conn == nil || conn.State != connection.StateConnected {
			return nil, Conflict(fmt.Sprintf("%s is not connected", provider.DisplayName()))
		}
	}

	report, err := h.Provisioner.Provision(cart.WithExportSubstitutions(req.Context, swaps), userID, providerID)
	if err != nil {
		var conflict *cart.ProvisionConflict
		if errors.As(err, &conflict) {
			return nil, Conflict(conflict.Reason)
		}
		if errors.Is(err, shopping.ErrDifferentProduct) {
			return nil, &HTTPError{Code: 422, Message: "Choose a brand of the same product"}
		}
		return nil, InternalError(err)
	}

	entries := make([]ProvisionEntry, 0, len(report.Entries))
	failedItems := make([]string, 0)
	unknownItems := make([]string, 0)
	for _, entry := range report.Entries {
		entries = append(entries, ProvisionEntry{
			EntryID:       entry.EntryID,
			ItemID:        entry.ItemID,
			Name:          entry.Name,
			Quantity:      entry.Quantity,
			Outcome:       string(entry.Outcome),
			OutcomeReason: string(entry.Reason),
		})
		switch entry.Outcome {
		case cart.OutcomeFailed:
			failedItems = append(failedItems, entry.Name)
		case cart.OutcomeUnknown:
			unknownItems = append(unknownItems, entry.Name)
		}
	}

	var handoff *HandoffResponse
	if report.Handoff != nil && report.Handoff.URL != "" {
		handoff = &HandoffResponse{
			URL:     report.Handoff.URL,
			Expires: "30 minutes from now",
		}
	}

	return &shoppingListExportResponse{
		Provider:     string(providerID),
		Exported:     report.Confirmed,
		FailedItems:  failedItems,
		UnknownItems: unknownItems,
		Entries:      entries,
		Handoff:      handoff,
		Items:        planned,
	}, nil
}

// planExportLines applies one-export brand swaps to the merged list.
// swaps maps the line's item id to the brand that should be bought.
func planExportLines(provision shoppingProvision, useItemIDs map[string]string) ([]exportedItemResponse, map[string]string, error) {
	planned := make([]exportedItemResponse, 0, len(provision.Merged))
	swaps := map[string]string{}
	for _, entry := range provision.Merged {
		itemID, err := shopping.SubstituteBrand(entry.ItemID, useItemIDs[entry.ItemID], provision.Needs)
		if err != nil {
			if errors.Is(err, shopping.ErrDifferentProduct) {
				return nil, nil, &HTTPError{Code: 422, Message: "Choose a brand of the same product"}
			}
			return nil, nil, InternalError(err)
		}
		if itemID != entry.ItemID {
			swaps[entry.ItemID] = itemID
		}
		planned = append(planned, exportedItemResponse{
			ItemID:   itemID,
			Name:     itemName(provision.Needs, itemID),
			Quantity: entry.Quantity,
		})
	}
	if len(planned) == 0 {
		return nil, swaps, nil
	}
	return planned, swaps, nil
}
