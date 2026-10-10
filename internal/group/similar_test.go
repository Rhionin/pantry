package group

import (
	"strings"
	"testing"
)

func TestSimilarMatchesFixture(t *testing.T) {
	tests := []struct {
		name    string
		in      []seedProduct
		want    [][]string
		blocked [][2]string
	}{
		{
			name: "store brands and a national brand share cut green beans",
			in: []seedProduct{
				{id: "gv", name: "Great Value Cut Green Beans", category: "Canned Vegetables"},
				{id: "kr", name: "Kroger Cut Green Beans", category: "Canned Vegetables"},
				{id: "dm", name: "Del Monte Cut Green Beans", category: "Canned Vegetables"},
			},
			want: [][]string{{"dm", "gv", "kr"}},
		},
		{
			name: "national brands match without a store brand anchor",
			in: []seedProduct{
				{id: "hunts", name: "Hunt's Cut Green Beans"},
				{id: "dm", name: "Del Monte Cut Green Beans"},
			},
			want: [][]string{{"dm", "hunts"}},
		},
		{
			name: "ketchup brands merge with the plain name",
			in: []seedProduct{
				{id: "hunts", name: "Hunt's Tomato Ketchup", category: "Condiments"},
				{id: "heinz", name: "Heinz Tomato Ketchup", category: "Condiments"},
				{id: "plain", name: "Ketchup", category: "Condiments"},
			},
			want: [][]string{{"heinz", "hunts", "plain"}},
		},
		{
			name: "peanut butter keeps variety cautions and still groups",
			in: []seedProduct{
				{id: "jif", name: "Jif Creamy Peanut Butter", category: "Peanut Butter"},
				{id: "skippy", name: "Skippy Super Chunk Peanut Butter", category: "Peanut Butter"},
				{id: "gv", name: "Great Value Peanut Butter", category: "Peanut Butter"},
			},
			want: [][]string{{"gv", "jif", "skippy"}},
		},
		{
			name: "one word foods group with both brands",
			in: []seedProduct{
				{id: "barilla", name: "Barilla Spaghetti", category: "Pasta"},
				{id: "dececco", name: "De Cecco Spaghetti", category: "Pasta"},
			},
			want: [][]string{{"barilla", "dececco"}},
		},
		{
			name: "sweet relish brands",
			in: []seedProduct{
				{id: "olive", name: "Mt. Olive Sweet Relish"},
				{id: "vlasic", name: "Vlasic Sweet Relish"},
			},
			want: [][]string{{"olive", "vlasic"}},
		},
		{
			name: "olive oil sizes and brands",
			in: []seedProduct{
				{id: "plain", name: "Olive Oil"},
				{id: "evoo", name: "Extra Virgin Olive Oil"},
				{id: "gv", name: "Great Value Olive Oil"},
			},
			want: [][]string{{"evoo", "gv", "plain"}},
		},
		{
			name: "milk fat levels share milk",
			in: []seedProduct{
				{id: "whole", name: "Whole Milk"},
				{id: "low", name: "2% Milk"},
			},
			want: [][]string{{"low", "whole"}},
		},
		{
			name: "diced tomato brands and the plural",
			in: []seedProduct{
				{id: "hunts", name: "Hunt's Diced Tomatoes"},
				{id: "dm", name: "Del Monte Diced Tomato"},
			},
			want: [][]string{{"dm", "hunts"}},
		},
		{
			name: "same category confirms a longer brand prefix",
			in: []seedProduct{
				{id: "happy", name: "Happy Farms Cut Green Beans", category: "Canned Vegetables"},
				{id: "sunny", name: "Sunny Fields Cut Green Beans", category: "Canned Vegetables"},
			},
			want: [][]string{{"happy", "sunny"}},
		},
		{
			name: "category does not glue different foods",
			in: []seedProduct{
				{id: "apple", name: "Apple Juice", category: "Juice"},
				{id: "orange", name: "Orange Juice", category: "Juice"},
			},
			blocked: [][2]string{{"apple", "orange"}},
		},
		{
			name: "beans of different colors stay apart",
			in: []seedProduct{
				{id: "black", name: "Black Beans"},
				{id: "green", name: "Green Beans"},
				{id: "kidney", name: "Kidney Beans"},
			},
			blocked: [][2]string{{"black", "green"}, {"black", "kidney"}, {"green", "kidney"}},
		},
		{
			name: "broth proteins stay apart",
			in: []seedProduct{
				{id: "chicken", name: "Chicken Broth"},
				{id: "beef", name: "Beef Broth"},
			},
			blocked: [][2]string{{"chicken", "beef"}},
		},
		{
			name: "oils stay apart",
			in: []seedProduct{
				{id: "olive", name: "Olive Oil"},
				{id: "coconut", name: "Coconut Oil"},
				{id: "canola", name: "Canola Oil"},
			},
			blocked: [][2]string{{"olive", "coconut"}, {"olive", "canola"}, {"coconut", "canola"}},
		},
		{
			name: "nut butters stay apart",
			in: []seedProduct{
				{id: "peanut", name: "Peanut Butter"},
				{id: "almond", name: "Almond Butter"},
			},
			blocked: [][2]string{{"peanut", "almond"}},
		},
		{
			name: "flake cereals stay apart",
			in: []seedProduct{
				{id: "frosted", name: "Frosted Flakes"},
				{id: "corn", name: "Corn Flakes"},
			},
			blocked: [][2]string{{"frosted", "corn"}},
		},
		{
			name: "cut green beans stay apart from french style",
			in: []seedProduct{
				{id: "cut", name: "Cut Green Beans"},
				{id: "french", name: "French Style Green Beans"},
				{id: "plain", name: "Green Beans"},
			},
			want:    [][]string{{"french", "plain"}},
			blocked: [][2]string{{"cut", "french"}, {"cut", "plain"}},
		},
		{
			name: "dairy milk stays apart from almond milk",
			in: []seedProduct{
				{id: "dairy", name: "Whole Milk"},
				{id: "almond", name: "Almond Milk"},
			},
			blocked: [][2]string{{"dairy", "almond"}},
		},
		{
			name: "one brand does not glue two foods",
			in: []seedProduct{
				{id: "beans", name: "Del Monte Cut Green Beans"},
				{id: "corn", name: "Del Monte Creamed Corn"},
			},
			blocked: [][2]string{{"beans", "corn"}},
		},
		{
			name: "cheerios do not absorb another cereal",
			in: []seedProduct{
				{id: "cheerios", name: "Cheerios"},
				{id: "flakes", name: "Frosted Flakes"},
			},
			blocked: [][2]string{{"cheerios", "flakes"}},
		},
		{
			name: "missing category does not confirm a weak brand prefix",
			in: []seedProduct{
				{id: "happy", name: "Happy Farms Cut Green Beans"},
				{id: "sunny", name: "Sunny Fields Cut Green Beans"},
			},
			blocked: [][2]string{{"happy", "sunny"}},
		},
		{
			name: "a different category does not confirm a weak brand prefix",
			in: []seedProduct{
				{id: "happy", name: "Happy Farms Cut Green Beans", category: "Canned Vegetables"},
				{id: "sunny", name: "Sunny Fields Cut Green Beans", category: "Frozen Vegetables"},
			},
			blocked: [][2]string{{"happy", "sunny"}},
		},
		{
			name: "a strong name still matches when categories disagree",
			in: []seedProduct{
				{id: "hunts", name: "Hunt's Tomato Ketchup", category: "Condiments"},
				{id: "heinz", name: "Heinz Tomato Ketchup", category: "Sauces"},
			},
			want: [][]string{{"heinz", "hunts"}},
		},
		{
			name: "spaghetti stays apart from a different pasta",
			in: []seedProduct{
				{id: "spaghetti", name: "Barilla Spaghetti"},
				{id: "penne", name: "Barilla Penne"},
			},
			blocked: [][2]string{{"spaghetti", "penne"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := clusterSets(test.in)
			if len(test.want) != len(got) {
				t.Fatalf("clusters %v, want %v", got, test.want)
			}
			seen := map[string]struct{}{}
			for _, set := range got {
				seen[strings.Join(set, ",")] = struct{}{}
			}
			for _, set := range test.want {
				if _, ok := seen[strings.Join(set, ",")]; !ok {
					t.Fatalf("clusters %v, missing %v", got, set)
				}
			}
			for _, pair := range test.blocked {
				if shareCluster(got, pair[0], pair[1]) {
					t.Fatalf("clusters %v put %s with %s", got, pair[0], pair[1])
				}
			}
		})
	}
}

func TestSimilarPeanutButterCautions(t *testing.T) {
	products := []seedProduct{
		{id: "jif", name: "Jif Creamy Peanut Butter"},
		{id: "skippy", name: "Skippy Super Chunk Peanut Butter"},
		{id: "gv", name: "Great Value Peanut Butter"},
	}
	matches := similarMatches(products, nil)
	byID := map[string]Match{}
	for i, match := range matches {
		byID[products[i].id] = match
	}
	if byID["jif"].Caution != "Creamy. Probably not the same." {
		t.Fatalf("jif caution %q", byID["jif"].Caution)
	}
	if byID["skippy"].Caution != "Super chunk. Probably not the same." {
		t.Fatalf("skippy caution %q", byID["skippy"].Caution)
	}
	if byID["gv"].Caution != "" {
		t.Fatalf("plain caution %q", byID["gv"].Caution)
	}
	if byID["jif"].Key == "" || byID["jif"].Key != byID["skippy"].Key || byID["jif"].Key != byID["gv"].Key {
		t.Fatalf("keys %#v", byID)
	}
}

func clusterSets(products []seedProduct) [][]string {
	matches := similarMatches(products, nil)
	cards := clustersFrom(products, matches, map[string]string{}, map[string]struct{}{}, func(key string) string { return key })
	out := make([][]string, 0, len(cards))
	for _, card := range cards {
		out = append(out, append([]string(nil), card.ids...))
	}
	return out
}

func shareCluster(sets [][]string, a, b string) bool {
	for _, set := range sets {
		hasA, hasB := false, false
		for _, id := range set {
			if id == a {
				hasA = true
			}
			if id == b {
				hasB = true
			}
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}
