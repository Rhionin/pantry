package group

import "testing"

func TestMemberBrandFromRelishNames(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{name: "Mt. Olive Sweet Relish, 10 FL OZ", title: "Sweet relish", want: "Mt. Olive"},
		{name: "Mt. Olive Sweet Relish, 8 FL OZ", title: "Sweet relish", want: "Mt. Olive"},
		{name: "Great Value Sweet Relish, 10 fl oz", title: "Sweet relish", want: "Great Value"},
		{name: "Vlasic Sweet Relish, 10 FL OZ", title: "Sweet relish", want: "Vlasic"},
		{name: "Sweet Relish", title: "Sweet relish", want: ""},
		{name: "Organic Sweet Relish", title: "Sweet relish", want: ""},
		{name: "Del Monte Cut Green Beans", title: "Cut green beans", want: "Del Monte"},
		{name: "Member's Mark Cut Green Beans", title: "Cut green beans", want: "Member's Mark"},
		{name: "Kroger® Cut Green Beans", title: "Cut green beans", want: "Kroger"},
		{name: "Cut Green Beans, Western Family", title: "Cut green beans", want: "Western Family"},
		{name: "French Style Green Beans", title: "Green beans", want: ""},
		{name: "Milk", title: "Beans", want: ""},
		{name: "Good & Gather Cut Green Beans", title: "Cut green beans", want: "Good & Gather"},
	}
	for _, tt := range cases {
		if got := memberBrand(tt.name, tt.title); got != tt.want {
			t.Errorf("memberBrand(%q, %q) = %q, want %q", tt.name, tt.title, got, tt.want)
		}
	}
}

func TestMemberVarietyUsesKnownDifferentiators(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "Great Value Crunchy Peanut Butter", want: "Crunchy"},
		{name: "Organic Crunchy Peanut Butter", want: "Organic, Crunchy"},
		{name: "French Style Green Beans", want: "French style"},
		{name: "Mt. Olive Sweet Relish, 10 FL OZ", want: ""},
		{name: "Kroger No Salt Cut Green Beans", want: "No salt"},
	}
	for _, tt := range cases {
		if got := memberVariety(tt.name); got != tt.want {
			t.Errorf("memberVariety(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
