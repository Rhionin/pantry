package group

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/shopping"
)

// Kind is one shopping rule.
type Kind string

const (
	KindSameAsRanOut Kind = "same_as_ran_out"
	KindFavorite     Kind = "favorite"
	KindBestDeal     Kind = "best_deal"
)

// Member is one product the rule can pick.
type Member struct {
	ProductID      string    `json:"productId"`
	Name           string    `json:"name"`
	OnHand         int       `json:"onHand"`
	ItemID         string    `json:"-"`
	NetBase        *float64  `json:"-"`
	Dimension      string    `json:"-"`
	LastConsumedAt time.Time `json:"-"`
	LastStockedAt  time.Time `json:"-"`
}

// Input is everything a picker needs. Rules do not read the database.
type Input struct {
	Members         []Member
	PinnedProductID string
	Deals           []shopping.Deal
}

// Result is the product to buy and the sentence that says why.
type Result struct {
	ProductID       string `json:"productId"`
	Because         string `json:"because"`
	ComparedPerItem bool   `json:"comparedPerItem"`
}

// Picker chooses one member for a rule.
type Picker interface {
	Kind() Kind
	Pick(Input) Result
}

// Valid reports whether rule is one of the three kinds.
func Valid(rule string) bool {
	switch Kind(rule) {
	case KindSameAsRanOut, KindFavorite, KindBestDeal:
		return true
	default:
		return false
	}
}

var registry = map[Kind]Picker{}

func init() {
	register(sameAsRanOut{})
	register(favoritePicker{})
	register(bestDealPicker{})
}

func register(p Picker) {
	registry[p.Kind()] = p
}

// Pick runs the picker for kind.
// An unknown kind is a programming error.
func Pick(kind Kind, in Input) (Result, error) {
	p, ok := registry[kind]
	if !ok {
		return Result{}, fmt.Errorf("unknown group rule %q", kind)
	}
	return p.Pick(in), nil
}

type sameAsRanOut struct{}

func (sameAsRanOut) Kind() Kind { return KindSameAsRanOut }

func (sameAsRanOut) Pick(in Input) Result { return pickSame(in) }

type favoritePicker struct{}

func (favoritePicker) Kind() Kind { return KindFavorite }

func (favoritePicker) Pick(in Input) Result {
	if len(in.Members) == 0 {
		return Result{}
	}
	if m, ok := memberByID(in.Members, in.PinnedProductID); ok && in.PinnedProductID != "" {
		return Result{ProductID: m.ProductID, Because: "This is the one with the star."}
	}
	r := pickSame(in)
	r.Because = "No favorite is set. " + r.Because
	return r
}

type bestDealPicker struct{}

func (bestDealPicker) Kind() Kind { return KindBestDeal }

func (bestDealPicker) Pick(in Input) Result {
	if len(in.Members) == 0 {
		return Result{}
	}
	sales := onSale(in)
	if len(sales) == 0 {
		if m, ok := memberByID(in.Members, in.PinnedProductID); ok && in.PinnedProductID != "" {
			return Result{ProductID: m.ProductID, Because: "Nothing is on sale. Otherwise buy " + m.Name + "."}
		}
		r := pickSame(in)
		r.Because = "Nothing is on sale and no fallback is set. " + r.Because
		return r
	}
	perItem := !sharedOunceDimension(sales)
	best := sales[0]
	best.score = scoreOf(best.member, best.deal, perItem)
	for _, s := range sales[1:] {
		s.score = scoreOf(s.member, s.deal, perItem)
		if betterScore(s, best) {
			best = s
		}
	}
	return Result{
		ProductID:       best.member.ProductID,
		Because:         saleBecause(best.deal),
		ComparedPerItem: perItem,
	}
}

func pickSame(in Input) Result {
	if len(in.Members) == 0 {
		return Result{}
	}
	var consumed []Member
	for _, m := range in.Members {
		if !m.LastConsumedAt.IsZero() {
			consumed = append(consumed, m)
		}
	}
	if len(consumed) > 0 {
		best := consumed[0]
		for _, m := range consumed[1:] {
			if laterMember(m.LastConsumedAt, m, best.LastConsumedAt, best) {
				best = m
			}
		}
		return Result{ProductID: best.ProductID, Because: "The last one used up was " + best.Name + "."}
	}
	var stocked []Member
	for _, m := range in.Members {
		if !m.LastStockedAt.IsZero() {
			stocked = append(stocked, m)
		}
	}
	if len(stocked) > 0 {
		best := stocked[0]
		for _, m := range stocked[1:] {
			if laterMember(m.LastStockedAt, m, best.LastStockedAt, best) {
				best = m
			}
		}
		return Result{ProductID: best.ProductID, Because: "Nothing has run out yet. This is the one you stocked last."}
	}
	best := in.Members[0]
	for _, m := range in.Members[1:] {
		if m.ProductID < best.ProductID {
			best = m
		}
	}
	return Result{ProductID: best.ProductID, Because: "Nothing has run out yet."}
}

func laterMember(at time.Time, m Member, bt time.Time, b Member) bool {
	if at.After(bt) {
		return true
	}
	if bt.After(at) {
		return false
	}
	if m.Name != b.Name {
		return m.Name < b.Name
	}
	return m.ProductID < b.ProductID
}

func memberByID(members []Member, id string) (Member, bool) {
	if id == "" {
		return Member{}, false
	}
	for _, m := range members {
		if m.ProductID == id {
			return m, true
		}
	}
	return Member{}, false
}

type scored struct {
	member Member
	deal   shopping.Deal
	score  float64
}

func onSale(in Input) []scored {
	var sales []scored
	for _, m := range in.Members {
		d, ok := findDeal(m, in.Deals)
		if !ok || !d.OnSale() {
			continue
		}
		sales = append(sales, scored{member: m, deal: d})
	}
	return sales
}

func findDeal(m Member, deals []shopping.Deal) (shopping.Deal, bool) {
	for _, d := range deals {
		if d.ItemID == "" {
			continue
		}
		if d.ItemID == m.ItemID || d.ItemID == m.ProductID {
			return d, true
		}
	}
	return shopping.Deal{}, false
}

func sharedOunceDimension(sales []scored) bool {
	if len(sales) == 0 {
		return false
	}
	dim := ""
	for _, s := range sales {
		if s.deal.PriceCents == nil || s.member.NetBase == nil {
			return false
		}
		if s.member.Dimension != product.DimensionMass && s.member.Dimension != product.DimensionVolume {
			return false
		}
		amount, _, ok := product.DisplayNetSize(*s.member.NetBase, s.member.Dimension)
		if !ok || amount <= 0 {
			return false
		}
		if dim == "" {
			dim = s.member.Dimension
			continue
		}
		if dim != s.member.Dimension {
			return false
		}
	}
	return true
}

func scoreOf(m Member, d shopping.Deal, perItem bool) float64 {
	if d.PriceCents == nil {
		return math.Inf(1)
	}
	price := float64(*d.PriceCents)
	if perItem {
		return price
	}
	amount, _, ok := product.DisplayNetSize(*m.NetBase, m.Dimension)
	if !ok || amount <= 0 {
		return price
	}
	return price / amount
}

func betterScore(a, b scored) bool {
	if a.score != b.score {
		return a.score < b.score
	}
	if a.member.Name != b.member.Name {
		return a.member.Name < b.member.Name
	}
	return a.member.ProductID < b.member.ProductID
}

func saleBecause(d shopping.Deal) string {
	var b strings.Builder
	if d.PriceCents == nil {
		b.WriteString("On sale")
	} else {
		fmt.Fprintf(&b, "On sale for %s", money(*d.PriceCents))
		if d.RegularPriceCents != nil {
			fmt.Fprintf(&b, ", usually %s", money(*d.RegularPriceCents))
		}
	}
	b.WriteByte('.')
	if !d.NotedAt.IsZero() {
		fmt.Fprintf(&b, " You noted this sale on %s.", d.NotedAt.Format("Jan 2"))
	}
	return b.String()
}

func money(cents int) string {
	if cents < 0 {
		cents = 0
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}
