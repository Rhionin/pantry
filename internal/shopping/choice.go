package shopping

import (
	"errors"
	"sort"
	"strings"
)

// ErrDifferentProduct is returned when a cart substitution picks a brand that
// is not part of the same replenishment need.
var ErrDifferentProduct = errors.New("that brand is a different product")

// MemberView is one brand that can fill a shared need, with any sale we know.
type MemberView struct {
	ItemID     string
	Name       string
	PriceCents *int
	OnSale     bool
	SaleLabel  string
	DealSource string
}

// Offer is a sale on a brand other than the one the list is about to buy.
// Accepting it is optional and does not change the saved preference.
type Offer struct {
	ItemID          string
	Name            string
	Label           string
	PriceCents      *int
	UsualPriceCents *int
	Source          string
}

// Consideration is the optional brand note for one shopping line.
type Consideration struct {
	LineItemID      string
	NeedKey         string
	GenericName     string
	ChosenItemID    string
	PreferredItemID string
	IgnorePrice     bool
	Members         []MemberView
	Offer           *Offer
}

// NeedKey is the shared-need identity used by brand preferences: the generic
// product name plus the normalized unit. ok is false when the name is only a brand.
func NeedKey(name, unit string) (string, bool) {
	return equivalenceKey(name, unit)
}

// ConsiderationsForLines builds optional brand notes for the lines that will
// actually be shown or exported. A sale on another member of the same need is
// an offer unless the saved preference says price is not the point. The offer
// does not change ChosenItemID; the shopper accepts it when exporting.
func ConsiderationsForLines(lineItemIDs []string, items []ReplenishmentItem, deals []Deal, prefs []Preference) []Consideration {
	if len(lineItemIDs) == 0 {
		return nil
	}
	groups := groupByNeed(items)
	dealByItem := indexDeals(deals)
	prefByNeed := indexPreferences(prefs)

	out := make([]Consideration, 0)
	seen := make(map[string]struct{}, len(lineItemIDs))
	for _, lineID := range lineItemIDs {
		if _, dup := seen[lineID]; dup {
			continue
		}
		seen[lineID] = struct{}{}
		item, ok := indexItems(items)[lineID]
		if !ok {
			continue
		}
		key, groupable := equivalenceKey(item.Name, item.UnitOfMeasure)
		if !groupable {
			continue
		}
		members := groups[key]
		if len(members) < 2 {
			continue
		}
		views := memberViews(members, dealByItem)
		pref, hasPref := prefByNeed[key]
		var preferredID string
		ignore := false
		if hasPref {
			if _, inGroup := indexItems(members)[pref.ItemID]; inGroup {
				preferredID = pref.ItemID
				ignore = pref.IgnorePrice
			}
		}
		var offer *Offer
		if !ignore {
			offer = bestOffer(lineID, views)
		}
		out = append(out, Consideration{
			LineItemID:      lineID,
			NeedKey:         key,
			GenericName:     GenericProductName(item.Name),
			ChosenItemID:    lineID,
			PreferredItemID: preferredID,
			IgnorePrice:     ignore,
			Members:         views,
			Offer:           offer,
		})
	}
	return out
}

// SubstituteBrand returns the item to put in the cart for a line.
// An empty useItemID keeps the line's brand. A different product is refused
// so a deal acceptance cannot swap unrelated rows.
func SubstituteBrand(lineItemID, useItemID string, items []ReplenishmentItem) (string, error) {
	if useItemID == "" || useItemID == lineItemID {
		return lineItemID, nil
	}
	byID := indexItems(items)
	line, ok := byID[lineItemID]
	if !ok {
		return "", ErrDifferentProduct
	}
	use, ok := byID[useItemID]
	if !ok {
		return "", ErrDifferentProduct
	}
	lineKey, lineOK := equivalenceKey(line.Name, line.UnitOfMeasure)
	useKey, useOK := equivalenceKey(use.Name, use.UnitOfMeasure)
	if !lineOK || !useOK || lineKey != useKey {
		return "", ErrDifferentProduct
	}
	return useItemID, nil
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

func bestOffer(chosenID string, members []MemberView) *Offer {
	chosen, ok := memberByID(members, chosenID)
	if !ok {
		return nil
	}
	var best *MemberView
	for i := range members {
		candidate := members[i]
		if candidate.ItemID == chosenID || !betterDeal(chosen, candidate) {
			continue
		}
		if best == nil || preferDeal(candidate, *best) {
			copy := candidate
			best = &copy
		}
	}
	if best == nil {
		return nil
	}
	label := best.SaleLabel
	if label == "" {
		label = "On sale"
	}
	return &Offer{
		ItemID:          best.ItemID,
		Name:            best.Name,
		Label:           label,
		PriceCents:      best.PriceCents,
		UsualPriceCents: chosen.PriceCents,
		Source:          best.DealSource,
	}
}

// betterDeal requires an actual sale. A known price on both brands has to be
// strictly lower, so a "sale" that costs more than the usual brand is not offered.
func betterDeal(usual, other MemberView) bool {
	if !other.OnSale {
		return false
	}
	if other.PriceCents != nil && usual.PriceCents != nil {
		return *other.PriceCents < *usual.PriceCents
	}
	return true
}

func preferDeal(candidate, current MemberView) bool {
	switch {
	case candidate.PriceCents != nil && current.PriceCents != nil && *candidate.PriceCents != *current.PriceCents:
		return *candidate.PriceCents < *current.PriceCents
	case candidate.PriceCents != nil && current.PriceCents == nil:
		return true
	case candidate.PriceCents == nil && current.PriceCents != nil:
		return false
	case candidate.Name != current.Name:
		return candidate.Name < current.Name
	default:
		return candidate.ItemID < current.ItemID
	}
}

func groupByNeed(items []ReplenishmentItem) map[string][]ReplenishmentItem {
	groups := make(map[string][]ReplenishmentItem)
	for _, item := range items {
		key, ok := equivalenceKey(item.Name, item.UnitOfMeasure)
		if !ok {
			continue
		}
		groups[key] = append(groups[key], item)
	}
	return groups
}

func memberViews(members []ReplenishmentItem, deals map[string]Deal) []MemberView {
	views := make([]MemberView, len(members))
	for i, member := range members {
		view := MemberView{ItemID: member.ItemID, Name: member.Name}
		if deal, ok := deals[member.ItemID]; ok {
			view.PriceCents = deal.PriceCents
			view.OnSale = deal.OnSale()
			view.SaleLabel = strings.TrimSpace(deal.Label)
			view.DealSource = deal.Source
		}
		views[i] = view
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Name != views[j].Name {
			return views[i].Name < views[j].Name
		}
		return views[i].ItemID < views[j].ItemID
	})
	return views
}

func indexItems(items []ReplenishmentItem) map[string]ReplenishmentItem {
	byID := make(map[string]ReplenishmentItem, len(items))
	for _, item := range items {
		byID[item.ItemID] = item
	}
	return byID
}

func indexPreferences(prefs []Preference) map[string]Preference {
	byNeed := make(map[string]Preference, len(prefs))
	for _, pref := range prefs {
		byNeed[pref.NeedKey] = pref
	}
	return byNeed
}

func indexDeals(deals []Deal) map[string]Deal {
	byItem := make(map[string]Deal, len(deals))
	for _, deal := range deals {
		byItem[deal.ItemID] = deal
	}
	return byItem
}

func memberByID(members []MemberView, itemID string) (MemberView, bool) {
	for _, member := range members {
		if member.ItemID == itemID {
			return member, true
		}
	}
	return MemberView{}, false
}
