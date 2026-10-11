package grocysync

import (
	"context"
	"fmt"

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
	if consume, ok, err := r.Map.Get(linkConsume, inst.ID); err != nil {
		return err
	} else if ok && consume.GrocyRef != "" {
		// The unit is back on the shelf. Undo the consume, then keep or replace the purchase.
		if err := r.Grocy.Undo(ctx, consume.GrocyRef); err != nil {
			return fmt.Errorf("could not undo a Grocy booking")
		}
		if err := r.Map.Delete(linkConsume, inst.ID); err != nil {
			return err
		}
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
	if found, err := r.findEntry(ctx, grocyID, note); err != nil {
		return err
	} else if found.StockID != "" && sameDay(found.BestBefore, wantBest) && sameDay(found.Purchased, wantPurchased) {
		ref := ""
		if ok {
			ref = link.GrocyRef
		}
		return r.Map.Put(Link{
			PantryType: linkInstance,
			PantryID:   inst.ID,
			GrocyType:  "stock",
			GrocyRef:   ref,
			Hash:       hash,
			Detail:     packDetail(inst.StockMoveID, found.StockID),
		})
	}
	if ok && link.Hash == hash && link.GrocyRef != "" {
		return nil
	}
	if ok && link.GrocyRef != "" {
		if err := r.Grocy.Undo(ctx, link.GrocyRef); err != nil {
			return fmt.Errorf("could not undo a Grocy booking")
		}
	} else if found, err := r.findEntry(ctx, grocyID, note); err != nil {
		return err
	} else if found.StockID != "" {
		// The map was lost and the dates changed. Consume the old row, then re-post.
		if _, err := r.Grocy.Consume(ctx, grocyID, consumeBody{
			Amount:          1,
			TransactionType: "consume",
			LocationID:      r.locationID,
			StockEntryID:    found.StockID,
		}); err != nil {
			return fmt.Errorf("could not undo a Grocy booking")
		}
	}
	ref, err := r.Grocy.AddStock(ctx, grocyID, addStockBody{
		Amount:          1,
		BestBefore:      bestBefore(inst.ExpiresAt, r.Loc),
		Purchased:       day(inst.StockInAt, r.Loc),
		TransactionType: "purchase",
		LocationID:      r.locationID,
		Note:            note,
	})
	if err != nil {
		return fmt.Errorf("could not add stock in Grocy")
	}
	stockID, err := r.stockID(ctx, grocyID, note)
	if err != nil {
		return err
	}
	return r.Map.Put(Link{
		PantryType: linkInstance,
		PantryID:   inst.ID,
		GrocyType:  "stock",
		GrocyRef:   ref,
		Hash:       hash,
		Detail:     packDetail(inst.StockMoveID, stockID),
	})
}

func (r *Reconciler) dropInstance(ctx context.Context, link Link, moves map[string]bool, removals map[string]syncsnap.Removal, products map[string]int) error {
	moveID, stockID := unpackDetail(link.Detail)
	inMoveAlive := moveID != "" && moves[moveID]
	if inMoveAlive {
		if consume, ok, err := r.Map.Get(linkConsume, link.PantryID); err != nil {
			return err
		} else if ok && consume.GrocyRef != "" {
			return nil
		}
		grocyID, err := r.grocyProduct(productIDFromHash(link.Hash), products)
		if err != nil {
			return err
		}
		spoiled := false
		if rm, ok := removals[link.PantryID]; ok && rm.Reason == "expired" {
			spoiled = true
		}
		ref, err := r.Grocy.Consume(ctx, grocyID, consumeBody{
			Amount:          1,
			TransactionType: "consume",
			Spoiled:         spoiled,
			LocationID:      r.locationID,
			StockEntryID:    stockID,
		})
		if err != nil {
			return fmt.Errorf("could not record a use in Grocy")
		}
		return r.Map.Put(Link{
			PantryType: linkConsume,
			PantryID:   link.PantryID,
			GrocyType:  "stock",
			GrocyRef:   ref,
			Detail:     link.Detail,
		})
	}
	if consume, ok, err := r.Map.Get(linkConsume, link.PantryID); err != nil {
		return err
	} else if ok && consume.GrocyRef != "" {
		if err := r.Grocy.Undo(ctx, consume.GrocyRef); err != nil {
			return fmt.Errorf("could not undo a Grocy booking")
		}
		if err := r.Map.Delete(linkConsume, link.PantryID); err != nil {
			return err
		}
	}
	if link.GrocyRef != "" {
		if err := r.Grocy.Undo(ctx, link.GrocyRef); err != nil {
			return fmt.Errorf("could not undo a Grocy booking")
		}
	}
	return r.Map.Delete(linkInstance, link.PantryID)
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
	// An empty list means this Grocy reply did not include entries. Callers can still undo by transaction id.
	return StockEntry{}, nil
}
