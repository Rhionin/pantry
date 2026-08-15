package inventory

import "time"

// ExpiryStatus represents the expiration state of an item instance.
type ExpiryStatus string

const (
	ExpiryStatusOK         ExpiryStatus = "ok"
	ExpiryStatusNearExpiry ExpiryStatus = "near_expiry"
	ExpiryStatusExpired    ExpiryStatus = "expired"
)

// ComputeExpiryStatus calculates the expiration status of an item instance
// based on its expiration date, the current time, and the warning period.
//
// Returns:
//   - ExpiryStatusExpired if expiresAt is before now
//   - ExpiryStatusNearExpiry if expiresAt is within warningDays of now
//   - ExpiryStatusOK if expiresAt is nil or more than warningDays away
//
// Validates Requirements 2.8, 2.9
func ComputeExpiryStatus(expiresAt *time.Time, now time.Time, warningDays int) ExpiryStatus {
	// No expiration date means always OK
	if expiresAt == nil {
		return ExpiryStatusOK
	}

	// Calculate days until expiry
	daysUntilExpiry := expiresAt.Sub(now).Hours() / 24

	// Already expired
	if daysUntilExpiry < 0 {
		return ExpiryStatusExpired
	}

	// Near expiry
	if daysUntilExpiry <= float64(warningDays) {
		return ExpiryStatusNearExpiry
	}

	// OK
	return ExpiryStatusOK
}
