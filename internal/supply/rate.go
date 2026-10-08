package supply

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type gap struct {
	open   time.Time
	close  time.Time
	qty    float64
	perDay float64
}

// AmountWithdrawal is one consumption moment.
// Qty is an item count or a measured amount in one unit. Rows at the same time are summed.
type AmountWithdrawal struct {
	At  time.Time
	Qty float64
}

// DeriveAmount is the household rate in Qty's unit per day.
// ok is false until 30 days after opening, and unless there is a stock-in,
// at least two gaps, a 21-day span, and the burst check passes.
func DeriveAmount(now, started time.Time, ins []time.Time, outs []AmountWithdrawal) (perDay float64, ok bool) {
	if started.IsZero() || now.Before(started.Add(30*24*time.Hour)) {
		return 0, false
	}
	if len(ins) == 0 {
		return 0, false
	}
	firstIn := ins[0]
	var counted []AmountWithdrawal
	for _, o := range outs {
		if o.At.After(firstIn) && o.Qty > 0 {
			counted = append(counted, o)
		}
	}
	counted = mergeAmounts(counted)

	prev := firstIn
	var gaps []gap
	for _, o := range counted {
		if !o.At.After(prev) {
			continue
		}
		days := o.At.Sub(prev).Hours() / 24
		gaps = append(gaps, gap{
			open:   prev,
			close:  o.At,
			qty:    o.Qty,
			perDay: o.Qty / days,
		})
		prev = o.At
	}
	if len(gaps) > 6 {
		gaps = gaps[len(gaps)-6:]
	}
	if len(gaps) < 2 {
		return 0, false
	}
	span := gaps[len(gaps)-1].close.Sub(gaps[0].open)
	if span < 21*24*time.Hour {
		return 0, false
	}
	rates := make([]float64, len(gaps))
	for i, g := range gaps {
		rates[i] = g.perDay
	}
	sort.Float64s(rates)
	med := rates[(len(rates)-1)/2]
	fastest := rates[len(rates)-1]
	if len(rates) < 4 && fastest >= 4*med {
		return 0, false
	}
	return med, true
}

// derive is the item-count rate. plan is the only caller.
func derive(now, started time.Time, ins []time.Time, outs []Withdrawal) (perDay float64, ok bool) {
	measured := make([]AmountWithdrawal, len(outs))
	for i, o := range outs {
		measured[i] = AmountWithdrawal{At: o.At, Qty: float64(o.Qty)}
	}
	return DeriveAmount(now, started, ins, measured)
}

func mergeAmounts(outs []AmountWithdrawal) []AmountWithdrawal {
	if len(outs) == 0 {
		return nil
	}
	merged := make([]AmountWithdrawal, 0, len(outs))
	for _, o := range outs {
		if n := len(merged); n > 0 && merged[n-1].At.Equal(o.At) {
			merged[n-1].Qty += o.Qty
			continue
		}
		merged = append(merged, o)
	}
	return merged
}

func roundHalfUp(x float64) int {
	if x <= 0 {
		return 0
	}
	return int(math.Floor(x + 0.5))
}

func noteRate(perMonth, par int, window Months) string {
	return fmt.Sprintf("about %d a month, so %d for %d months.", perMonth, par, int(window))
}

func noteReplace(q Units) string {
	return fmt.Sprintf("replacing %d you used", int(q))
}
