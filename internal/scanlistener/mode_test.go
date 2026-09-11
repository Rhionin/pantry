package scanlistener

import (
	"testing"

	"github.com/Rhionin/pantry/internal/scan"
)

func TestNewModeState_DefaultsToStockIn(t *testing.T) {
	m := newModeState()
	if got := m.get(); got != scan.StockIn {
		t.Errorf("get(): want %v, got %v", scan.StockIn, got)
	}
}

func TestModeState_SetThenGet(t *testing.T) {
	tests := []struct {
		name string
		set  scan.ScanDirection
	}{
		{name: "set stock_in", set: scan.StockIn},
		{name: "set stock_out", set: scan.StockOut},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModeState()
			m.set(tt.set)

			// get() repeatedly returns the value just set without drifting
			// back to the default.
			for i := 0; i < 3; i++ {
				if got := m.get(); got != tt.set {
					t.Errorf("get() call %d: want %v, got %v", i, tt.set, got)
				}
			}
		})
	}
}

func TestClassify(t *testing.T) {
	const stockInBarcode = "STOCK_IN"
	const stockOutBarcode = "STOCK_OUT"

	tests := []struct {
		name        string
		barcode     string
		wantMode    scan.ScanDirection
		wantControl bool
	}{
		{
			name:        "exact match on stock-in barcode",
			barcode:     stockInBarcode,
			wantMode:    scan.StockIn,
			wantControl: true,
		},
		{
			name:        "exact match on stock-out barcode",
			barcode:     stockOutBarcode,
			wantMode:    scan.StockOut,
			wantControl: true,
		},
		{
			name:        "unrelated product barcode",
			barcode:     "0123456789012",
			wantControl: false,
		},
		{
			name:        "differs from stock-in barcode only by case",
			barcode:     "stock_in",
			wantControl: false,
		},
		{
			name:        "differs from stock-out barcode only by surrounding whitespace",
			barcode:     " STOCK_OUT ",
			wantControl: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMode, gotControl := classify(tt.barcode, stockInBarcode, stockOutBarcode)
			if gotControl != tt.wantControl {
				t.Errorf("classify(%q): isControl want %v, got %v", tt.barcode, tt.wantControl, gotControl)
			}
			if tt.wantControl && gotMode != tt.wantMode {
				t.Errorf("classify(%q): mode want %v, got %v", tt.barcode, tt.wantMode, gotMode)
			}
		})
	}
}
