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
// It fetches the current shopping list and submits it to the cart exporter.
type ShoppingListExportHandler struct {
	ShoppingList interface {
		ListManualItems(ctx context.Context, userID string) ([]shopping.ShoppingListItem, error)
		SyncDerivedItems(ctx context.Context, userID string, derived []shopping.DerivedEntry) ([]shopping.ShoppingListItem, error)
		GetItemByID(ctx context.Context, entryID string) (*shopping.ShoppingListItem, error)
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
	Provider string `json:"provider"`
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
	Provider     string           `json:"provider"`
	Exported     int              `json:"exported"`
	FailedItems  []string         `json:"failedItems,omitempty"`
	UnknownItems []string         `json:"unknownItems,omitempty"`
	Entries      []ProvisionEntry `json:"entries,omitempty"`
	Handoff      *HandoffResponse `json:"handoff,omitempty"`
}

type HandoffResponse struct {
	URL     string `json:"url,omitempty"`
	Expires string `json:"expires,omitempty"`
}

func (h *ShoppingListExportHandler) Handle(req Request[shoppingListExportRequest, struct{}]) (*shoppingListExportResponse, error) {
	userID := "user-1"

	providerID := cart.ProviderID(req.Body.Provider)
	if providerID == "" && h.Registry != nil {
		if id, ok := h.Registry.SoleConfigured(); ok {
			providerID = id
		}
	}

	// No configured provider: the historical no-op report. Naming an unknown
	// or unconfigured provider is still an error, so a client can tell those
	// apart from "nothing was sent".
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
		return &shoppingListExportResponse{Provider: string(providerID), Exported: 0}, nil
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

	report, err := h.Provisioner.Provision(req.Context, userID, providerID)
	if err != nil {
		var conflict *cart.ProvisionConflict
		if errors.As(err, &conflict) {
			return nil, Conflict(conflict.Reason)
		}
		return nil, InternalError(err)
	}

	// Build response entries from the report
	entries := make([]ProvisionEntry, 0)
	failedItems := make([]string, 0)
	unknownItems := make([]string, 0)

	for _, entry := range report.Entries {
		provisionEntry := ProvisionEntry{
			EntryID:       entry.EntryID,
			ItemID:        entry.ItemID,
			Name:          entry.Name,
			Quantity:      entry.Quantity,
			Outcome:       string(entry.Outcome),
			OutcomeReason: string(entry.Reason),
		}

		entries = append(entries, provisionEntry)

		// Categorize by outcome
		switch entry.Outcome {
		case cart.OutcomeFailed:
			failedItems = append(failedItems, entry.Name)
		case cart.OutcomeUnknown:
			unknownItems = append(unknownItems, entry.Name)
		}
	}

	// Handle handoff if present
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
	}, nil
}
