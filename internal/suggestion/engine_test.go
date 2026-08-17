package suggestion

import (
	"strings"
	"testing"
	"time"
)

// baseTime is an arbitrary anchor used to build event lists in tests.
var baseTime = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

// eventsWithIntervals builds a ConsumptionEvent slice where each event is
// spaced by the corresponding interval (in days) from the previous one.
func eventsWithIntervals(itemID string, intervals []float64) []ConsumptionEvent {
	events := make([]ConsumptionEvent, len(intervals)+1)
	events[0] = ConsumptionEvent{ID: "e0", ItemID: itemID, ConsumedAt: baseTime}
	for i, d := range intervals {
		events[i+1] = ConsumptionEvent{
			ID:        "e" + string(rune('1'+i)),
			ItemID:    itemID,
			ConsumedAt: events[i].ConsumedAt.Add(time.Duration(d*24) * time.Hour),
		}
	}
	return events
}

// TestSuggestTargetQuantity_DataInsufficient verifies that 0, 1, and 2 events
// all return DataInsufficient=true with no suggested quantity.
func TestSuggestTargetQuantity_DataInsufficient(t *testing.T) {
	itemID := "item-1"

	for _, count := range []int{0, 1, 2} {
		events := make([]ConsumptionEvent, count)
		for i := range events {
			events[i] = ConsumptionEvent{ID: "e", ItemID: itemID, ConsumedAt: baseTime.Add(time.Duration(i) * 24 * time.Hour)}
		}

		result := SuggestTargetQuantity(itemID, events)

		if !result.DataInsufficient {
			t.Errorf("count=%d: expected DataInsufficient=true, got false", count)
		}
		if result.SuggestedQuantity != 0 {
			t.Errorf("count=%d: expected SuggestedQuantity=0, got %d", count, result.SuggestedQuantity)
		}
		if result.Reasoning != "" {
			t.Errorf("count=%d: expected empty Reasoning, got %q", count, result.Reasoning)
		}
		if result.ConsumptionEventCount != count {
			t.Errorf("count=%d: expected ConsumptionEventCount=%d, got %d", count, count, result.ConsumptionEventCount)
		}
		if result.ItemID != itemID {
			t.Errorf("count=%d: expected ItemID=%q, got %q", count, itemID, result.ItemID)
		}
	}
}

// TestSuggestTargetQuantity_ExactlyThreeEvents verifies the boundary at exactly 3 events
// produces a valid numeric suggestion.
func TestSuggestTargetQuantity_ExactlyThreeEvents(t *testing.T) {
	itemID := "item-2"
	// Two intervals of 7 days each → median = 7 days
	// ceil(14 / 7) + 1 = 2 + 1 = 3
	events := eventsWithIntervals(itemID, []float64{7, 7})

	result := SuggestTargetQuantity(itemID, events)

	if result.DataInsufficient {
		t.Fatal("expected DataInsufficient=false")
	}
	if result.SuggestedQuantity != 3 {
		t.Errorf("expected SuggestedQuantity=3, got %d", result.SuggestedQuantity)
	}
	if result.ConsumptionEventCount != 3 {
		t.Errorf("expected ConsumptionEventCount=3, got %d", result.ConsumptionEventCount)
	}
	if result.Reasoning == "" {
		t.Error("expected non-empty Reasoning")
	}
	if !strings.Contains(result.Reasoning, "3 consumption events") {
		t.Errorf("expected Reasoning to mention event count, got %q", result.Reasoning)
	}
}

// TestSuggestTargetQuantity_FrequentUsage verifies items consumed every 2 days
// produce a high suggestion to cover the 14-day horizon.
func TestSuggestTargetQuantity_FrequentUsage(t *testing.T) {
	itemID := "item-3"
	// 4 events, intervals of 2 days each → median = 2 days
	// ceil(14 / 2) + 1 = 7 + 1 = 8
	events := eventsWithIntervals(itemID, []float64{2, 2, 2})

	result := SuggestTargetQuantity(itemID, events)

	if result.DataInsufficient {
		t.Fatal("expected DataInsufficient=false")
	}
	if result.SuggestedQuantity != 8 {
		t.Errorf("expected SuggestedQuantity=8, got %d", result.SuggestedQuantity)
	}
}

// TestSuggestTargetQuantity_InfrequentUsage verifies items consumed rarely
// produce a low but still positive suggestion.
func TestSuggestTargetQuantity_InfrequentUsage(t *testing.T) {
	itemID := "item-4"
	// 4 events, intervals of 30 days each → median = 30 days
	// ceil(14 / 30) + 1 = 1 + 1 = 2
	events := eventsWithIntervals(itemID, []float64{30, 30, 30})

	result := SuggestTargetQuantity(itemID, events)

	if result.DataInsufficient {
		t.Fatal("expected DataInsufficient=false")
	}
	if result.SuggestedQuantity != 2 {
		t.Errorf("expected SuggestedQuantity=2, got %d", result.SuggestedQuantity)
	}
}

// TestSuggestTargetQuantity_UnevenIntervals verifies the median is taken, not the mean.
// Intervals: [1, 1, 1, 100] → sorted [1, 1, 1, 100] → median = (1+1)/2 = 1 day
// ceil(14 / 1) + 1 = 15
func TestSuggestTargetQuantity_UnevenIntervals(t *testing.T) {
	itemID := "item-5"
	events := eventsWithIntervals(itemID, []float64{1, 1, 1, 100})

	result := SuggestTargetQuantity(itemID, events)

	if result.DataInsufficient {
		t.Fatal("expected DataInsufficient=false")
	}
	// median of [1,1,1,100] = (1+1)/2 = 1 → ceil(14/1)+1 = 15
	if result.SuggestedQuantity != 15 {
		t.Errorf("expected SuggestedQuantity=15, got %d", result.SuggestedQuantity)
	}
}

// TestSuggestTargetQuantity_EventsOutOfOrder verifies that unsorted input
// produces the same result as sorted input.
func TestSuggestTargetQuantity_EventsOutOfOrder(t *testing.T) {
	itemID := "item-6"
	t1 := baseTime
	t2 := baseTime.Add(7 * 24 * time.Hour)
	t3 := baseTime.Add(14 * 24 * time.Hour)

	// Reversed order
	events := []ConsumptionEvent{
		{ID: "e3", ItemID: itemID, ConsumedAt: t3},
		{ID: "e1", ItemID: itemID, ConsumedAt: t1},
		{ID: "e2", ItemID: itemID, ConsumedAt: t2},
	}

	result := SuggestTargetQuantity(itemID, events)

	if result.DataInsufficient {
		t.Fatal("expected DataInsufficient=false")
	}
	// Sorted intervals: [7, 7] → median 7 → ceil(14/7)+1 = 3
	if result.SuggestedQuantity != 3 {
		t.Errorf("expected SuggestedQuantity=3, got %d", result.SuggestedQuantity)
	}
}

// TestSuggestTargetQuantity_ReasoningContent verifies the reasoning string
// includes key information expected by Requirement 3.2.
func TestSuggestTargetQuantity_ReasoningContent(t *testing.T) {
	itemID := "item-7"
	events := eventsWithIntervals(itemID, []float64{7, 7, 7, 7}) // 5 events

	result := SuggestTargetQuantity(itemID, events)

	if result.DataInsufficient {
		t.Fatal("expected DataInsufficient=false")
	}
	if result.Reasoning == "" {
		t.Fatal("expected non-empty Reasoning")
	}
	if !strings.Contains(result.Reasoning, "5 consumption events") {
		t.Errorf("reasoning should mention event count, got: %q", result.Reasoning)
	}
	if !strings.Contains(result.Reasoning, "14-day supply") {
		t.Errorf("reasoning should mention restock horizon, got: %q", result.Reasoning)
	}
}

// TestSuggestTargetQuantity_ItemIDPropagated verifies the ItemID is echoed back.
func TestSuggestTargetQuantity_ItemIDPropagated(t *testing.T) {
	itemID := "item-abc-123"
	events := eventsWithIntervals(itemID, []float64{7, 7})

	result := SuggestTargetQuantity(itemID, events)

	if result.ItemID != itemID {
		t.Errorf("expected ItemID=%q, got %q", itemID, result.ItemID)
	}
}
