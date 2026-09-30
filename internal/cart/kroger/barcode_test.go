package kroger

import (
	"testing"

	"github.com/Rhionin/pantry/internal/cart"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		barcode string
		want    cart.ProductIdentity
		wantOK  bool
	}{
		{
			name:    "UPC-A discards check digit and pads two zeros",
			barcode: "011110728227",
			want:    "0001111072822",
			wantOK:  true,
		},
		{
			name:    "UPC-A second valid code",
			barcode: "036000291452",
			want:    "0003600029145",
			wantOK:  true,
		},
		{
			name:    "UPC-A check digit mismatch",
			barcode: "011110728228",
			wantOK:  false,
		},
		{
			name:    "UPC-A with non-digit",
			barcode: "01111072822X",
			wantOK:  false,
		},
		{
			name:    "EAN-13 discards check digit and pads one zero",
			barcode: "4006381333931",
			want:    "0400638133393",
			wantOK:  true,
		},
		{
			name:    "EAN-13 second valid code",
			barcode: "9780143007234",
			want:    "0978014300723",
			wantOK:  true,
		},
		{
			name:    "EAN-13 check digit mismatch",
			barcode: "4006381333930",
			wantOK:  false,
		},
		{
			name:    "EAN-13 with non-digit",
			barcode: "400638133393X",
			wantOK:  false,
		},
		{
			name:    "UPC-E P6=0 expansion",
			barcode: "01230004",
			want:    "0001200000300",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=1 expansion",
			barcode: "01230013",
			want:    "0001210000300",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=2 expansion",
			barcode: "01230022",
			want:    "0001220000300",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=3 expansion",
			barcode: "01230030",
			want:    "0001230000000",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=4 expansion",
			barcode: "01230040",
			want:    "0001230000000",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=5 expansion",
			barcode: "01230055",
			want:    "0001230000005",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=6 expansion",
			barcode: "01230062",
			want:    "0001230000006",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=7 expansion",
			barcode: "01230079",
			want:    "0001230000007",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=8 expansion",
			barcode: "01230086",
			want:    "0001230000008",
			wantOK:  true,
		},
		{
			name:    "UPC-E P6=9 expansion",
			barcode: "01230093",
			want:    "0001230000009",
			wantOK:  true,
		},
		{
			name:    "UPC-E expanded check digit mismatch",
			barcode: "01230005",
			wantOK:  false,
		},
		{
			name:    "UPC-E not starting with zero",
			barcode: "11230004",
			wantOK:  false,
		},
		{
			name:    "UPC-E with non-digit",
			barcode: "0123000X",
			wantOK:  false,
		},
		{
			name:    "wrong length ten digits",
			barcode: "0123456789",
			wantOK:  false,
		},
		{
			name:    "empty string",
			barcode: "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Normalize(tt.barcode)
			if ok != tt.wantOK {
				t.Fatalf("Normalize(%q) ok = %v, want %v", tt.barcode, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.barcode, got, tt.want)
			}
		})
	}
}
