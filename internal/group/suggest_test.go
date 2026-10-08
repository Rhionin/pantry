package group

import (
	"context"
	"testing"
)

func TestRefreshSuggestionsClustersLookAlikesOnce(t *testing.T) {
	groups, catalog, _ := newTestGroups(t)
	ctx := context.Background()
	mustProduct(t, catalog, "gv", "Great Value Cut Green Beans")
	mustProduct(t, catalog, "kr", "Kroger Cut Green Beans")
	mustProduct(t, catalog, "dm", "Del Monte Cut Green Beans")
	mustProduct(t, catalog, "fr", "French Style Green Beans")
	mustProduct(t, catalog, "cr", "Great Value Crunchy Peanut Butter")
	mustProduct(t, catalog, "sm", "Kroger Creamy Peanut Butter")
	mustProduct(t, catalog, "gvpb", "Great Value Peanut Butter")

	first, err := groups.ListSuggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("cards: %+v", first)
	}
	beans := cardByTitle(t, first, "Cut green beans")
	if beans.Kind != "looks_alike" || len(beans.Members) != 3 {
		t.Fatalf("%+v", beans)
	}
	for _, member := range beans.Members {
		if member.ProductID == "fr" || !member.Included || member.Name == "" {
			t.Fatalf("%+v", beans.Members)
		}
	}
	butter := cardByTitle(t, first, "Peanut butter")
	var crunchyIncluded bool
	var sawCrunchy bool
	for _, member := range butter.Members {
		if member.ProductID == "cr" {
			sawCrunchy = true
			crunchyIncluded = member.Included
			if member.Caution != "Crunchy. Probably not the same." {
				t.Fatalf("caution %q", member.Caution)
			}
		}
	}
	if !sawCrunchy || crunchyIncluded {
		t.Fatalf("%+v", butter.Members)
	}

	second, err := groups.ListSuggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || !sameIDs(first, second) {
		t.Fatalf("duplicated: %+v", second)
	}

	if err := groups.Dismiss(ctx, beans.ID); err != nil {
		t.Fatal(err)
	}
	after, err := groups.ListSuggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].Title == "Cut green beans" {
		t.Fatalf("dismissed card returned: %+v", after)
	}
}

func TestRefreshSuggestionsJoinsAnExistingGroup(t *testing.T) {
	groups, catalog, _ := newTestGroups(t)
	ctx := context.Background()
	mustProduct(t, catalog, "gv", "Great Value Cut Green Beans")
	mustProduct(t, catalog, "kr", "Kroger Cut Green Beans")
	view, err := groups.Create(ctx, "Cut green beans", []string{"gv", "kr"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mustProduct(t, catalog, "dm", "Del Monte Cut Green Beans")

	cards, err := groups.ListSuggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].ExistingGroupID != view.ID {
		t.Fatalf("%+v", cards)
	}
	if len(cards[0].Members) != 3 {
		t.Fatalf("%+v", cards[0].Members)
	}

	if _, err := groups.AddMembers(ctx, view.ID, []string{"dm"}, "", nil); err != nil {
		t.Fatal(err)
	}
	left, err := groups.ListSuggestions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].ID != cards[0].ID {
		t.Fatalf("open card changed after the product joined: %+v", left)
	}
}

func sameIDs(a, b []Suggestion) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]struct{}{}
	for _, card := range a {
		seen[card.ID] = struct{}{}
	}
	for _, card := range b {
		if _, ok := seen[card.ID]; !ok {
			return false
		}
	}
	return true
}

func cardByTitle(t *testing.T, cards []Suggestion, title string) Suggestion {
	t.Helper()
	for _, card := range cards {
		if card.Title == title {
			return card
		}
	}
	t.Fatalf("missing %s in %+v", title, cards)
	return Suggestion{}
}
