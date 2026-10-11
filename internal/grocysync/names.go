package grocysync

import (
	"sort"
	"strings"

	"github.com/Rhionin/pantry/internal/syncsnap"
)

// assignGrocyNames gives every parent and product a case-insensitive unique
// Grocy name. The same snapshot always produces the same names.
// A parent that shares a name with a product or another parent is "Name (group)".
// Products that share a name keep a brand, then a size, then a short Pantry id.
func assignGrocyNames(snap syncsnap.Snapshot) (parents, products map[string]string) {
	byFold := map[string][]syncsnap.Product{}
	for _, p := range snap.Products {
		key := foldName(p.Name)
		byFold[key] = append(byFold[key], p)
	}
	for key := range byFold {
		sort.Slice(byFold[key], func(i, j int) bool { return byFold[key][i].ID < byFold[key][j].ID })
	}
	products = map[string]string{}
	for _, list := range byFold {
		if len(list) == 1 {
			products[list[0].ID] = strings.TrimSpace(list[0].Name)
			continue
		}
		details := distinguishingDetails(list)
		for _, p := range list {
			products[p.ID] = strings.TrimSpace(p.Name) + " (" + details[p.ID] + ")"
		}
	}

	parentFold := map[string][]syncsnap.Group{}
	for _, g := range snap.Groups {
		key := foldName(g.Name)
		parentFold[key] = append(parentFold[key], g)
	}
	parents = map[string]string{}
	for _, g := range snap.Groups {
		base := strings.TrimSpace(g.Name)
		if len(byFold[foldName(g.Name)]) > 0 || len(parentFold[foldName(g.Name)]) > 1 {
			base += " (group)"
		}
		parents[g.ID] = base
	}
	disambiguate(parents, products)
	return parents, products
}

func distinguishingDetails(list []syncsnap.Product) map[string]string {
	brandCount := map[string]int{}
	sizeCount := map[string]int{}
	brands := map[string]string{}
	sizes := map[string]string{}
	for _, p := range list {
		if brand := strings.TrimSpace(p.Brand); brand != "" {
			brands[p.ID] = brand
			brandCount[foldName(brand)]++
		}
		if size := sizeNote(p); size != "" {
			sizes[p.ID] = size
			sizeCount[foldName(size)]++
		}
	}
	out := map[string]string{}
	var needID []string
	for _, p := range list {
		if brand := brands[p.ID]; brand != "" && brandCount[foldName(brand)] == 1 {
			out[p.ID] = brand
			continue
		}
		if size := sizes[p.ID]; size != "" && sizeCount[foldName(size)] == 1 {
			out[p.ID] = size
			continue
		}
		needID = append(needID, p.ID)
	}
	for id, short := range shortPantryIDs(needID) {
		out[id] = short
	}
	return out
}

func shortPantryIDs(ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	maxLen := 0
	for _, id := range ids {
		if len(id) > maxLen {
			maxLen = len(id)
		}
	}
	if maxLen < 8 {
		maxLen = 8
	}
	for n := 8; n <= maxLen; n++ {
		seen := map[string]string{}
		clear := true
		next := map[string]string{}
		for _, id := range ids {
			short := id
			if len(short) > n {
				short = short[:n]
			}
			if prev, ok := seen[short]; ok && prev != id {
				clear = false
				break
			}
			seen[short] = id
			next[id] = short
		}
		if clear {
			return next
		}
	}
	for _, id := range ids {
		out[id] = id
	}
	return out
}

func disambiguate(parents, products map[string]string) {
	type item struct {
		kind string
		id   string
	}
	var items []item
	for id := range parents {
		items = append(items, item{"parent", id})
	}
	for id := range products {
		items = append(items, item{"product", id})
	}
	sort.Slice(items, func(i, j int) bool {
		ni, nj := lookupName(parents, products, items[i].kind, items[i].id), lookupName(parents, products, items[j].kind, items[j].id)
		if foldName(ni) != foldName(nj) {
			return foldName(ni) < foldName(nj)
		}
		if items[i].id != items[j].id {
			return items[i].id < items[j].id
		}
		return items[i].kind < items[j].kind
	})
	used := map[string]bool{}
	for _, it := range items {
		name := lookupName(parents, products, it.kind, it.id)
		if used[foldName(name)] {
			name = name + " (" + shortPantryIDs([]string{it.id})[it.id] + ")"
		}
		if used[foldName(name)] {
			name = strings.TrimSpace(name) + " (" + it.id + ")"
		}
		used[foldName(name)] = true
		if it.kind == "parent" {
			parents[it.id] = name
		} else {
			products[it.id] = name
		}
	}
}

func lookupName(parents, products map[string]string, kind, id string) string {
	if kind == "parent" {
		return parents[id]
	}
	return products[id]
}

func nameLines(grocyName, original string, lines ...string) []string {
	original = strings.TrimSpace(original)
	if original != "" && strings.TrimSpace(grocyName) != original {
		return append([]string{"Name: " + original}, lines...)
	}
	return lines
}

func foldName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
