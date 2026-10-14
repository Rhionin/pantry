package grocysync

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Rhionin/pantry/internal/syncsnap"
)

// PantrySource is the read API the sidecar is allowed to call.
type PantrySource interface {
	Snapshot(ctx context.Context) (syncsnap.Snapshot, error)
}

// Reconciler copies one Pantry snapshot into Grocy.
// It does not write Grocy's stock journal history. That is Backfill.
type Reconciler struct {
	Pantry PantrySource
	Grocy  *API
	Map    *Map
	Now    func() time.Time
	Loc    *time.Location

	locationID int
	unitID     int
}

// Reconcile is safe to run on a timer. A second run with the same pantry
// state does not create a second copy.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Loc == nil {
		r.Loc = time.UTC
	}
	snap, err := r.Pantry.Snapshot(ctx)
	if err != nil {
		return err
	}
	if err := r.prepare(ctx); err != nil {
		return err
	}
	labels, err := r.syncLabels(ctx, snap)
	if err != nil {
		return err
	}
	parents, err := r.syncParents(ctx, snap, labels)
	if err != nil {
		return err
	}
	products, err := r.syncProducts(ctx, snap, labels, parents)
	if err != nil {
		return err
	}
	if err := r.syncBarcodes(ctx, snap, products); err != nil {
		return err
	}
	if err := r.syncStock(ctx, snap, products); err != nil {
		return err
	}
	if err := r.syncShopping(ctx, snap, products, parents); err != nil {
		return err
	}
	if err := r.deleteStale(ctx, snap); err != nil {
		return err
	}
	log.Printf("grocy-sync: reconciled %d products, %d on hand", len(snap.Products), len(snap.Instances))
	return nil
}

func (r *Reconciler) prepare(ctx context.Context) error {
	var err error
	r.locationID, err = r.ensureNamed(ctx, "locations", linkLocation, "Pantry", map[string]any{
		"name":        "Pantry",
		"description": "Synced from Pantry",
		"is_freezer":  0,
	})
	if err != nil {
		return err
	}
	r.unitID, err = r.ensureNamed(ctx, "quantity_units", linkUnit, "Piece", map[string]any{
		"name":        "Piece",
		"name_plural": "Pieces",
		"description": "One item",
	})
	if err != nil {
		return err
	}
	return r.ensureUserfields(ctx)
}

func (r *Reconciler) ensureNamed(ctx context.Context, entity, linkType, name string, body any) (int, error) {
	rows, err := r.Grocy.List(ctx, entity)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if row.Name == name {
			if err := r.Map.Put(Link{PantryType: linkType, PantryID: name, GrocyType: entity, GrocyID: row.ID}); err != nil {
				return 0, err
			}
			return row.ID, nil
		}
	}
	id, err := r.Grocy.Create(ctx, entity, body)
	if err != nil {
		return 0, fmt.Errorf("could not create %s in Grocy", name)
	}
	if err := r.Map.Put(Link{PantryType: linkType, PantryID: name, GrocyType: entity, GrocyID: id}); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Reconciler) ensureUserfields(ctx context.Context) error {
	rows, err := r.Grocy.List(ctx, "userfields")
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, row := range rows {
		if row.Entity == "products" {
			have[row.Name] = true
		}
	}
	want := []struct{ name, caption string }{
		{"pantry_id", "Pantry id"},
		{"pantry_name", "Pantry name"},
		{"pantry_size", "Pantry size"},
		{"pantry_rule", "Pantry group rule"},
	}
	for _, field := range want {
		if have[field.name] {
			continue
		}
		if _, err := r.Grocy.Create(ctx, "userfields", map[string]any{
			"entity":                   "products",
			"name":                     field.name,
			"caption":                  field.caption,
			"type":                     "text-single-line",
			"show_as_column_in_tables": 0,
			"input_required":           0,
		}); err != nil {
			return fmt.Errorf("could not create the %s field in Grocy", field.caption)
		}
	}
	return nil
}

type productWrite struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	LocationID        int    `json:"location_id"`
	QUIDPurchase      int    `json:"qu_id_purchase"`
	QUIDStock         int    `json:"qu_id_stock"`
	MinStock          int    `json:"min_stock_amount"`
	DefaultBestBefore int    `json:"default_best_before_days"`
	ParentProductID   *int   `json:"parent_product_id"`
	ProductGroupID    *int   `json:"product_group_id"`
	Cumulate          int    `json:"cumulate_min_stock_amount_of_sub_products"`
	NoOwnStock        int    `json:"no_own_stock"`
	Active            int    `json:"active"`
}

func baseProduct(name, desc string, location, unit, minStock, cumulate, noOwn int, parent, group *int) productWrite {
	return productWrite{
		Name:              name,
		Description:       desc,
		LocationID:        location,
		QUIDPurchase:      unit,
		QUIDStock:         unit,
		MinStock:          minStock,
		DefaultBestBefore: -1,
		ParentProductID:   parent,
		ProductGroupID:    group,
		Cumulate:          cumulate,
		NoOwnStock:        noOwn,
		Active:            1,
	}
}

func (r *Reconciler) syncLabels(ctx context.Context, snap syncsnap.Snapshot) (map[string]int, error) {
	needed := map[string]string{}
	for _, g := range snap.Groups {
		needed[labelKey(g.Name)] = g.Name
	}
	for _, p := range snap.Products {
		if p.GroupID == "" && p.Category != "" {
			needed[labelKey(p.Category)] = p.Category
		}
	}
	rows, err := r.Grocy.List(ctx, "product_groups")
	if err != nil {
		return nil, err
	}
	byName := map[string]int{}
	for _, row := range rows {
		byName[labelKey(row.Name)] = row.ID
	}
	out := map[string]int{}
	for key, name := range needed {
		id, ok := byName[key]
		if !ok {
			var err error
			id, err = r.Grocy.Create(ctx, "product_groups", map[string]any{
				"name":        name,
				"description": markerLine(key),
			})
			if err != nil {
				return nil, fmt.Errorf("could not create the %s group in Grocy", name)
			}
			byName[key] = id
		}
		if err := r.Map.Put(Link{PantryType: linkLabel, PantryID: key, GrocyType: "product_groups", GrocyID: id}); err != nil {
			return nil, err
		}
		out[key] = id
	}
	return out, nil
}

func (r *Reconciler) syncParents(ctx context.Context, snap syncsnap.Snapshot, labels map[string]int) (map[string]int, error) {
	listed, err := r.indexProducts(ctx)
	if err != nil {
		return nil, err
	}
	parentNames, _ := assignGrocyNames(snap)
	out := map[string]int{}
	for _, g := range snap.Groups {
		marker := parentMarker(g.ID)
		var groupID *int
		if id, ok := labels[labelKey(g.Name)]; ok {
			groupID = &id
		}
		grocyName := parentNames[g.ID]
		body := baseProduct(grocyName, description(marker, nameLines(grocyName, g.Name, "Rule: "+ruleNote(g))...), r.locationID, r.unitID, 0, 1, 1, nil, groupID)
		fields := productFields(marker, "", ruleNote(g))
		fields["pantry_name"] = strings.TrimSpace(g.Name)
		id, err := r.upsertProduct(ctx, linkParent, g.ID, marker, body, fields, listed)
		if err != nil {
			return nil, fmt.Errorf("could not save the %s group in Grocy: %w", g.Name, err)
		}
		out[g.ID] = id
		listed.byMarker[marker] = id
		listed.byPantry[marker] = id
		listed.ids[id] = true
	}
	return out, nil
}

func (r *Reconciler) syncProducts(ctx context.Context, snap syncsnap.Snapshot, labels map[string]int, parents map[string]int) (map[string]int, error) {
	listed, err := r.indexProducts(ctx)
	if err != nil {
		return nil, err
	}
	groups := map[string]syncsnap.Group{}
	for _, g := range snap.Groups {
		groups[g.ID] = g
	}
	_, productNames := assignGrocyNames(snap)
	out := map[string]int{}
	for _, p := range snap.Products {
		var parent *int
		var group *int
		if p.GroupID != "" {
			if id, ok := parents[p.GroupID]; ok {
				parent = &id
			}
			if g, ok := groups[p.GroupID]; ok {
				if id, ok := labels[labelKey(g.Name)]; ok {
					group = &id
				}
			}
		} else if p.Category != "" {
			if id, ok := labels[labelKey(p.Category)]; ok {
				group = &id
			}
		}
		minStock := 0
		if p.TargetQuantity != nil && *p.TargetQuantity > 0 {
			minStock = *p.TargetQuantity
		}
		lines := []string{}
		if size := sizeNote(p); size != "" {
			lines = append(lines, "Size: "+size)
		}
		if p.Category != "" {
			lines = append(lines, "Category: "+p.Category)
		}
		if p.NoRestock {
			lines = append(lines, "Not restocked")
		}
		grocyName := productNames[p.ID]
		body := baseProduct(grocyName, description(p.ID, nameLines(grocyName, p.Name, lines...)...), r.locationID, r.unitID, minStock, 0, 0, parent, group)
		fields := productFields(p.ID, sizeNote(p), "")
		fields["pantry_name"] = strings.TrimSpace(p.Name)
		id, err := r.upsertProduct(ctx, linkProduct, p.ID, p.ID, body, fields, listed)
		if err != nil {
			return nil, fmt.Errorf("could not save %s in Grocy: %w", p.Name, err)
		}
		out[p.ID] = id
		listed.byMarker[p.ID] = id
		listed.byPantry[p.ID] = id
		listed.ids[id] = true
	}
	return out, nil
}

type productIndex struct {
	byMarker map[string]int
	byPantry map[string]int
	ids      map[int]bool
}

func (r *Reconciler) indexProducts(ctx context.Context) (productIndex, error) {
	rows, err := r.Grocy.List(ctx, "products")
	if err != nil {
		return productIndex{}, err
	}
	idx := productIndex{byMarker: map[string]int{}, byPantry: map[string]int{}, ids: map[int]bool{}}
	for _, row := range rows {
		idx.ids[row.ID] = true
		if marker := markerID(row.description()); marker != "" {
			idx.byMarker[marker] = row.ID
		}
		fields, err := r.Grocy.Userfields(ctx, row.ID)
		if err != nil {
			return productIndex{}, err
		}
		if pantryID := strings.TrimSpace(fields["pantry_id"]); pantryID != "" {
			idx.byPantry[pantryID] = row.ID
		}
	}
	return idx, nil
}

func (r *Reconciler) upsertProduct(ctx context.Context, linkType, pantryID, marker string, body productWrite, fields map[string]string, listed productIndex) (int, error) {
	hash := hashState(body, fields)
	link, ok, err := r.Map.Get(linkType, pantryID)
	if err != nil {
		return 0, err
	}
	if ok && link.GrocyID > 0 && link.Hash == hash && listed.ids[link.GrocyID] {
		return link.GrocyID, nil
	}
	id := 0
	if ok && link.GrocyID > 0 && listed.ids[link.GrocyID] {
		id = link.GrocyID
	} else if existing, found := listed.byPantry[marker]; found {
		id = existing
	} else if existing, found := listed.byMarker[marker]; found {
		id = existing
	}
	if id > 0 {
		if err := r.Grocy.Update(ctx, "products", id, body); err != nil {
			return 0, fmt.Errorf("could not save the product in Grocy: %w", err)
		}
	} else {
		var err error
		id, err = r.Grocy.Create(ctx, "products", body)
		if err != nil {
			return 0, fmt.Errorf("could not save the product in Grocy: %w", err)
		}
	}
	if err := r.Grocy.PutUserfields(ctx, id, fields); err != nil {
		return 0, err
	}
	if err := r.Map.Put(Link{
		PantryType: linkType,
		PantryID:   pantryID,
		GrocyType:  "products",
		GrocyID:    id,
		Hash:       hash,
	}); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Reconciler) syncBarcodes(ctx context.Context, snap syncsnap.Snapshot, products map[string]int) error {
	rows, err := r.Grocy.List(ctx, "product_barcodes")
	if err != nil {
		return err
	}
	byCode := map[string]objectRow{}
	for _, row := range rows {
		if row.Barcode != "" {
			byCode[row.Barcode] = row
		}
	}
	want := map[string]string{}
	for _, p := range snap.Products {
		for _, code := range p.Barcodes {
			want[code] = p.ID
		}
	}
	for code, pantryID := range want {
		grocyID := products[pantryID]
		if grocyID == 0 {
			return fmt.Errorf("could not attach barcode %s because the product is not in Grocy", code)
		}
		hash := hashState(map[string]any{"barcode": code, "product_id": grocyID}, nil)
		if row, ok := byCode[code]; ok {
			if row.ProductID != grocyID || row.note() != barcodeNote {
				if err := r.Grocy.Update(ctx, "product_barcodes", row.ID, map[string]any{
					"barcode":    code,
					"product_id": grocyID,
					"note":       barcodeNote,
				}); err != nil {
					return fmt.Errorf("could not update barcode %s", code)
				}
			}
			if err := r.Map.Put(Link{PantryType: linkBarcode, PantryID: code, GrocyType: "product_barcodes", GrocyID: row.ID, Hash: hash}); err != nil {
				return err
			}
			continue
		}
		id, err := r.Grocy.Create(ctx, "product_barcodes", map[string]any{
			"barcode":    code,
			"product_id": grocyID,
			"note":       barcodeNote,
		})
		if err != nil {
			return fmt.Errorf("could not save barcode %s", code)
		}
		if err := r.Map.Put(Link{PantryType: linkBarcode, PantryID: code, GrocyType: "product_barcodes", GrocyID: id, Hash: hash}); err != nil {
			return err
		}
	}
	mapped, err := r.Map.ByType(linkBarcode)
	if err != nil {
		return err
	}
	owned := map[string]Link{}
	for _, link := range mapped {
		owned[link.PantryID] = link
	}
	for code, row := range byCode {
		if _, ok := want[code]; ok {
			continue
		}
		_, ours := owned[code]
		if !ours && row.note() != barcodeNote {
			continue
		}
		if err := r.Grocy.Delete(ctx, "product_barcodes", row.ID); err != nil {
			return fmt.Errorf("could not remove barcode %s", code)
		}
		if err := r.Map.Delete(linkBarcode, code); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) syncShopping(ctx context.Context, snap syncsnap.Snapshot, products, parents map[string]int) error {
	listID, err := r.ensureShoppingList(ctx)
	if err != nil {
		return err
	}
	rows, err := r.Grocy.List(ctx, "shopping_list")
	if err != nil {
		return err
	}
	byNote := map[string]objectRow{}
	for _, row := range rows {
		if row.ShoppingListID != listID {
			continue
		}
		if note := row.note(); note != "" {
			byNote[note] = row
		}
	}
	want := map[string]bool{}
	for _, line := range snap.ShoppingList {
		note := linePrefix + line.ID
		want[note] = true
		grocyProduct := products[line.ProductID]
		if line.GroupID != "" {
			if id, ok := parents[line.GroupID]; ok {
				grocyProduct = id
			}
		}
		if grocyProduct == 0 {
			return fmt.Errorf("could not add a shopping line because the product is not in Grocy")
		}
		body := map[string]any{
			"product_id":       grocyProduct,
			"shopping_list_id": listID,
			"amount":           line.Quantity,
			"note":             note,
			"done":             0,
		}
		hash := hashState(body, nil)
		if row, ok := byNote[note]; ok {
			// Truncating the amount hides 2.5, and a checked-off row was left checked off.
			if row.ProductID != grocyProduct || float64(row.Amount) != float64(line.Quantity) || float64(row.Done) != 0 {
				if err := r.Grocy.Update(ctx, "shopping_list", row.ID, body); err != nil {
					return fmt.Errorf("could not update the Pantry shopping list")
				}
			}
			if err := r.Map.Put(Link{PantryType: linkShopItem, PantryID: line.ID, GrocyType: "shopping_list", GrocyID: row.ID, Hash: hash}); err != nil {
				return err
			}
			continue
		}
		id, err := r.Grocy.Create(ctx, "shopping_list", body)
		if err != nil {
			return fmt.Errorf("could not update the Pantry shopping list")
		}
		if err := r.Map.Put(Link{PantryType: linkShopItem, PantryID: line.ID, GrocyType: "shopping_list", GrocyID: id, Hash: hash}); err != nil {
			return err
		}
	}
	// Anything else on this list is not Pantry's list anymore. Other lists are not read.
	for _, row := range rows {
		if row.ShoppingListID != listID || want[row.note()] {
			continue
		}
		if err := r.Grocy.Delete(ctx, "shopping_list", row.ID); err != nil {
			return fmt.Errorf("could not update the Pantry shopping list")
		}
	}
	// Drop map rows for lines that are gone.
	mapped, err := r.Map.ByType(linkShopItem)
	if err != nil {
		return err
	}
	for _, link := range mapped {
		if !want[linePrefix+link.PantryID] {
			if err := r.Map.Delete(linkShopItem, link.PantryID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) ensureShoppingList(ctx context.Context) (int, error) {
	rows, err := r.Grocy.List(ctx, "shopping_lists")
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if row.Name == "Pantry" {
			if err := r.Map.Put(Link{PantryType: linkShopList, PantryID: "Pantry", GrocyType: "shopping_lists", GrocyID: row.ID}); err != nil {
				return 0, err
			}
			return row.ID, nil
		}
	}
	id, err := r.Grocy.Create(ctx, "shopping_lists", map[string]any{"name": "Pantry"})
	if err != nil {
		return 0, fmt.Errorf("could not create the Pantry shopping list")
	}
	if err := r.Map.Put(Link{PantryType: linkShopList, PantryID: "Pantry", GrocyType: "shopping_lists", GrocyID: id}); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Reconciler) deleteStale(ctx context.Context, snap syncsnap.Snapshot) error {
	wantProducts := map[string]bool{}
	for _, p := range snap.Products {
		wantProducts[p.ID] = true
	}
	wantParents := map[string]bool{}
	wantLabels := map[string]bool{}
	for _, g := range snap.Groups {
		wantParents[g.ID] = true
		wantLabels[labelKey(g.Name)] = true
	}
	for _, p := range snap.Products {
		if p.GroupID == "" && p.Category != "" {
			wantLabels[labelKey(p.Category)] = true
		}
	}
	if err := r.deleteTyped(ctx, linkProduct, "products", wantProducts); err != nil {
		return err
	}
	if err := r.deleteTyped(ctx, linkParent, "products", wantParents); err != nil {
		return err
	}
	labels, err := r.Map.ByType(linkLabel)
	if err != nil {
		return err
	}
	for _, link := range labels {
		if wantLabels[link.PantryID] {
			continue
		}
		if err := r.Grocy.Delete(ctx, "product_groups", link.GrocyID); err != nil {
			log.Printf("grocy-sync: could not remove unused group label %s", link.PantryID)
			continue
		}
		if err := r.Map.Delete(linkLabel, link.PantryID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) deleteTyped(ctx context.Context, linkType, entity string, want map[string]bool) error {
	links, err := r.Map.ByType(linkType)
	if err != nil {
		return err
	}
	for _, link := range links {
		if want[link.PantryID] {
			continue
		}
		if err := r.Grocy.Delete(ctx, entity, link.GrocyID); err != nil {
			if entity == "products" {
				_ = r.Grocy.Update(ctx, entity, link.GrocyID, map[string]any{"active": 0})
			}
			log.Printf("grocy-sync: could not remove %s", link.PantryID)
			continue
		}
		if err := r.Map.Delete(linkType, link.PantryID); err != nil {
			return err
		}
	}
	return nil
}
