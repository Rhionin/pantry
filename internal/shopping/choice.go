package shopping

import "sort"

// ReplenishmentItem is one pantry item named on a shopping line.
type ReplenishmentItem struct {
	ItemID         string
	Name           string
	UnitOfMeasure  string
	HasTarget      bool
	TargetQuantity int
	CurrentCount   int
}

// MergeDeals combines retailer prices with household notes. A recorded sale
// wins on the same item so a store feed cannot overwrite a price the household set.
func MergeDeals(recorded, live []Deal) []Deal {
	byItem := make(map[string]Deal, len(live)+len(recorded))
	for _, deal := range live {
		if deal.ItemID == "" {
			continue
		}
		byItem[deal.ItemID] = deal
	}
	for _, deal := range recorded {
		if deal.ItemID == "" {
			continue
		}
		byItem[deal.ItemID] = deal
	}
	if len(byItem) == 0 {
		return nil
	}
	out := make([]Deal, 0, len(byItem))
	for _, deal := range byItem {
		out = append(out, deal)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemID < out[j].ItemID })
	return out
}

// CombineDeals keeps recorded sales when the live lookup fails. A retailer
// outage must not hide notes the household already saved or block the cart.
func CombineDeals(recorded, live []Deal, liveErr error) []Deal {
	if liveErr != nil {
		live = nil
	}
	return MergeDeals(recorded, live)
}
