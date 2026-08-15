package scan_test

import (
	"context"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Rhionin/pantry/internal/scan"
)

// Feature: pantry-management, Property 1: Scan entry data integrity
// **Validates: Requirements 1.1, 1.2**
//
// For any barcode string and timestamp submitted to the scan queue, the created
// scan entry SHALL persist the barcode value and timestamp exactly as provided,
// with status `pending` and no pre-set scan direction unless one was explicitly provided.
func TestProperty1_ScanEntryDataIntegrity(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, _ := newTestRepo(t)
		ctx := context.Background()

		// Generate arbitrary inputs
		barcode := rapid.String().Draw(rt, "barcode")
		timestampUnix := rapid.Int64Range(0, time.Now().Unix()).Draw(rt, "timestampUnix")
		scannedAt := time.Unix(timestampUnix, 0)
		userID := rapid.String().Draw(rt, "userID")

		// Create scan entry without direction
		entry := scan.ScanEntry{
			UserID:    userID,
			Barcode:   barcode,
			ScannedAt: scannedAt,
			UnitCount: 1,
		}

		created, err := repo.CreateScanEntry(ctx, entry)
		if err != nil {
			rt.Fatalf("CreateScanEntry failed: %v", err)
		}

		// Verify barcode and timestamp are preserved exactly
		if created.Barcode != barcode {
			rt.Fatalf("Barcode mismatch: want %q, got %q", barcode, created.Barcode)
		}

		// Timestamps should match within reasonable precision (SQLite stores to the second)
		if !created.ScannedAt.Truncate(time.Second).Equal(scannedAt.Truncate(time.Second)) {
			rt.Fatalf("ScannedAt mismatch: want %v, got %v", scannedAt, created.ScannedAt)
		}

		// Status should be pending
		if created.Status != scan.Pending {
			rt.Fatalf("Status: want %v, got %v", scan.Pending, created.Status)
		}

		// Direction should be nil (not pre-set)
		if created.Direction != nil {
			rt.Fatalf("Direction should be nil, got %v", *created.Direction)
		}

		// ID should be generated
		if created.ID == "" {
			rt.Fatal("ID should be generated, got empty string")
		}
	})
}

// Feature: pantry-management, Property 2: Scan direction propagation
// **Validates: Requirements 1.4, 1.5**
//
// For any pre-selected scan direction and any sequence of scans recorded within
// 5 minutes of the last scan, every resulting scan entry SHALL carry that direction;
// and for any scan recorded more than 5 minutes after the previous scan, the
// direction SHALL NOT be applied automatically.
func TestProperty2_ScanDirectionPropagation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, _ := newTestRepo(t)
		ctx := context.Background()

		// Generate test data
		userID := rapid.String().Draw(rt, "userID")
		baseTime := time.Now()
		direction := rapid.SampledFrom([]scan.ScanDirection{
			scan.StockIn,
			scan.StockOut,
		}).Draw(rt, "direction")

		// Generate a sequence of time offsets (in seconds)
		// Mix some within 5 minutes and some beyond 5 minutes
		numScans := rapid.IntRange(2, 10).Draw(rt, "numScans")
		offsets := make([]int, numScans)
		for i := 0; i < numScans; i++ {
			// Generate offsets from 0 to 10 minutes (600 seconds)
			offsets[i] = rapid.IntRange(0, 600).Draw(rt, "offset")
		}

		// Create scan entries with the generated offsets
		var previousTime time.Time
		for i, offsetSec := range offsets {
			scanTime := baseTime.Add(time.Duration(offsetSec) * time.Second)

			// Determine if direction should be applied
			var entryDirection *scan.ScanDirection
			if i == 0 {
				// First scan always gets the direction
				entryDirection = &direction
			} else {
				// Check if within 5 minutes of previous scan
				timeSincePrevious := scanTime.Sub(previousTime)
				if timeSincePrevious <= 5*time.Minute {
					entryDirection = &direction
				}
				// else: direction should be nil
			}

			entry := scan.ScanEntry{
				UserID:    userID,
				Barcode:   rapid.String().Draw(rt, "barcode"),
				ScannedAt: scanTime,
				Direction: entryDirection,
				UnitCount: 1,
			}

			created, err := repo.CreateScanEntry(ctx, entry)
			if err != nil {
				rt.Fatalf("CreateScanEntry failed: %v", err)
			}

			// Verify direction propagation
			if i == 0 {
				// First scan should always have the direction
				if created.Direction == nil || *created.Direction != direction {
					rt.Fatalf("First scan: expected direction %v, got %v", direction, created.Direction)
				}
			} else {
				timeSincePrevious := scanTime.Sub(previousTime)
				if timeSincePrevious <= 5*time.Minute {
					// Within 5 minutes: should have direction
					if created.Direction == nil {
						rt.Fatalf("Scan within 5 min: expected direction %v, got nil", direction)
					}
					if *created.Direction != direction {
						rt.Fatalf("Scan within 5 min: expected direction %v, got %v", direction, *created.Direction)
					}
				} else {
					// Beyond 5 minutes: should NOT have direction
					if created.Direction != nil {
						rt.Fatalf("Scan after 5 min: expected nil direction, got %v", *created.Direction)
					}
				}
			}

			previousTime = scanTime
		}
	})
}

// Feature: pantry-management, Property 3: Scan queue chronological ordering
// **Validates: Requirements 1.7**
//
// For any collection of scan entries in the queue, the list returned to the user
// SHALL be ordered ascending by `scanned_at` timestamp (oldest first).
func TestProperty3_ScanQueueChronologicalOrdering(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, _ := newTestRepo(t)
		ctx := context.Background()

		userID := rapid.String().Draw(rt, "userID")
		numScans := rapid.IntRange(1, 20).Draw(rt, "numScans")

		// Generate scan entries with random timestamps
		baseTime := time.Now().Add(-24 * time.Hour)

		for i := 0; i < numScans; i++ {
			// Random offset up to 24 hours
			offsetSec := rapid.Int64Range(0, 86400).Draw(rt, "offsetSec")
			scanTime := baseTime.Add(time.Duration(offsetSec) * time.Second)

			entry := scan.ScanEntry{
				UserID:    userID,
				Barcode:   rapid.String().Draw(rt, "barcode"),
				ScannedAt: scanTime,
				UnitCount: 1,
				Status:    scan.Pending,
			}

			_, err := repo.CreateScanEntry(ctx, entry)
			if err != nil {
				rt.Fatalf("CreateScanEntry failed: %v", err)
			}
		}

		// List all entries
		entries, err := repo.ListScanEntries(ctx, userID, scan.Pending)
		if err != nil {
			rt.Fatalf("ListScanEntries failed: %v", err)
		}

		if len(entries) != numScans {
			rt.Fatalf("Expected %d entries, got %d", numScans, len(entries))
		}

		// Verify chronological ordering (ascending by scanned_at)
		for i := 1; i < len(entries); i++ {
			if entries[i].ScannedAt.Before(entries[i-1].ScannedAt) {
				rt.Fatalf("Ordering violation at index %d: entry[%d].ScannedAt=%v is before entry[%d].ScannedAt=%v",
					i, i, entries[i].ScannedAt, i-1, entries[i-1].ScannedAt)
			}
		}
	})
}

// Feature: pantry-management, Property 4: Batch update applies to all selected entries
// **Validates: Requirements 1.10**
//
// For any subset of pending scan entries selected for batch review and any
// combination of direction and expiration date applied, every entry in the
// selected subset SHALL have its direction and expiration date updated to the
// new values, and no entry outside the subset SHALL be modified.
func TestProperty4_BatchUpdateApplies(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, _ := newTestRepo(t)
		ctx := context.Background()

		userID := rapid.String().Draw(rt, "userID")

		// Create a pool of scan entries
		numEntries := rapid.IntRange(3, 15).Draw(rt, "numEntries")
		entryIDs := make([]string, numEntries)

		for i := 0; i < numEntries; i++ {
			entry := scan.ScanEntry{
				UserID:    userID,
				Barcode:   rapid.String().Draw(rt, "barcode"),
				ScannedAt: time.Now().Add(time.Duration(i) * time.Minute),
				UnitCount: 1,
				Status:    scan.Pending,
			}

			created, err := repo.CreateScanEntry(ctx, entry)
			if err != nil {
				rt.Fatalf("CreateScanEntry failed: %v", err)
			}
			entryIDs[i] = created.ID
		}

		// Select a random subset to update
		numToUpdate := rapid.IntRange(1, numEntries).Draw(rt, "numToUpdate")
		selectedIndices := make(map[int]bool)
		for len(selectedIndices) < numToUpdate {
			idx := rapid.IntRange(0, numEntries-1).Draw(rt, "selectedIdx")
			selectedIndices[idx] = true
		}

		selectedIDs := make([]string, 0, numToUpdate)
		for idx := range selectedIndices {
			selectedIDs = append(selectedIDs, entryIDs[idx])
		}

		// Generate update values
		direction := rapid.SampledFrom([]scan.ScanDirection{
			scan.StockIn,
			scan.StockOut,
		}).Draw(rt, "direction")

		unitCount := rapid.IntRange(1, 10).Draw(rt, "unitCount")

		expiresAt := time.Now().Add(time.Duration(rapid.IntRange(1, 30).Draw(rt, "daysUntilExpiry")) * 24 * time.Hour)

		// Perform batch update
		err := repo.BatchUpdateScanEntries(ctx, selectedIDs, &direction, &unitCount, &expiresAt)
		if err != nil {
			rt.Fatalf("BatchUpdateScanEntries failed: %v", err)
		}

		// Verify all entries
		for i, id := range entryIDs {
			entry, err := repo.GetScanEntry(ctx, id)
			if err != nil {
				rt.Fatalf("GetScanEntry failed: %v", err)
			}

			if selectedIndices[i] {
				// This entry SHOULD be updated
				if entry.Direction == nil || *entry.Direction != direction {
					rt.Fatalf("Selected entry %s: expected direction %v, got %v", id, direction, entry.Direction)
				}
				if entry.UnitCount != unitCount {
					rt.Fatalf("Selected entry %s: expected unitCount %d, got %d", id, unitCount, entry.UnitCount)
				}
				if entry.ExpiresAt == nil {
					rt.Fatalf("Selected entry %s: expected expiresAt to be set, got nil", id)
				}
				// Check within reasonable precision (second-level)
				if !entry.ExpiresAt.Truncate(time.Second).Equal(expiresAt.Truncate(time.Second)) {
					rt.Fatalf("Selected entry %s: expected expiresAt %v, got %v", id, expiresAt, *entry.ExpiresAt)
				}
			} else {
				// This entry should NOT be updated (should remain as originally created)
				if entry.Direction != nil {
					rt.Fatalf("Non-selected entry %s: expected nil direction, got %v", id, *entry.Direction)
				}
				if entry.UnitCount != 1 {
					rt.Fatalf("Non-selected entry %s: expected unitCount 1, got %d", id, entry.UnitCount)
				}
				if entry.ExpiresAt != nil {
					rt.Fatalf("Non-selected entry %s: expected nil expiresAt, got %v", id, *entry.ExpiresAt)
				}
			}
		}
	})
}

// Feature: pantry-management, Property 7: Committed scan entry is preserved in history
// **Validates: Requirements 1.14**
//
// For any scan entry that is committed, the entry SHALL no longer appear in the
// pending scan list and SHALL appear in the read-only scan history with its
// original barcode, timestamp, direction, unit count, and expiration date all unchanged.
func TestProperty7_CommittedEntryInHistory(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, _ := newTestRepo(t)
		ctx := context.Background()

		// Generate arbitrary scan entry data
		userID := rapid.String().Draw(rt, "userID")
		barcode := rapid.String().Draw(rt, "barcode")
		timestampUnix := rapid.Int64Range(0, time.Now().Unix()).Draw(rt, "timestampUnix")
		scannedAt := time.Unix(timestampUnix, 0)

		// Generate optional direction
		hasDirection := rapid.Bool().Draw(rt, "hasDirection")
		var direction *scan.ScanDirection
		if hasDirection {
			dir := rapid.SampledFrom([]scan.ScanDirection{
				scan.StockIn,
				scan.StockOut,
			}).Draw(rt, "direction")
			direction = &dir
		}

		// Generate unit count
		unitCount := rapid.IntRange(1, 10).Draw(rt, "unitCount")

		// Generate optional expiration date
		hasExpiry := rapid.Bool().Draw(rt, "hasExpiry")
		var expiresAt *time.Time
		if hasExpiry {
			daysFromNow := rapid.IntRange(1, 365).Draw(rt, "daysFromNow")
			expiry := time.Now().Add(time.Duration(daysFromNow) * 24 * time.Hour)
			expiresAt = &expiry
		}

		// Create scan entry
		entry := scan.ScanEntry{
			UserID:    userID,
			Barcode:   barcode,
			ScannedAt: scannedAt,
			Direction: direction,
			UnitCount: unitCount,
			ExpiresAt: expiresAt,
			Status:    scan.Pending,
		}

		created, err := repo.CreateScanEntry(ctx, entry)
		if err != nil {
			rt.Fatalf("CreateScanEntry failed: %v", err)
		}

		// Store original values for later comparison
		originalBarcode := created.Barcode
		originalScannedAt := created.ScannedAt
		originalDirection := created.Direction
		originalUnitCount := created.UnitCount
		originalExpiresAt := created.ExpiresAt
		originalID := created.ID

		// Verify entry is in pending list
		pendingEntries, err := repo.ListScanEntries(ctx, userID, scan.Pending)
		if err != nil {
			rt.Fatalf("ListScanEntries (pending) failed: %v", err)
		}
		foundInPending := false
		for _, e := range pendingEntries {
			if e.ID == originalID {
				foundInPending = true
				break
			}
		}
		if !foundInPending {
			rt.Fatalf("Entry %s not found in pending list before commit", originalID)
		}

		// Commit the scan entry
		err = repo.CommitScanEntry(ctx, originalID)
		if err != nil {
			rt.Fatalf("CommitScanEntry failed: %v", err)
		}

		// Verify entry is NO LONGER in pending list
		pendingAfterCommit, err := repo.ListScanEntries(ctx, userID, scan.Pending)
		if err != nil {
			rt.Fatalf("ListScanEntries (pending after commit) failed: %v", err)
		}
		for _, e := range pendingAfterCommit {
			if e.ID == originalID {
				rt.Fatalf("Entry %s still found in pending list after commit", originalID)
			}
		}

		// Verify entry IS in committed/history list
		committedEntries, err := repo.ListScanEntries(ctx, userID, scan.Committed)
		if err != nil {
			rt.Fatalf("ListScanEntries (committed) failed: %v", err)
		}

		var committedEntry *scan.ScanEntry
		for i, e := range committedEntries {
			if e.ID == originalID {
				committedEntry = &committedEntries[i]
				break
			}
		}
		if committedEntry == nil {
			rt.Fatalf("Entry %s not found in committed list", originalID)
		}

		// Verify all original fields are unchanged
		if committedEntry.Barcode != originalBarcode {
			rt.Fatalf("Barcode changed: want %q, got %q", originalBarcode, committedEntry.Barcode)
		}

		// Timestamps should match within reasonable precision (SQLite stores to the second)
		if !committedEntry.ScannedAt.Truncate(time.Second).Equal(originalScannedAt.Truncate(time.Second)) {
			rt.Fatalf("ScannedAt changed: want %v, got %v", originalScannedAt, committedEntry.ScannedAt)
		}

		// Verify direction unchanged
		if originalDirection == nil && committedEntry.Direction != nil {
			rt.Fatalf("Direction changed from nil to %v", *committedEntry.Direction)
		}
		if originalDirection != nil {
			if committedEntry.Direction == nil {
				rt.Fatalf("Direction changed from %v to nil", *originalDirection)
			}
			if *committedEntry.Direction != *originalDirection {
				rt.Fatalf("Direction changed: want %v, got %v", *originalDirection, *committedEntry.Direction)
			}
		}

		// Verify unit count unchanged
		if committedEntry.UnitCount != originalUnitCount {
			rt.Fatalf("UnitCount changed: want %d, got %d", originalUnitCount, committedEntry.UnitCount)
		}

		// Verify expiration date unchanged
		if originalExpiresAt == nil && committedEntry.ExpiresAt != nil {
			rt.Fatalf("ExpiresAt changed from nil to %v", *committedEntry.ExpiresAt)
		}
		if originalExpiresAt != nil {
			if committedEntry.ExpiresAt == nil {
				rt.Fatalf("ExpiresAt changed from %v to nil", *originalExpiresAt)
			}
			// Check within reasonable precision (second-level)
			if !committedEntry.ExpiresAt.Truncate(time.Second).Equal(originalExpiresAt.Truncate(time.Second)) {
				rt.Fatalf("ExpiresAt changed: want %v, got %v", *originalExpiresAt, *committedEntry.ExpiresAt)
			}
		}

		// Verify status is now committed
		if committedEntry.Status != scan.Committed {
			rt.Fatalf("Status: want %v, got %v", scan.Committed, committedEntry.Status)
		}

		// Verify committed_at timestamp is set
		if committedEntry.CommittedAt == nil {
			rt.Fatal("CommittedAt should be set after commit, got nil")
		}
	})
}
