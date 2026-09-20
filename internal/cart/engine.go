// Package cart provides provider-agnostic provisioning capability types and interfaces.
package cart

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Rhionin/pantry/internal/cart/connection"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/suggestion"
	"github.com/Rhionin/pantry/internal/shopping"
)

// ReplenishmentMode identifies the shopping list calculation mode.
type ReplenishmentMode string

const (
	ReplenishMode ReplenishmentMode = "replenish"
	TargetMode    ReplenishmentMode = "target"
)

// Valid returns true if the mode is a valid value.
func (m ReplenishmentMode) Valid() bool {
	return m == ReplenishMode || m == TargetMode
}

// Engine implements the provisioning operation.
// It coordinates the flow from shopping list → identities → batches → provider → ledger.
type Engine struct {
	mu       sync.Mutex
	inFlight map[ProviderID]struct{} // tracks in-flight provisioning operations per provider

	registry       *Registry
	ledger         *Ledger
	tokenBroker    *connection.TokenBroker
	shoppingList   shopping.Store
	pantry         inventory.Pantry
	consumptionLog suggestion.ConsumptionLog
	catalog        product.Catalog
}

// NewEngine creates a new provisioning engine.
func NewEngine(registry *Registry, ledger *Ledger, tokenBroker *connection.TokenBroker) *Engine {
	return &Engine{
		registry:       registry,
		ledger:         ledger,
		tokenBroker:    tokenBroker,
		inFlight:       make(map[ProviderID]struct{}),
	}
}

// SetShoppingList sets the shopping list store.
func (e *Engine) SetShoppingList(shoppingList shopping.Store) {
	e.shoppingList = shoppingList
}

// SetPantry sets the pantry.
func (e *Engine) SetPantry(pantry inventory.Pantry) {
	e.pantry = pantry
}

// SetConsumptionLog sets the consumption log.
func (e *Engine) SetConsumptionLog(consumptionLog suggestion.ConsumptionLog) {
	e.consumptionLog = consumptionLog
}

// SetCatalog sets the product catalog.
func (e *Engine) SetCatalog(catalog product.Catalog) {
	e.catalog = catalog
}

// Provision performs the complete provisioning operation for one provider.
// It claims an in-flight slot, computes quantities, resolves identities, batches requests,
// submits to the provider, records outcomes, and updates the ledger.
func (e *Engine) Provision(ctx context.Context, userID string, providerID ProviderID) (ProvisionReport, error) {
	e.mu.Lock()
	if _, exists := e.inFlight[providerID]; exists {
		e.mu.Unlock()
		return ProvisionReport{}, fmt.Errorf("provisioning operation already in progress for provider %q", providerID)
	}
	e.inFlight[providerID] = struct{}{}
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.inFlight, providerID)
		e.mu.Unlock()
	}()

	// Get the provider from registry
	provider, ok := e.registry.Get(providerID)
	if !ok {
		return ProvisionReport{}, fmt.Errorf("provider %q not registered", providerID)
	}

	caps := provider.Capabilities()

	// Check connection state
	conn, err := e.tokenBroker.Directory.Read(ctx, string(providerID))
	if err != nil {
		return ProvisionReport{}, fmt.Errorf("failed to read connection: %w", err)
	}

	// For auth=none providers, the connection may not exist - that's OK
	shouldCheckConnection := caps.Auth != AuthNone
	if shouldCheckConnection && (conn == nil || conn.State != connection.StateConnected) {
		return ProvisionReport{}, fmt.Errorf("provider %q is not connected", providerID)
	}

	// Get computed entries from shopping list (with ledger-net quantities)
	entries, err := e.getComputedEntries(ctx, providerID, userID)
	if err != nil {
		return ProvisionReport{}, fmt.Errorf("failed to compute entries: %w", err)
	}

	// Resolve identities for each entry
	resolvedItems, unresolved, err := e.resolveIdentities(ctx, provider, caps, providerID, entries)
	if err != nil {
		return ProvisionReport{}, fmt.Errorf("failed to resolve identities: %w", err)
	}

	// Combine resolved and unresolved into outcomes for entries with quantity >= 1
	outcomes := make([]EntryOutcome, 0, len(resolvedItems)+len(unresolved))

	// Add unresolved items as failed with reason no_product_identity
	for _, item := range unresolved {
		outcomes = append(outcomes, EntryOutcome{
			EntryID:  item.EntryID,
			ItemID:   item.ItemID,
			Name:     item.Name,
			Quantity: item.Quantity,
			Outcome:  OutcomeFailed,
			Reason:   ReasonNoProductIdentity,
		})
	}

	// If no resolved items, return early with just unresolved items
	if len(resolvedItems) == 0 {
		return ProvisionReport{
			Provider:  providerID,
			Confirmed: 0,
			Entries:   outcomes,
		}, nil
	}

	// Batch resolved items into ProvisionRequests
	batches := e.batchRequests(providerID, resolvedItems, caps)

	// Get credential based on auth type
	var cred Credential
	if caps.Auth == AuthNone {
		cred = NoCredential{}
	} else if caps.Auth == AuthOAuth2 {
		// For OAuth2, use the token broker to get an access token
		token, err := e.tokenBroker.AccessToken(ctx, string(providerID), func(refreshToken string) (string, string, time.Duration, error) {
			// This callback is called when refresh is needed
			// For now, we return error since we don't have the refresh function
			return "", "", 0, fmt.Errorf("token refresh not implemented")
		})
		if err != nil {
			// For indeterminate outcomes, record failed/unknown and return
			for _, item := range resolvedItems {
				outcomes = append(outcomes, EntryOutcome{
					EntryID:  item.EntryID,
					ItemID:   item.ItemID,
					Name:     item.Name,
					Quantity: item.Quantity,
					Outcome:  OutcomeUnknown,
					Reason:   ReasonRequestIncomplete,
				})
			}
			return ProvisionReport{
				Provider:  providerID,
				Confirmed: 0,
				Entries:   outcomes,
			}, nil
		}
		cred = NewBearerCredential(token)
	}

	// Submit each batch to provider
	var confirmedCount int
	for _, req := range batches {
		var result ProvisionResult
		if caps.Delivery == DeliveryServerPush {
			// Get the ServerPush interface
			if serverPush, ok := provider.(ServerPush); ok {
				result, err = serverPush.Add(ctx, cred, req)
			} else {
				err = fmt.Errorf("provider does not support server_push delivery")
			}
		} else {
			err = fmt.Errorf("client_handoff delivery not implemented")
		}

		if err != nil || result.Disposition == DispositionIndeterminate {
			// Indeterminate or error: record all accounted entries as unknown
			for _, line := range req.Lines {
				for _, item := range line.Accounts {
					outcomes = append(outcomes, EntryOutcome{
						EntryID:  item.EntryID,
						ItemID:   item.ItemID,
						Name:     item.Name,
						Quantity: item.Quantity,
						Outcome:  OutcomeUnknown,
						Reason:   ReasonRequestIncomplete,
					})
				}
			}
			continue
		}

		if result.Disposition == DispositionRejected {
			// Rejected: record all accounted entries as failed
			for _, line := range req.Lines {
				for _, item := range line.Accounts {
					outcomes = append(outcomes, EntryOutcome{
						EntryID:  item.EntryID,
						ItemID:   item.ItemID,
						Name:     item.Name,
						Quantity: item.Quantity,
						Outcome:  OutcomeFailed,
						Reason:   ReasonProviderRejected,
					})
				}
			}
			continue
		}

		// Accepted: record outcomes and update ledger
		var acceptedCount int
		switch caps.Confirmation {
		case ConfirmPerLine:
			// per_line: check each line's identity in PerLine map
			for _, line := range req.Lines {
				confirmed := result.PerLine[line.Identity]
				for _, item := range line.Accounts {
					outcomes = append(outcomes, EntryOutcome{
						EntryID:  item.EntryID,
						ItemID:   item.ItemID,
						Name:     item.Name,
						Quantity: item.Quantity,
						Outcome:  OutcomeConfirmed,
						Reason:   "", // not used for confirmed
					})
					if confirmed {
						acceptedCount++
					}
				}
			}
		case ConfirmPerRequest:
			// per_request: all accounted entries are confirmed
			for _, line := range req.Lines {
				for _, item := range line.Accounts {
					outcomes = append(outcomes, EntryOutcome{
						EntryID:  item.EntryID,
						ItemID:   item.ItemID,
						Name:     item.Name,
						Quantity: item.Quantity,
						Outcome:  OutcomeConfirmed,
						Reason:   "", // not used for confirmed
					})
					acceptedCount += len(line.Accounts)
				}
			}
		case ConfirmNone:
			// none: all accounted entries are unknown
			for _, line := range req.Lines {
				for _, item := range line.Accounts {
					outcomes = append(outcomes, EntryOutcome{
						EntryID:  item.EntryID,
						ItemID:   item.ItemID,
						Name:     item.Name,
						Quantity: item.Quantity,
						Outcome:  OutcomeUnknown,
						Reason:   ReasonNoPerItemResult,
					})
				}
			}
		}

		// Update ledger for accepted items
		if acceptedCount > 0 {
			if err := e.updateLedgerAndAdjustments(ctx, providerID, req, result); err != nil {
				// If ledger update fails, we have a partial failure
				return ProvisionReport{
					Provider:  providerID,
					Confirmed: confirmedCount,
					Entries:   outcomes,
				}, fmt.Errorf("failed to update ledger: %w", err)
			}
			confirmedCount += acceptedCount
		}
	}

	return ProvisionReport{
		Provider:  providerID,
		Confirmed: confirmedCount,
		Entries:   outcomes,
	}, nil
}

// getComputedEntries gets shopping list entries with ledger-net quantities.
func (e *Engine) getComputedEntries(ctx context.Context, providerID ProviderID, userID string) ([]ResolvedItem, error) {
	// Get shopping list manual items
	manualItems, err := e.shoppingList.ListManualItems(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list manual items: %w", err)
	}

	// Get ledger entries for this provider
	ledger, err := e.ledger.ListForProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list ledger: %w", err)
	}

	// Get pantry instances
	pantryInstances, err := e.pantry.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list pantry: %w", err)
	}

	// Build map of instances per item
	instancesByItem := make(map[string]int)
	for _, inst := range pantryInstances {
		instancesByItem[inst.ItemID]++
	}

	// Get consumption log - all items
	consumed, err := e.consumptionLog.ListConsumedAtByItems(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumption: %w", err)
	}

	// Get all item IDs from manual items and consumption
	allItemIDs := make(map[string]bool)
	for _, item := range manualItems {
		allItemIDs[item.ItemID] = true
	}
	for itemID := range consumed {
		allItemIDs[itemID] = true
	}

	// Get modes and targets from items
	itemModes := make(map[string]ReplenishmentMode)
	itemTargets := make(map[string]*int)
	itemNames := make(map[string]string)
	for itemID := range allItemIDs {
		mode, err := e.pantry.GetReplenishmentMode(ctx, itemID)
		if err != nil {
			return nil, fmt.Errorf("failed to get mode for %s: %w", itemID, err)
		}
		// Convert inventory.ReplenishmentMode to cart.ReplenishmentMode
		itemModes[itemID] = ReplenishmentMode(mode)

		target, err := e.pantry.GetTargetQuantity(ctx, itemID)
		if err != nil {
			return nil, fmt.Errorf("failed to get target for %s: %w", itemID, err)
		}
		itemTargets[itemID] = target

		// Get product name
		product, err := e.catalog.GetProductByID(ctx, itemID)
		if err != nil {
			return nil, fmt.Errorf("failed to get product %s: %w", itemID, err)
		}
		if product != nil {
			itemNames[itemID] = product.Name
		} else {
			itemNames[itemID] = "Unknown"
		}
	}

	// Compute entries using ComputeEntries from shopping package
	var result []ResolvedItem
	// Use shopping package's ComputeEntries function
	// First, prepare the inputs in the format shopping expects
	derivedEntries := make([]shopping.DerivedEntry, 0)
	manualEntries := make([]shopping.ManualEntry, 0, len(manualItems))

	// Build derived entries from all items
	for itemID := range allItemIDs {
		target, err := e.pantry.GetTargetQuantity(ctx, itemID)
		if err != nil {
			return nil, fmt.Errorf("failed to get target for %s: %w", itemID, err)
		}

		currentQty := instancesByItem[itemID]
		requestedQty := ledger[itemID].Requested

		// Use shopping package's ComputeQuantity
		qty := shopping.ComputeQuantity(
			len(consumed[itemID]),
			requestedQty,
			target,
			currentQty,
			shopping.ReplenishmentMode(itemModes[itemID]),
		)

		if qty > 0 {
			derivedEntries = append(derivedEntries, shopping.DerivedEntry{
				ItemID:   itemID,
				Quantity: qty,
				Source:   "auto",
			})
		}
	}

	// Manual entries - use entry IDs from manualItems
	for _, item := range manualItems {
		if item.PurchasedAt != nil {
			continue
		}

		// Get the computed quantity
		qty := shopping.ComputeQuantity(
			len(consumed[item.ItemID]),
			ledger[item.ItemID].Requested,
			itemTargets[item.ItemID],
			instancesByItem[item.ItemID],
			shopping.ReplenishmentMode(itemModes[item.ItemID]),
		)

		if qty > 0 {
			manualEntries = append(manualEntries, shopping.ManualEntry{
				ItemID:   item.ItemID,
				Quantity: qty,
			})
		}
	}

	// Merge entries using shopping package function
	merged := shopping.MergeEntries(derivedEntries, manualEntries)

	// Build result - need to map merged items back to entry IDs
	entryIDByItemID := make(map[string]string)
	for _, item := range manualItems {
		entryIDByItemID[item.ItemID] = item.ID
	}

	for _, entry := range merged {
		entryID, hasEntry := entryIDByItemID[entry.ItemID]
		if !hasEntry {
			continue // derived entry, no entry ID
		}

		result = append(result, ResolvedItem{
			EntryID:  entryID,
			ItemID:   entry.ItemID,
			Name:     itemNames[entry.ItemID],
			Identity: "",
			Quantity: entry.Quantity,
		})
	}

	return result, nil
}

// resolveIdentities resolves identities for each entry.
func (e *Engine) resolveIdentities(ctx context.Context, provider Provider, caps Capabilities, providerID ProviderID, entries []ResolvedItem) ([]ResolvedItem, []ResolvedItem, error) {
	var resolved, unresolved []ResolvedItem

	// Check if provider implements DerivedIdentity or LookedUpIdentity
	var derivedIdentity DerivedIdentity
	var lookedUpIdentity LookedUpIdentity
	var hasDerived bool
	var hasLookedUp bool

	if caps := provider.Capabilities(); caps.Identity == IdentityDerived {
		if di, ok := provider.(DerivedIdentity); ok {
			derivedIdentity = di
			hasDerived = true
		}
	} else if caps.Identity == IdentityLookedUp {
		if lu, ok := provider.(LookedUpIdentity); ok {
			lookedUpIdentity = lu
			hasLookedUp = true
		}
	}

	for _, entry := range entries {
		// Get product barcodes
		barcodes, err := e.catalog.ListBarcodesForProduct(ctx, entry.ItemID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list barcodes for %s: %w", entry.ItemID, err)
		}

		// Try each barcode until we find a valid identity
		var identity ProductIdentity
		found := false

		if hasDerived {
			for _, barcode := range barcodes {
				id, ok := derivedIdentity.DeriveIdentity(barcode)
				if ok {
					identity = id
					found = true
					break
				}
			}
		} else if hasLookedUp {
			// For looked_up, we need to call the provider's LookUpIdentity
			// Get credential based on auth type
			var cred Credential
			if caps.Auth == AuthNone {
				cred = NoCredential{}
			} else if caps.Auth == AuthOAuth2 {
				token, err := e.tokenBroker.AccessToken(ctx, string(providerID), func(refreshToken string) (string, string, time.Duration, error) {
					return "", "", 0, fmt.Errorf("token refresh not implemented")
				})
				if err != nil {
					unresolved = append(unresolved, entry)
					continue
				}
				cred = NewBearerCredential(token)
			}

			for _, barcode := range barcodes {
				id, ok, err := lookedUpIdentity.LookUpIdentity(ctx, cred, barcode)
				if err != nil {
					continue
				}
				if ok {
					identity = id
					found = true
					break
				}
			}
		}

		if found {
			entry.Identity = identity
			resolved = append(resolved, entry)
		} else {
			unresolved = append(unresolved, entry)
		}
	}

	return resolved, unresolved, nil
}

// batchRequests groups resolved items into batches based on identity.
func (e *Engine) batchRequests(providerID ProviderID, items []ResolvedItem, caps Capabilities) []ProvisionRequest {
	// Group by identity
	groups := make(map[ProductIdentity][]ResolvedItem)
	for _, item := range items {
		groups[item.Identity] = append(groups[item.Identity], item)
	}

	// Build lines
	var lines []ProvisionLine
	for identity, group := range groups {
		var totalQty int
		for _, item := range group {
			totalQty += item.Quantity
			if totalQty > 999 {
				totalQty = 999
			}
		}

		lines = append(lines, ProvisionLine{
			Identity: identity,
			Quantity: totalQty,
			Accounts: group,
		})
	}

	// Slice into batches
	var batches []ProvisionRequest
	const BatchSize = 50 // Default batch size

	for len(lines) > 0 {
		batchSize := BatchSize
		if len(lines) < batchSize {
			batchSize = len(lines)
		}

		batchLines := make([]ProvisionLine, batchSize)
		copy(batchLines, lines[:batchSize])
		lines = lines[batchSize:]

		batches = append(batches, ProvisionRequest{
			Provider: providerID,
			Lines:    batchLines,
		})
	}

	return batches
}

// updateLedgerAndAdjustments updates the ledger and clears adjustments for accepted items.
func (e *Engine) updateLedgerAndAdjustments(ctx context.Context, providerID ProviderID, req ProvisionRequest, result ProvisionResult) error {
	// Create a transaction
	tx, err := e.ledger.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	currentTime := time.Now().UTC()

	// For per_request confirmation, all lines are confirmed
	// For per_line, check PerLine map
	confirmedLines := make(map[ProductIdentity]bool)
	if result.Disposition == DispositionAccepted {
		if result.PerLine != nil {
			confirmedLines = result.PerLine
		} else {
			// per_request or none - all lines confirmed
			for _, line := range req.Lines {
				confirmedLines[line.Identity] = true
			}
		}
	}

	for _, line := range req.Lines {
		// Update ledger
		_, err := e.ledger.Advance(ctx, providerID, line.Accounts[0].ItemID, line.Quantity, currentTime)
		if err != nil {
			return fmt.Errorf("failed to advance ledger: %w", err)
		}

		// Clear adjustments for all accounts of this line
		for _, item := range line.Accounts {
			err := e.shoppingList.ClearAdjustmentTx(ctx, tx, item.EntryID, string(providerID))
			if err != nil {
				return fmt.Errorf("failed to clear adjustment for %s: %w", item.EntryID, err)
			}
		}
	}

	return tx.Commit()
}
