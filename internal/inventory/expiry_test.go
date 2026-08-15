package inventory

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestComputeExpiryStatus(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	exp := func(d time.Duration) *time.Time { t := now.Add(d); return &t }

	tests := []struct {
		name        string
		expiresAt   *time.Time
		warningDays int
		want        ExpiryStatus
	}{
		{"nil expiry is always ok", nil, 7, ExpiryStatusOK},
		{"expired 5 days ago", exp(-5 * 24 * time.Hour), 7, ExpiryStatusExpired},
		{"expired 1 second ago", exp(-time.Second), 7, ExpiryStatusExpired},
		{"expires in 1 hour (near expiry)", exp(time.Hour), 7, ExpiryStatusNearExpiry},
		{"expires in 1 day (near expiry)", exp(24 * time.Hour), 7, ExpiryStatusNearExpiry},
		{"expires in 3 days (near expiry)", exp(3 * 24 * time.Hour), 7, ExpiryStatusNearExpiry},
		{"expires at exactly warning boundary", exp(7 * 24 * time.Hour), 7, ExpiryStatusNearExpiry},
		{"expires 1 second beyond warning boundary", exp(7*24*time.Hour + time.Second), 7, ExpiryStatusOK},
		{"expires 15 days out (ok)", exp(15 * 24 * time.Hour), 7, ExpiryStatusOK},
		// warningDays variation: expiresAt is 5 days from now
		{"5-day expiry, 3-day warning: ok", exp(5 * 24 * time.Hour), 3, ExpiryStatusOK},
		{"5-day expiry, 5-day warning: near expiry", exp(5 * 24 * time.Hour), 5, ExpiryStatusNearExpiry},
		{"5-day expiry, 7-day warning: near expiry", exp(5 * 24 * time.Hour), 7, ExpiryStatusNearExpiry},
		{"5-day expiry, 10-day warning: near expiry", exp(5 * 24 * time.Hour), 10, ExpiryStatusNearExpiry},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeExpiryStatus(tt.expiresAt, now, tt.warningDays)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Feature: pantry-management, Property 9: Expiry status is consistent with dates and warning period
//
// Validates: Requirements 2.8, 2.9
//
// For any item instance with an expiration date and any current time, the computed expiry status
// SHALL satisfy: expired when expiresAt < now; near_expiry when 0 ≤ (expiresAt − now) ≤ warningPeriod;
// and ok otherwise. Instances with no expiration date SHALL always have status ok.
func TestProperty_ExpiryStatusConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nowUnix := rapid.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC).Unix(),
		).Draw(t, "nowUnix")
		now := time.Unix(nowUnix, 0).UTC()

		warningDays := rapid.IntRange(1, 30).Draw(t, "warningDays")

		if ComputeExpiryStatus(nil, now, warningDays) != ExpiryStatusOK {
			t.Fatalf("nil expiration should always return ok")
		}

		expiresAtUnix := rapid.Int64Range(
			time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
			time.Date(2030, 12, 31, 23, 59, 59, 0, time.UTC).Unix(),
		).Draw(t, "expiresAtUnix")
		expiresAt := time.Unix(expiresAtUnix, 0).UTC()

		got := ComputeExpiryStatus(&expiresAt, now, warningDays)

		daysUntil := expiresAt.Sub(now).Hours() / 24
		var want ExpiryStatus
		switch {
		case daysUntil < 0:
			want = ExpiryStatusExpired
		case daysUntil <= float64(warningDays):
			want = ExpiryStatusNearExpiry
		default:
			want = ExpiryStatusOK
		}

		if got != want {
			t.Fatalf("now=%v expiresAt=%v warningDays=%d daysUntil=%.2f: got %q, want %q",
				now.Format(time.RFC3339), expiresAt.Format(time.RFC3339),
				warningDays, daysUntil, got, want)
		}
	})
}
