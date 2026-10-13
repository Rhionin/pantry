package history

import (
	"database/sql"
	"testing"
)

func TestProductDetail(t *testing.T) {
	cans := productDetail("cans", sql.NullFloat64{Float64: 15 * 28.349523125, Valid: true}, "mass", sql.NullInt64{})
	if cans != "15 oz cans" {
		t.Fatalf("detail = %q", cans)
	}
	plain := productDetail("unit", sql.NullFloat64{}, "", sql.NullInt64{})
	if plain != "" {
		t.Fatalf("empty detail = %q", plain)
	}
	pack := productDetail("cans", sql.NullFloat64{Float64: 15 * 28.349523125, Valid: true}, "mass", sql.NullInt64{Int64: 6, Valid: true})
	if pack != "6 x 15 oz cans" {
		t.Fatalf("pack detail = %q", pack)
	}
	mixed := sameDetail([]string{"15 oz cans", "12 oz cans"})
	if mixed != "2 products" {
		t.Fatalf("mixed = %q", mixed)
	}
	shared := sameDetail([]string{"15 oz cans", "15 oz cans"})
	if shared != "15 oz cans" {
		t.Fatalf("shared = %q", shared)
	}
}
