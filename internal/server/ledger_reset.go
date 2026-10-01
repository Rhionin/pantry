package server

import (
	"context"
	"database/sql"
	"time"

	"github.com/Rhionin/pantry/internal/cart"
)

// providerLedgerReset clears every registered provider's ledger row for one
// pantry item. Stock-in is evidence about the item, not about one provider.
type providerLedgerReset struct {
	ledger   *cart.Ledger
	registry *cart.Registry
}

func (p providerLedgerReset) ResetForItemTx(ctx context.Context, tx *sql.Tx, itemID string, at time.Time) error {
	if p.ledger == nil || p.registry == nil {
		return nil
	}
	for _, id := range p.registry.List() {
		if err := p.ledger.ResetForItemTx(ctx, tx, id, itemID, at); err != nil {
			return err
		}
	}
	return nil
}
