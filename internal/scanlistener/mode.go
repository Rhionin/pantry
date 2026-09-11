package scanlistener

import (
	"sync"

	"github.com/Rhionin/pantry/internal/scan"
)

// modeState holds Current_Mode. A mutex guards it because ScanListener.Run is
// the only writer but the same value could be inspected by future health
// endpoints; the mutex is cheap and removes any doubt.
type modeState struct {
	mu      sync.Mutex
	current scan.ScanDirection
}

// newModeState creates a modeState defaulting to stock_in, matching the rule
// that no Control_Barcode has been scanned since the server started.
func newModeState() *modeState {
	return &modeState{current: scan.StockIn}
}

func (m *modeState) get() scan.ScanDirection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *modeState) set(d scan.ScanDirection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = d
}

// classify reports whether barcode is one of the two configured control
// barcodes and, if so, which mode it selects. An exact string match only —
// no trimming or case-folding beyond what assemble() already produced.
func classify(barcode, stockInBarcode, stockOutBarcode string) (mode scan.ScanDirection, isControl bool) {
	switch barcode {
	case stockInBarcode:
		return scan.StockIn, true
	case stockOutBarcode:
		return scan.StockOut, true
	default:
		return "", false
	}
}
