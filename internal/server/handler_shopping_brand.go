package server

import (
	"context"
	"strings"

	"time"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/shopping"
	"github.com/Rhionin/pantry/internal/supply"
)

// ShoppingListConsiderationsHandler handles GET /api/shopping-list/considerations.
// Offers are optional. Export stays available without accepting any of them.
type ShoppingListConsiderationsHandler struct {
	ShoppingList shoppingListReader
	Pantry       pantryLister
	Supply       *supply.Service
	Retailer     shopping.RetailerDealConfig
}

type brandMemberResponse struct {
	ItemID     string `json:"itemId"`
	Name       string `json:"name"`
	PriceCents *int   `json:"priceCents"`
	OnSale     bool   `json:"onSale"`
	SaleLabel  string `json:"saleLabel"`
	DealSource string `json:"dealSource"`
}

type dealOfferResponse struct {
	ItemID          string `json:"itemId"`
	Name            string `json:"name"`
	Label           string `json:"label"`
	PriceCents      *int   `json:"priceCents"`
	UsualPriceCents *int   `json:"usualPriceCents"`
	Source          string `json:"source"`
}

type considerationResponse struct {
	LineItemID      string                `json:"lineItemId"`
	NeedKey         string                `json:"needKey"`
	GenericName     string                `json:"genericName"`
	ChosenItemID    string                `json:"chosenItemId"`
	PreferredItemID string                `json:"preferredItemId"`
	IgnorePrice     bool                  `json:"ignorePrice"`
	Members         []brandMemberResponse `json:"members"`
	Offer           *dealOfferResponse    `json:"offer"`
}

type considerationsResponse struct {
	RetailerDeals  string                  `json:"retailerDeals"`
	RetailerDetail string                  `json:"retailerDetail"`
	Considerations []considerationResponse `json:"considerations"`
}

func (h *ShoppingListConsiderationsHandler) Handle(req Request[struct{}, struct{}]) (*considerationsResponse, error) {
	const userID = "user-1"

	provision, err := loadShoppingProvision(req.Context, userID, h.Pantry, h.ShoppingList, h.Supply, time.Now())
	if err != nil {
		return nil, InternalError(err)
	}

	itemIDs := make([]string, len(provision.Needs))
	for i, need := range provision.Needs {
		itemIDs[i] = need.ItemID
	}
	live, liveErr := h.Retailer.LiveDeals(req.Context, userID, itemIDs)
	deals := shopping.CombineDeals(provision.Deals, live, liveErr)

	lineIDs := make([]string, 0, len(provision.Rows))
	for _, row := range provision.Rows {
		lineIDs = append(lineIDs, row.ItemID)
	}
	notes := shopping.ConsiderationsForLines(lineIDs, provision.Needs, deals, provision.Prefs)
	status, detail := h.Retailer.Status()
	return &considerationsResponse{
		RetailerDeals:  status,
		RetailerDetail: detail,
		Considerations: considerationsToResponse(notes),
	}, nil
}

func considerationsToResponse(notes []shopping.Consideration) []considerationResponse {
	resp := make([]considerationResponse, 0, len(notes))
	for _, note := range notes {
		members := make([]brandMemberResponse, len(note.Members))
		for i, member := range note.Members {
			members[i] = brandMemberResponse{
				ItemID:     member.ItemID,
				Name:       member.Name,
				PriceCents: member.PriceCents,
				OnSale:     member.OnSale,
				SaleLabel:  member.SaleLabel,
				DealSource: member.DealSource,
			}
		}
		var offer *dealOfferResponse
		if note.Offer != nil {
			offer = &dealOfferResponse{
				ItemID:          note.Offer.ItemID,
				Name:            note.Offer.Name,
				Label:           note.Offer.Label,
				PriceCents:      note.Offer.PriceCents,
				UsualPriceCents: note.Offer.UsualPriceCents,
				Source:          note.Offer.Source,
			}
		}
		resp = append(resp, considerationResponse{
			LineItemID:      note.LineItemID,
			NeedKey:         note.NeedKey,
			GenericName:     note.GenericName,
			ChosenItemID:    note.ChosenItemID,
			PreferredItemID: note.PreferredItemID,
			IgnorePrice:     note.IgnorePrice,
			Members:         members,
			Offer:           offer,
		})
	}
	return resp
}

type shoppingPreferenceRequest struct {
	ItemID      string `json:"itemId"`
	IgnorePrice bool   `json:"ignorePrice"`
}

type shoppingPreferenceResponse struct {
	ItemID      string `json:"itemId"`
	NeedKey     string `json:"needKey"`
	GenericName string `json:"genericName"`
	IgnorePrice bool   `json:"ignorePrice"`
}

// ShoppingPreferencePutHandler handles PUT /api/shopping-list/preferences.
type ShoppingPreferencePutHandler struct {
	ShoppingList interface {
		SavePreference(ctx context.Context, userID string, pref shopping.Preference) error
	}
	Pantry interface {
		GetItem(ctx context.Context, itemID string) (*inventory.Item, error)
	}
}

func (h *ShoppingPreferencePutHandler) Handle(req Request[shoppingPreferenceRequest, struct{}]) (*shoppingPreferenceResponse, error) {
	if req.Body.ItemID == "" {
		return nil, &HTTPError{Code: 422, Message: "Choose a brand to remember"}
	}
	item, key, generic, err := preferenceNeed(req.Context, h.Pantry, req.Body.ItemID)
	if err != nil {
		return nil, err
	}
	const userID = "user-1"
	pref := shopping.Preference{NeedKey: key, ItemID: item.ID, IgnorePrice: req.Body.IgnorePrice}
	if err := h.ShoppingList.SavePreference(req.Context, userID, pref); err != nil {
		return nil, InternalError(err)
	}
	return &shoppingPreferenceResponse{
		ItemID:      item.ID,
		NeedKey:     key,
		GenericName: generic,
		IgnorePrice: pref.IgnorePrice,
	}, nil
}

type shoppingPreferencePath struct {
	ItemID string `json:"itemId"`
}

// ShoppingPreferenceDeleteHandler handles DELETE /api/shopping-list/preferences/{itemId}.
type ShoppingPreferenceDeleteHandler struct {
	ShoppingList interface {
		DeletePreference(ctx context.Context, userID, needKey string) error
	}
	Pantry interface {
		GetItem(ctx context.Context, itemID string) (*inventory.Item, error)
	}
}

func (h *ShoppingPreferenceDeleteHandler) Handle(req Request[struct{}, shoppingPreferencePath]) (struct{}, error) {
	_, key, _, err := preferenceNeed(req.Context, h.Pantry, req.PathParams.ItemID)
	if err != nil {
		return struct{}{}, err
	}
	const userID = "user-1"
	if err := h.ShoppingList.DeletePreference(req.Context, userID, key); err != nil {
		return struct{}{}, InternalError(err)
	}
	return struct{}{}, nil
}

type preferenceItemLookup interface {
	GetItem(ctx context.Context, itemID string) (*inventory.Item, error)
}

func preferenceNeed(ctx context.Context, pantry preferenceItemLookup, itemID string) (*inventory.Item, string, string, error) {
	item, err := pantry.GetItem(ctx, itemID)
	if err != nil {
		return nil, "", "", InternalError(err)
	}
	if item == nil {
		return nil, "", "", NotFound("item not found")
	}
	name, unit := "", ""
	if item.Product != nil {
		name = item.Product.Name
		unit = item.Product.UnitOfMeasure
	}
	key, ok := shopping.NeedKey(name, unit)
	if !ok {
		return nil, "", "", &HTTPError{Code: 422, Message: "This product name is only a brand, so it can't be saved as a preference"}
	}
	return item, key, shopping.GenericProductName(name), nil
}

type shoppingDealRequest struct {
	ItemID            string `json:"itemId"`
	PriceCents        *int   `json:"priceCents"`
	RegularPriceCents *int   `json:"regularPriceCents"`
	Label             string `json:"label"`
}

type shoppingDealResponse struct {
	ItemID            string `json:"itemId"`
	PriceCents        *int   `json:"priceCents"`
	RegularPriceCents *int   `json:"regularPriceCents"`
	Label             string `json:"label"`
	Source            string `json:"source"`
}

// ShoppingDealPutHandler handles PUT /api/shopping-list/deals.
type ShoppingDealPutHandler struct {
	ShoppingList interface {
		SaveDeal(ctx context.Context, userID string, deal shopping.Deal) error
	}
	Pantry interface {
		GetItem(ctx context.Context, itemID string) (*inventory.Item, error)
	}
}

func (h *ShoppingDealPutHandler) Handle(req Request[shoppingDealRequest, struct{}]) (*shoppingDealResponse, error) {
	if req.Body.ItemID == "" {
		return nil, &HTTPError{Code: 422, Message: "Choose a brand to mark on sale"}
	}
	label := strings.TrimSpace(req.Body.Label)
	if req.Body.PriceCents == nil && label == "" {
		return nil, &HTTPError{Code: 422, Message: "Add a sale price or a short note"}
	}
	if (req.Body.PriceCents != nil && *req.Body.PriceCents < 0) || (req.Body.RegularPriceCents != nil && *req.Body.RegularPriceCents < 0) {
		return nil, &HTTPError{Code: 422, Message: "Sale price can't be negative"}
	}
	item, err := h.Pantry.GetItem(req.Context, req.Body.ItemID)
	if err != nil {
		return nil, InternalError(err)
	}
	if item == nil {
		return nil, NotFound("item not found")
	}
	deal := shopping.Deal{
		ItemID:            item.ID,
		PriceCents:        req.Body.PriceCents,
		RegularPriceCents: req.Body.RegularPriceCents,
		Label:             label,
		Source:            shopping.DealSourceRecorded,
	}
	const userID = "user-1"
	if err := h.ShoppingList.SaveDeal(req.Context, userID, deal); err != nil {
		return nil, InternalError(err)
	}
	return &shoppingDealResponse{
		ItemID:            deal.ItemID,
		PriceCents:        deal.PriceCents,
		RegularPriceCents: deal.RegularPriceCents,
		Label:             deal.Label,
		Source:            deal.Source,
	}, nil
}

type shoppingDealPath struct {
	ItemID string `json:"itemId"`
}

// ShoppingDealDeleteHandler handles DELETE /api/shopping-list/deals/{itemId}.
type ShoppingDealDeleteHandler struct {
	ShoppingList interface {
		DeleteDeal(ctx context.Context, userID, itemID string) error
	}
}

func (h *ShoppingDealDeleteHandler) Handle(req Request[struct{}, shoppingDealPath]) (struct{}, error) {
	if req.PathParams.ItemID == "" {
		return struct{}{}, &HTTPError{Code: 422, Message: "Choose a brand to clear"}
	}
	const userID = "user-1"
	if err := h.ShoppingList.DeleteDeal(req.Context, userID, req.PathParams.ItemID); err != nil {
		return struct{}{}, InternalError(err)
	}
	return struct{}{}, nil
}
