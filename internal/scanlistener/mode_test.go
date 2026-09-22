package scanlistener

import (
	"testing"

	"github.com/Rhionin/pantry/internal/scan"
)

func TestModeState(t *testing.T) {
	state := newModeState()

	// Default should be StockIn
	if got := state.get(); got != scan.StockIn {
		t.Errorf("newModeState().get() = %v, want %v", got, scan.StockIn)
	}

	// set followed by get should return the value just set
	state.set(scan.StockOut)
	if got := state.get(); got != scan.StockOut {
		t.Errorf("after set(StockOut), get() = %v, want %v", got, scan.StockOut)
	}

	state.set(scan.StockIn)
	if got := state.get(); got != scan.StockIn {
		t.Errorf("after set(StockIn), get() = %v, want %v", got, scan.StockIn)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		barcode    string
		stockIn    string
		stockOut   string
		wantMode   scan.ScanDirection
		wantIsCtrl bool
	}{
		{"stock-in match", "STOCK_IN", "STOCK_IN", "STOCK_OUT", scan.StockIn, true},
		{"stock-out match", "STOCK_OUT", "STOCK_IN", "STOCK_OUT", scan.StockOut, true},
		{"no match", "123456", "STOCK_IN", "STOCK_OUT", "", false},
		{"case sensitive", "stock_in", "STOCK_IN", "STOCK_OUT", "", false},
		{"whitespace sensitive", " STOCK_IN", "STOCK_IN", "STOCK_OUT", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, isCtrl := classify(tt.barcode, tt.stockIn, tt.stockOut)
			if mode != tt.wantMode {
				t.Errorf("classify(%q, %q, %q) mode = %v, want %v", tt.barcode, tt.stockIn, tt.stockOut, mode, tt.wantMode)
			}
			if isCtrl != tt.wantIsCtrl {
				t.Errorf("classify(%q, %q, %q) isCtrl = %v, want %v", tt.barcode, tt.stockIn, tt.stockOut, isCtrl, tt.wantIsCtrl)
			}
		})
	}
}
