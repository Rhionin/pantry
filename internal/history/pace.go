package history

import (
	"math"
	"time"
)

const (
	paceWindow = 30 * 24 * time.Hour
	// Two uses inside a few days are a coincidence, not a pace.
	minUses = 2
	minSpan = 7 * 24 * time.Hour
	// A pace within 15% of the previous 30 days is steady.
	trendBand = 0.15
)

// Pace is the household's recent rate for one product or one group.
// DaysBetweenUses is the last 30 days (or the time since the first use in
// that window, when that is shorter) divided by how many units were used.
// That is the "about 1 every N days" figure.
type Pace struct {
	Known           bool    `json:"known"`
	DaysBetweenUses float64 `json:"daysBetweenUses,omitempty"`
	DaysLeft        int     `json:"daysLeft,omitempty"`
	DaysLeftKnown   bool    `json:"daysLeftKnown"`
	Trend           string  `json:"trend,omitempty"`
	Sparkline       []int   `json:"sparkline"`
}

// PaceFromUses builds the pace from one timestamp per unit used.
func PaceFromUses(now time.Time, onHand int, uses []time.Time) Pace {
	now = now.UTC()
	recentStart := now.Add(-paceWindow)
	priorStart := now.Add(-2 * paceWindow)

	var recent, prior []time.Time
	for _, used := range uses {
		at := used.UTC()
		if !at.After(now) && !at.Before(recentStart) {
			recent = append(recent, at)
			continue
		}
		if at.Before(recentStart) && !at.Before(priorStart) {
			prior = append(prior, at)
		}
	}

	pace := Pace{Sparkline: sparkline(now, recent)}
	if len(recent) < minUses {
		return pace
	}
	oldest := recent[0]
	for _, at := range recent[1:] {
		if at.Before(oldest) {
			oldest = at
		}
	}
	span := now.Sub(oldest)
	if span < minSpan {
		return pace
	}
	days := span.Hours() / 24
	pace.Known = true
	pace.DaysBetweenUses = roundTenth(days / float64(len(recent)))
	if pace.DaysBetweenUses <= 0 {
		pace.Known = false
		pace.DaysBetweenUses = 0
		return pace
	}
	pace.DaysLeftKnown = true
	pace.DaysLeft = int(math.Round(float64(onHand) * pace.DaysBetweenUses))
	pace.Trend = compareTrend(float64(len(recent))/days, prior)
	return pace
}

func compareTrend(recentPerDay float64, prior []time.Time) string {
	if len(prior) < minUses || recentPerDay <= 0 {
		return ""
	}
	priorPerDay := float64(len(prior)) / (paceWindow.Hours() / 24)
	if priorPerDay <= 0 {
		return ""
	}
	ratio := recentPerDay / priorPerDay
	switch {
	case ratio > 1+trendBand:
		return "faster"
	case ratio < 1-trendBand:
		return "slower"
	default:
		return "steady"
	}
}

func sparkline(now time.Time, recent []time.Time) []int {
	if len(recent) == 0 {
		return []int{}
	}
	end := now.UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -29)
	buckets := make([]int, 30)
	for _, at := range recent {
		idx := int(at.UTC().Truncate(24*time.Hour).Sub(start).Hours() / 24)
		if idx >= 0 && idx < len(buckets) {
			buckets[idx]++
		}
	}
	return buckets
}

func roundTenth(v float64) float64 {
	return math.Round(v*10) / 10
}
