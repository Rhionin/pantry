package grocysync

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Rhionin/pantry/internal/syncsnap"
)

func (r *Reconciler) syncStock(ctx context.Context, snap syncsnap.Snapshot, products map[string]int) error {
	moves := map[string]bool{}
	for _, m := range snap.Moves {
		moves[m.ID] = true
	}
	removals := map[string]syncsnap.Removal{}
	for _, rm := range snap.Removals {
		removals[rm.InstanceID] = rm
	}
	open := map[string]bool{}
	for _, inst := range snap.Instances {
		open[inst.ID] = true
		if err := r.ensureInstance(ctx, inst, products); err != nil {
			return err
		}
	}
	existing, err := r.Map.ByType(linkInstance)
	if err != nil {
		return err
	}
	for _, link := range existing {
		if open[link.PantryID] {
			continue
		}
		if err := r.dropInstance(ctx, link, moves, removals, products); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) ensureInstance(ctx context.Context, inst syncsnap.Instance, products map[string]int) error {
	if err := r.undoConsumeLink(ctx, inst.ID); err != nil {
		return err
	}
	hash := instanceHash(inst)
	link, ok, err := r.Map.Get(linkInstance, inst.ID)
	if err != nil {
		return err
	}
	grocyID, err := r.grocyProduct(inst.ProductID, products)
	if err != nil {
		return err
	}
	note := instancePrefix + inst.ID
	wantBest := bestBefore(inst.ExpiresAt, r.Loc)
	wantPurchased := day(inst.StockInAt, r.Loc)
	kept, err := r.collapsePurchases(ctx, grocyID, note)
	if err != nil {
		return err
	}
	found, err := r.findEntry(ctx, grocyID, note)
	if err != nil {
		return err
	}
	if found.StockID != "" && sameDay(found.BestBefore, wantBest) && sameDay(found.Purchased, wantPurchased) {
		ref := kept.TransactionID.String()
		if ref == "" && ok {
			ref = link.GrocyRef
		}
		detail := packDetail(inst.StockMoveID, found.StockID)
		if ok && link.Hash == hash && link.GrocyRef == ref && link.Detail == detail {
			return nil
		}
		return r.Map.Put(Link{
			PantryType: linkInstance,
			PantryID:   inst.ID,
			GrocyType:  "stock",
			GrocyRef:   ref,
			Hash:       hash,
			Detail:     detail,
		})
	}
	if kept.TransactionID.String() != "" {
		if err := r.Grocy.Undo(ctx, kept.TransactionID.String()); err != nil {
			return fmt.Errorf("could not undo a Grocy booking: %w", err)
		}
	} else if ok && link.GrocyRef != "" && found.StockID != "" {
		if err := r.Grocy.Undo(ctx, link.GrocyRef); err != nil {
			return fmt.Errorf("could not undo a Grocy booking: %w", err)
		}
	} else if found.StockID != "" {
		if _, err := r.Grocy.Consume(ctx, grocyID, consumeBody{
			Amount:          1,
			TransactionType: "consume",
			LocationID:      r.locationID,
			StockEntryID:    found.StockID,
		}); err != nil {
			return fmt.Errorf("could not undo a Grocy booking: %w", err)
		}
	}
	booking, err := r.Grocy.AddStock(ctx, grocyID, addStockBody{
		Amount:          1,
		BestBefore:      wantBest,
		Purchased:       wantPurchased,
		TransactionType: "purchase",
		LocationID:      r.locationID,
		Note:            note,
	})
	if err != nil {
		return fmt.Errorf("could not add stock in Grocy: %w", err)
	}
	stockID := booking.StockID
	if stockID == "" {
		stockID, err = r.stockID(ctx, grocyID, note)
		if err != nil {
			return err
		}
	}
	return r.Map.Put(Link{
		PantryType: linkInstance,
		PantryID:   inst.ID,
		GrocyType:  "stock",
		GrocyRef:   booking.TransactionID,
		Hash:       hash,
		Detail:     packDetail(inst.StockMoveID, stockID),
	})
}

func (r *Reconciler) dropInstance(ctx context.Context, link Link, moves map[string]bool, removals map[string]syncsnap.Removal, products map[string]int) error {
	moveID, stockID := unpackDetail(link.Detail)
	inMoveAlive := moveID != "" && moves[moveID]
	grocyID, err := r.grocyProduct(productIDFromHash(link.Hash), products)
	if err != nil {
		return err
	}
	note := instancePrefix + link.PantryID
	purchases, err := r.openPurchases(ctx, grocyID, note)
	if err != nil {
		return err
	}
	if inMoveAlive {
		if consume, ok, err := r.Map.Get(linkConsume, link.PantryID); err != nil {
			return err
		} else if ok && consume.GrocyRef != "" {
			return r.clearLeftover(ctx, grocyID, note, purchases)
		}
		if len(purchases) > 1 {
			// Undoing one purchase deletes the shared stock row, so put a single unit back and then record the use.
			sample := purchases[0]
			if err := r.undoPurchases(ctx, purchases, ""); err != nil {
				return err
			}
			booking, err := r.Grocy.AddStock(ctx, grocyID, addStockBody{
				Amount:          1,
				BestBefore:      sample.BestBefore,
				Purchased:       sample.Purchased,
				TransactionType: "purchase",
				LocationID:      r.locationID,
				Note:            note,
			})
			if err != nil {
				return fmt.Errorf("could not add stock in Grocy: %w", err)
			}
			stockID = booking.StockID
		} else if found, err := r.findEntry(ctx, grocyID, note); err != nil {
			return err
		} else if found.StockID != "" {
			stockID = found.StockID
		}
		spoiled := false
		if rm, ok := removals[link.PantryID]; ok && rm.Reason == "expired" {
			spoiled = true
		}
		booking, err := r.Grocy.Consume(ctx, grocyID, consumeBody{
			Amount:          1,
			TransactionType: "consume",
			Spoiled:         spoiled,
			LocationID:      r.locationID,
			StockEntryID:    stockID,
		})
		if err != nil {
			return fmt.Errorf("could not record a use in Grocy: %w", err)
		}
		return r.Map.Put(Link{
			PantryType: linkConsume,
			PantryID:   link.PantryID,
			GrocyType:  "stock",
			GrocyRef:   booking.TransactionID,
			Detail:     link.Detail,
		})
	}
	if err := r.undoConsumeLink(ctx, link.PantryID); err != nil {
		return err
	}
	if err := r.undoPurchases(ctx, purchases, link.GrocyRef); err != nil {
		return err
	}
	return r.Map.Delete(linkInstance, link.PantryID)
}

func (r *Reconciler) undoConsumeLink(ctx context.Context, instanceID string) error {
	consume, ok, err := r.Map.Get(linkConsume, instanceID)
	if err != nil {
		return err
	}
	if !ok || consume.GrocyRef == "" {
		return nil
	}
	if err := r.Grocy.Undo(ctx, consume.GrocyRef); err != nil {
		return fmt.Errorf("could not undo a Grocy booking: %w", err)
	}
	return r.Map.Delete(linkConsume, instanceID)
}

func (r *Reconciler) clearLeftover(ctx context.Context, productID int, note string, purchases []StockLog) error {
	found, err := r.findEntry(ctx, productID, note)
	if err != nil {
		return err
	}
	if found.StockID == "" {
		return nil
	}
	// A purchase undo removes the whole stock row, so the newest extra takes the leftover amount with it.
	for i := len(purchases) - 1; i >= 0; i-- {
		if err := r.Grocy.Undo(ctx, purchases[i].TransactionID.String()); err != nil {
			return fmt.Errorf("could not undo a Grocy booking: %w", err)
		}
		found, err = r.findEntry(ctx, productID, note)
		if err != nil {
			return err
		}
		if found.StockID == "" {
			return nil
		}
	}
	found, err = r.findEntry(ctx, productID, note)
	if err != nil {
		return err
	}
	if found.StockID == "" {
		return nil
	}
	if _, err := r.Grocy.Consume(ctx, productID, consumeBody{
		Amount:          1,
		TransactionType: "consume",
		LocationID:      r.locationID,
		StockEntryID:    found.StockID,
	}); err != nil {
		return fmt.Errorf("could not record a use in Grocy: %w", err)
	}
	return nil
}

func (r *Reconciler) collapsePurchases(ctx context.Context, productID int, note string) (StockLog, error) {
	rows, err := r.openPurchases(ctx, productID, note)
	if err != nil || len(rows) == 0 {
		return StockLog{}, err
	}
	if len(rows) == 1 {
		return rows[0], nil
	}
	// Two purchases with one pantry-instance note share a stock_id and summed amount.
	// Undoing either purchase deletes that stock row, and an older booking cannot be
	// undone while a newer one on the same stock_id is still open. Undo newest first,
	// then the caller adds the single unit back.
	if err := r.undoPurchases(ctx, rows, ""); err != nil {
		return StockLog{}, err
	}
	return StockLog{}, nil
}

func (r *Reconciler) undoPurchases(ctx context.Context, rows []StockLog, also string) error {
	seen := map[string]bool{}
	for i := len(rows) - 1; i >= 0; i-- {
		id := rows[i].TransactionID.String()
		seen[id] = true
		if err := r.Grocy.Undo(ctx, id); err != nil {
			return fmt.Errorf("could not undo a Grocy booking: %w", err)
		}
	}
	if also != "" && !seen[also] {
		if err := r.Grocy.Undo(ctx, also); err != nil {
			return fmt.Errorf("could not undo a Grocy booking: %w", err)
		}
	}
	return nil
}

func (r *Reconciler) openPurchases(ctx context.Context, productID int, note string) ([]StockLog, error) {
	rows, err := r.Grocy.StockLogs(ctx, productID)
	if err != nil {
		return nil, err
	}
	var out []StockLog
	for _, row := range rows {
		if row.note() != note || row.undone() || row.TransactionType != "purchase" || row.TransactionID.String() == "" {
			continue
		}
		out = append(out, row)
	}
	for i := 1; i < len(out); i++ {
		row := out[i]
		j := i
		for j > 0 && earlierLog(row, out[j-1]) {
			out[j] = out[j-1]
			j--
		}
		out[j] = row
	}
	return out, nil
}

func earlierLog(a, b StockLog) bool {
	ai, aerr := strconv.Atoi(a.ID.String())
	bi, berr := strconv.Atoi(b.ID.String())
	if aerr == nil && berr == nil && ai != bi {
		return ai < bi
	}
	if a.Created != b.Created {
		return a.Created < b.Created
	}
	return a.TransactionID.String() < b.TransactionID.String()
}

func (r *Reconciler) grocyProduct(pantryID string, fresh map[string]int) (int, error) {
	if id := fresh[pantryID]; id > 0 {
		return id, nil
	}
	link, ok, err := r.Map.Get(linkProduct, pantryID)
	if err != nil {
		return 0, err
	}
	if !ok || link.GrocyID == 0 {
		return 0, fmt.Errorf("could not find a product in Grocy")
	}
	return link.GrocyID, nil
}

func (r *Reconciler) stockID(ctx context.Context, productID int, note string) (string, error) {
	entry, err := r.findEntry(ctx, productID, note)
	if err != nil {
		return "", err
	}
	return entry.StockID, nil
}

func (r *Reconciler) findEntry(ctx context.Context, productID int, note string) (StockEntry, error) {
	entries, err := r.Grocy.StockEntries(ctx, productID)
	if err != nil {
		return StockEntry{}, err
	}
	for _, entry := range entries {
		if entry.note() == note {
			return entry, nil
		}
	}
	return StockEntry{}, nil
}
