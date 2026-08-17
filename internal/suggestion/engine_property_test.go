package suggestion

// Feature: pantry-management, Property 14: Suggestion returns data-insufficient for fewer than 3 consumption events

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// makeEvent constructs a ConsumptionEvent for the given item at the given time offset (seconds from epoch).
func makeEvent(itemID string, offsetSec int64) ConsumptionEvent {
	return ConsumptionEvent{
		ID:        "e",
		ItemID:    itemID,
		ConsumedAt: time.Unix(offsetSec, 0),
	}
}

// TestProperty14_DataInsufficientThreshold verifies the data-sufficiency boundary.
//
// For any item with 0, 1, or 2 consumption events: DataInsufficient must be true,
// SuggestedQuantity must be 0, and Reasoning must be empty.
//
// For any item with 3–10 consumption events: DataInsufficient must be false,
// SuggestedQuantity must be > 0, and Reasoning must be non-empty.
//
// Validates: Requirements 3.1, 3.2, 3.5
func TestProperty14_DataInsufficientThreshold(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		itemID := "item-prop14"

		// --- Sub-property A: fewer than 3 events → data insufficient ---
		insufficientCount := rapid.IntRange(0, 2).Draw(rt, "insufficientCount")
		insufficientEvents := make([]ConsumptionEvent, insufficientCount)
		for i := 0; i < insufficientCount; i++ {
			offset := rapid.Int64().Draw(rt, "offset")
			insufficientEvents[i] = makeEvent(itemID, offset)
		}

		resultInsufficient := SuggestTargetQuantity(itemID, insufficientEvents)

		if !resultInsufficient.DataInsufficient {
			rt.Fatalf("count=%d: expected DataInsufficient=true, got false", insufficientCount)
		}
		if resultInsufficient.SuggestedQuantity != 0 {
			rt.Fatalf("count=%d: expected SuggestedQuantity=0, got %d",
				insufficientCount, resultInsufficient.SuggestedQuantity)
		}
		if resultInsufficient.Reasoning != "" {
			rt.Fatalf("count=%d: expected empty Reasoning, got %q",
				insufficientCount, resultInsufficient.Reasoning)
		}
		if resultInsufficient.ItemID != itemID {
			rt.Fatalf("count=%d: expected ItemID=%q, got %q",
				insufficientCount, itemID, resultInsufficient.ItemID)
		}
		if resultInsufficient.ConsumptionEventCount != insufficientCount {
			rt.Fatalf("count=%d: expected ConsumptionEventCount=%d, got %d",
				insufficientCount, insufficientCount, resultInsufficient.ConsumptionEventCount)
		}

		// --- Sub-property B: 3 or more events → numeric suggestion with reasoning ---
		sufficientCount := rapid.IntRange(3, 10).Draw(rt, "sufficientCount")
		sufficientEvents := make([]ConsumptionEvent, sufficientCount)

		// Use a base time and spread events out so intervals are positive.
		baseOffsetSec := int64(1_700_000_000)
		for i := 0; i < sufficientCount; i++ {
			// Each event is 1–30 days after the previous to guarantee distinct, ordered times.
			dayOffset := rapid.Int64Range(1, 30).Draw(rt, "dayOffset")
			baseOffsetSec += dayOffset * 86400
			sufficientEvents[i] = makeEvent(itemID, baseOffsetSec)
		}

		resultSufficient := SuggestTargetQuantity(itemID, sufficientEvents)

		if resultSufficient.DataInsufficient {
			rt.Fatalf("count=%d: expected DataInsufficient=false, got true", sufficientCount)
		}
		if resultSufficient.SuggestedQuantity <= 0 {
			rt.Fatalf("count=%d: expected SuggestedQuantity>0, got %d",
				sufficientCount, resultSufficient.SuggestedQuantity)
		}
		if resultSufficient.Reasoning == "" {
			rt.Fatalf("count=%d: expected non-empty Reasoning, got empty string", sufficientCount)
		}
		if resultSufficient.ItemID != itemID {
			rt.Fatalf("count=%d: expected ItemID=%q, got %q",
				sufficientCount, itemID, resultSufficient.ItemID)
		}
		if resultSufficient.ConsumptionEventCount != sufficientCount {
			rt.Fatalf("count=%d: expected ConsumptionEventCount=%d, got %d",
				sufficientCount, sufficientCount, resultSufficient.ConsumptionEventCount)
		}
	})
}
