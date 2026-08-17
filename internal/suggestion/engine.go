package suggestion

import (
	"fmt"
	"math"
	"sort"
)

const (
	// minEvents is the minimum number of consumption events required to produce
	// a data-sufficient suggestion (Requirement 3.5).
	minEvents = 3

	// restockHorizon is the number of days of supply we aim to maintain.
	restockHorizon = 14
)

// SuggestTargetQuantity computes a suggested target instance count for an item
// based on its recorded consumption events.
//
// When fewer than 3 events are present, the result carries DataInsufficient = true
// and no numeric suggestion is returned (Requirement 3.5). With 3 or more events,
// the algorithm:
//  1. Computes the inter-consumption intervals (days between consecutive events).
//  2. Takes the median interval as the estimated usage cadence.
//  3. Returns ceil(restockHorizon / medianInterval) + 1 as the suggested quantity.
//
// Validates: Requirements 3.1, 3.2, 3.5
func SuggestTargetQuantity(itemID string, events []ConsumptionEvent) TargetQuantitySuggestion {
	if len(events) < minEvents {
		return TargetQuantitySuggestion{
			ItemID:                itemID,
			ConsumptionEventCount: len(events),
			DataInsufficient:      true,
		}
	}

	// Sort events ascending by consumed_at so intervals are always positive.
	sorted := make([]ConsumptionEvent, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ConsumedAt.Before(sorted[j].ConsumedAt)
	})

	// Build inter-consumption intervals in days.
	intervals := make([]float64, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		diff := sorted[i].ConsumedAt.Sub(sorted[i-1].ConsumedAt)
		intervals[i-1] = diff.Hours() / 24
	}

	medianInterval := median(intervals)

	// Total span for reasoning text.
	totalDays := sorted[len(sorted)-1].ConsumedAt.Sub(sorted[0].ConsumedAt).Hours() / 24

	// Protect against divide-by-zero for events that all share the same timestamp.
	var rawSuggestion float64
	if medianInterval <= 0 {
		rawSuggestion = float64(restockHorizon)
	} else {
		rawSuggestion = math.Ceil(float64(restockHorizon) / medianInterval)
	}
	suggestion := int(rawSuggestion) + 1

	reasoning := fmt.Sprintf(
		"Based on %d consumption events over %.0f days, you use this item approximately every %.1f days. "+
			"To maintain a %d-day supply, we suggest a target of %d.",
		len(events), totalDays, medianInterval, restockHorizon, suggestion,
	)

	return TargetQuantitySuggestion{
		ItemID:                itemID,
		SuggestedQuantity:     suggestion,
		Reasoning:             reasoning,
		ConsumptionEventCount: len(events),
		DataInsufficient:      false,
	}
}

// median returns the median value of a sorted or unsorted float64 slice.
// It sorts the slice in place to avoid an extra allocation.
func median(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}

	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	mid := n / 2
	if n%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}
