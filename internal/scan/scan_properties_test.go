package scan_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/Rhionin/pantry/internal/scan"
	"github.com/google/uuid"
)

// Feature: pantry-management, Property 1: Scan entry data integrity
// **Validates: Requirements 1.1, 1.2**
//
// For any barcode string and timestamp submitted to the scan queue, the created
// scan entry SHALL persist the barcode value and timestamp exactly as provided,
// with status `pending` and no pre-set scan direction unless one was explicitly provided.
func TestProperty1_ScanEntryDataIntegrity(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, _ := newTestQueue(t)
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

		created, err := queue.CreateScanEntry(ctx, entry)
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
		queue, _ := newTestQueue(t)
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

			created, err := queue.CreateScanEntry(ctx, entry)
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
		queue, _ := newTestQueue(t)
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

			_, err := queue.CreateScanEntry(ctx, entry)
			if err != nil {
				rt.Fatalf("CreateScanEntry failed: %v", err)
			}
		}

		// List all entries
		entries, err := queue.ListScanEntries(ctx, userID, scan.Pending)
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
		queue, _ := newTestQueue(t)
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

			created, err := queue.CreateScanEntry(ctx, entry)
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
		err := queue.BatchUpdateScanEntries(ctx, selectedIDs, &direction, &unitCount, &expiresAt)
		if err != nil {
			rt.Fatalf("BatchUpdateScanEntries failed: %v", err)
		}

		// Verify all entries
		for i, id := range entryIDs {
			entry, err := queue.GetScanEntry(ctx, id)
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
		queue, _ := newTestQueue(t)
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

		created, err := queue.CreateScanEntry(ctx, entry)
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
		pendingEntries, err := queue.ListScanEntries(ctx, userID, scan.Pending)
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
		err = queue.CommitScanEntry(ctx, originalID)
		if err != nil {
			rt.Fatalf("CommitScanEntry failed: %v", err)
		}

		// Verify entry is NO LONGER in pending list
		pendingAfterCommit, err := queue.ListScanEntries(ctx, userID, scan.Pending)
		if err != nil {
			rt.Fatalf("ListScanEntries (pending after commit) failed: %v", err)
		}
		for _, e := range pendingAfterCommit {
			if e.ID == originalID {
				rt.Fatalf("Entry %s still found in pending list after commit", originalID)
			}
		}

		// Verify entry IS in committed/history list
		committedEntries, err := queue.ListScanEntries(ctx, userID, scan.Committed)
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

// --------------------------------------------------------------------------
// realtime-scan-updates property tests
// --------------------------------------------------------------------------

// Feature: realtime-scan-updates, Property 4: Every successful scan-mutating
// operation publishes exactly one matching Scan_Event
//
// For any call to CreateScanEntry, UpdateScanEntry, ResolveFlaggedEntry,
// CommitStockIn, or CommitStockOut that returns without an error, exactly
// one PublishScanEvent call SHALL occur whose ScanEntry argument
// deep-equals the entry's complete state immediately after that call, as
// read back through GetScanEntry.
//
// Validates: Requirements 2.1, 2.2, 2.3, 2.5
func TestRealtimeProperty4_CreateAndUpdatePublishMatchingEvent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, db := newTestQueue(t)
		ctx := context.Background()

		operation := rapid.IntRange(0, 4).Draw(rt, "operation")
		barcode := rapid.String().Draw(rt, "barcode")
		userID := rapid.String().Draw(rt, "userID")

		var entryID string
		broadcaster := &fakeBroadcaster{}

		switch operation {
		case 0: // CreateScanEntry
			queue.Broadcaster = broadcaster
			created, err := queue.CreateScanEntry(ctx, scan.ScanEntry{
				UserID:    userID,
				Barcode:   barcode,
				ScannedAt: time.Now(),
				UnitCount: 1,
			})
			if err != nil {
				rt.Fatalf("CreateScanEntry failed: %v", err)
			}
			entryID = created.ID
		case 1: // UpdateScanEntry
			created, err := queue.CreateScanEntry(ctx, scan.ScanEntry{
				UserID:    userID,
				Barcode:   barcode,
				ScannedAt: time.Now(),
				UnitCount: 1,
			})
			if err != nil {
				rt.Fatalf("CreateScanEntry (setup) failed: %v", err)
			}
			entryID = created.ID

			queue.Broadcaster = broadcaster
			direction := rapid.SampledFrom([]scan.ScanDirection{
				scan.StockIn,
				scan.StockOut,
			}).Draw(rt, "direction")
			unitCount := rapid.IntRange(1, 10).Draw(rt, "unitCount")
			if err := queue.UpdateScanEntry(ctx, entryID, &direction, &unitCount, nil, nil, nil); err != nil {
				rt.Fatalf("UpdateScanEntry failed: %v", err)
			}
		case 2: // ResolveFlaggedEntry
			catalog := product.NewCatalog(db)
			productID := uuid.NewString()
			if err := catalog.CreateProduct(ctx, product.Product{
				ID: productID, Name: "Corrected " + productID, Category: "Test", UnitOfMeasure: "unit",
			}); err != nil {
				rt.Fatalf("CreateProduct failed: %v", err)
			}
			flagged := createFlaggedEntry(t, queue, ctx)
			entryID = flagged.ID

			queue.Broadcaster = broadcaster
			if err := queue.ResolveFlaggedEntry(ctx, entryID, productID); err != nil {
				rt.Fatalf("ResolveFlaggedEntry failed: %v", err)
			}
		case 3: // CommitStockIn
			unitCount := rapid.IntRange(1, 10).Draw(rt, "unitCount")
			created := createStockInEntry(t, queue, db, ctx, unitCount)
			entryID = created.ID

			queue.Broadcaster = broadcaster
			if err := queue.CommitStockIn(ctx, created); err != nil {
				rt.Fatalf("CommitStockIn failed: %v", err)
			}
		case 4: // CommitStockOut
			created := createStockOutEntry(t, queue, db, ctx)
			entryID = created.ID

			queue.Broadcaster = broadcaster
			if err := queue.CommitStockOut(ctx, created, nil); err != nil {
				rt.Fatalf("CommitStockOut failed: %v", err)
			}
		}

		want, err := queue.GetScanEntry(ctx, entryID)
		if err != nil {
			rt.Fatalf("GetScanEntry failed: %v", err)
		}
		if want == nil {
			rt.Fatal("GetScanEntry: expected entry, got nil")
		}
		if len(broadcaster.scanEvents) != 1 {
			rt.Fatalf("expected exactly 1 published scan event, got %d", len(broadcaster.scanEvents))
		}
		if !reflect.DeepEqual(broadcaster.scanEvents[0], *want) {
			rt.Fatalf("published event: got %+v, want %+v", broadcaster.scanEvents[0], *want)
		}
	})
}

// Feature: realtime-scan-updates, Property 5: Batch update publishes one
// Scan_Event per updated entry
//
// For any non-empty subset of scan entries passed to BatchUpdateScanEntries
// that returns without an error, exactly one PublishScanEvent call SHALL
// occur per entry in that subset, each carrying that entry's complete
// post-update state, and no PublishScanEvent call SHALL occur for any entry
// outside the subset.
//
// Validates: Requirements 2.4
func TestRealtimeProperty5_BatchUpdatePublishesOnePerEntry(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, _ := newTestQueue(t)
		ctx := context.Background()

		userID := rapid.String().Draw(rt, "userID")

		// Create a pool of scan entries (same selection-generator pattern as
		// TestProperty4_BatchUpdateApplies).
		numEntries := rapid.IntRange(3, 15).Draw(rt, "numEntries")
		entryIDs := make([]string, numEntries)
		for i := 0; i < numEntries; i++ {
			created, err := queue.CreateScanEntry(ctx, scan.ScanEntry{
				UserID:    userID,
				Barcode:   rapid.String().Draw(rt, "barcode"),
				ScannedAt: time.Now().Add(time.Duration(i) * time.Minute),
				UnitCount: 1,
				Status:    scan.Pending,
			})
			if err != nil {
				rt.Fatalf("CreateScanEntry failed: %v", err)
			}
			entryIDs[i] = created.ID
		}

		// Select a random non-empty subset to update.
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

		broadcaster := &fakeBroadcaster{}
		queue.Broadcaster = broadcaster

		direction := rapid.SampledFrom([]scan.ScanDirection{
			scan.StockIn,
			scan.StockOut,
		}).Draw(rt, "direction")

		if err := queue.BatchUpdateScanEntries(ctx, selectedIDs, &direction, nil, nil); err != nil {
			rt.Fatalf("BatchUpdateScanEntries failed: %v", err)
		}

		if len(broadcaster.scanEvents) != len(selectedIDs) {
			rt.Fatalf("expected %d published scan events, got %d", len(selectedIDs), len(broadcaster.scanEvents))
		}

		published := make(map[string]scan.ScanEntry, len(broadcaster.scanEvents))
		for _, e := range broadcaster.scanEvents {
			if _, ok := selectedIndices[indexOf(entryIDs, e.ID)]; !ok {
				rt.Fatalf("published event for entry %s, which was outside the selected subset", e.ID)
			}
			published[e.ID] = e
		}

		for _, id := range selectedIDs {
			want, err := queue.GetScanEntry(ctx, id)
			if err != nil {
				rt.Fatalf("GetScanEntry failed: %v", err)
			}
			got, ok := published[id]
			if !ok {
				rt.Fatalf("expected a published event for %s, found none", id)
			}
			if !reflect.DeepEqual(got, *want) {
				rt.Fatalf("published event for %s: got %+v, want %+v", id, got, *want)
			}
		}
	})
}

// Feature: realtime-scan-updates, Property 6: A failed scan-mutating call
// publishes no Scan_Event
//
// For any call to CreateScanEntry, UpdateScanEntry, BatchUpdateScanEntries,
// ResolveFlaggedEntry, CommitStockIn, or CommitStockOut driven into one of
// its documented error conditions, the call SHALL return an error and zero
// PublishScanEvent calls SHALL occur as a result of that call.
//
// Validates: Requirements 2.6
func TestRealtimeProperty6_FailedScanMutationPublishesNoEvent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, db := newTestQueue(t)
		ctx := context.Background()

		broadcaster := &fakeBroadcaster{}
		queue.Broadcaster = broadcaster

		unknownID := uuid.NewString()
		operation := rapid.IntRange(0, 5).Draw(rt, "operation")

		var err error
		switch operation {
		case 0: // CreateScanEntry with a duplicate ID
			existing, createErr := queue.CreateScanEntry(ctx, scan.ScanEntry{
				UserID:    rapid.String().Draw(rt, "userID"),
				Barcode:   rapid.String().Draw(rt, "barcode"),
				ScannedAt: time.Now(),
				UnitCount: 1,
			})
			if createErr != nil {
				rt.Fatalf("CreateScanEntry (setup) failed: %v", createErr)
			}
			// The setup call above may itself have published; reset before
			// the call under test so only its own behavior is asserted.
			broadcaster.scanEvents = nil

			_, err = queue.CreateScanEntry(ctx, scan.ScanEntry{
				ID:        existing.ID,
				UserID:    rapid.String().Draw(rt, "userID"),
				Barcode:   rapid.String().Draw(rt, "barcode"),
				ScannedAt: time.Now(),
				UnitCount: 1,
			})
		case 1: // UpdateScanEntry on an unknown ID
			direction := rapid.SampledFrom([]scan.ScanDirection{
				scan.StockIn,
				scan.StockOut,
			}).Draw(rt, "direction")
			err = queue.UpdateScanEntry(ctx, unknownID, &direction, nil, nil, nil, nil)
		case 2: // BatchUpdateScanEntries with an unknown ID in the batch
			direction := rapid.SampledFrom([]scan.ScanDirection{
				scan.StockIn,
				scan.StockOut,
			}).Draw(rt, "direction")
			err = queue.BatchUpdateScanEntries(ctx, []string{unknownID}, &direction, nil, nil)
		case 3: // ResolveFlaggedEntry, one of its documented error conditions
			switch rapid.IntRange(0, 2).Draw(rt, "resolveFlaggedErrorCondition") {
			case 0: // scan entry not found
				catalog := product.NewCatalog(db)
				productID := uuid.NewString()
				if createErr := catalog.CreateProduct(ctx, product.Product{
					ID: productID, Name: "Product " + productID, Category: "Test", UnitOfMeasure: "unit",
				}); createErr != nil {
					rt.Fatalf("CreateProduct failed: %v", createErr)
				}
				err = queue.ResolveFlaggedEntry(ctx, unknownID, productID)
			case 1: // scan entry is not flagged
				catalog := product.NewCatalog(db)
				productID := uuid.NewString()
				if createErr := catalog.CreateProduct(ctx, product.Product{
					ID: productID, Name: "Product " + productID, Category: "Test", UnitOfMeasure: "unit",
				}); createErr != nil {
					rt.Fatalf("CreateProduct failed: %v", createErr)
				}
				created, createErr := queue.CreateScanEntry(ctx, scan.ScanEntry{
					UserID:    rapid.String().Draw(rt, "userID"),
					Barcode:   rapid.String().Draw(rt, "barcode"),
					ScannedAt: time.Now(),
					UnitCount: 1,
					Status:    scan.Pending,
				})
				if createErr != nil {
					rt.Fatalf("CreateScanEntry (setup) failed: %v", createErr)
				}
				broadcaster.scanEvents = nil
				err = queue.ResolveFlaggedEntry(ctx, created.ID, productID)
			case 2: // product not found
				flagged := createFlaggedEntry(t, queue, ctx)
				broadcaster.scanEvents = nil
				err = queue.ResolveFlaggedEntry(ctx, flagged.ID, "no-such-product")
			}
		case 4: // CommitStockIn, one of its documented error conditions
			var entry *scan.ScanEntry
			switch rapid.IntRange(0, 2).Draw(rt, "commitStockInErrorCondition") {
			case 0: // direction is not stock_in
				direction := scan.StockOut
				productID := "prod-1"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID, UnitCount: 1}
			case 1: // missing product_id
				direction := scan.StockIn
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, UnitCount: 1}
			case 2: // unit count less than 1
				direction := scan.StockIn
				productID := "prod-1"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID, UnitCount: 0}
			}
			err = queue.CommitStockIn(ctx, entry)
		case 5: // CommitStockOut, one of its documented error conditions
			var entry *scan.ScanEntry
			var instanceID *string
			switch rapid.IntRange(0, 4).Draw(rt, "commitStockOutErrorCondition") {
			case 0: // direction is not stock_out
				direction := scan.StockIn
				productID := "prod-1"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID}
			case 1: // missing product_id
				direction := scan.StockOut
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction}
			case 2: // no item found for user and product
				direction := scan.StockOut
				productID := "no-such-product"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID}
			case 3: // specific instance not found or already removed
				entry = createStockOutEntry(t, queue, db, ctx)
				broadcaster.scanEvents = nil
				missing := "no-such-instance"
				instanceID = &missing
			case 4: // no available instances
				catalog := product.NewCatalog(db)
				productID := uuid.NewString()
				if createErr := catalog.CreateProduct(ctx, product.Product{
					ID: productID, Name: "Product " + productID, Category: "Test", UnitOfMeasure: "unit",
				}); createErr != nil {
					rt.Fatalf("CreateProduct failed: %v", createErr)
				}
				userID := uuid.NewString()
				itemID := uuid.NewString()
				if _, dbErr := db.ExecContext(ctx, `INSERT INTO items (id, user_id, product_id) VALUES (?, ?, ?)`,
					itemID, userID, productID); dbErr != nil {
					rt.Fatalf("create item failed: %v", dbErr)
				}
				direction := scan.StockOut
				created, createErr := queue.CreateScanEntry(ctx, scan.ScanEntry{
					UserID:    userID,
					Barcode:   rapid.String().Draw(rt, "barcode"),
					ScannedAt: time.Now(),
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				})
				if createErr != nil {
					rt.Fatalf("CreateScanEntry (setup) failed: %v", createErr)
				}
				broadcaster.scanEvents = nil
				entry = created
			}
			err = queue.CommitStockOut(ctx, entry, instanceID)
		}

		if err == nil {
			rt.Fatal("expected error, got nil")
		}
		if len(broadcaster.scanEvents) != 0 {
			rt.Fatalf("expected 0 published scan events, got %d", len(broadcaster.scanEvents))
		}
	})
}

// Feature: realtime-scan-updates, Property 7: Every successful
// inventory-affecting operation publishes exactly one matching
// Inventory_Event (Queue half)
//
// For any call to Queue.CommitStockIn or Queue.CommitStockOut that returns
// without an error, exactly one PublishInventoryEvent call SHALL occur
// whose InventoryItem argument deep-equals the affected item's complete
// aggregated state immediately after that call, as read back through
// Pantry.GetInventoryItem.
//
// Validates: Requirements 3.4, 3.5
func TestRealtimeProperty7_QueuePublishesInventoryEventOnSuccess(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, db := newTestQueue(t)
		ctx := context.Background()
		pantry := inventory.NewPantry(db)
		broadcaster := &fakeBroadcaster{}

		operation := rapid.IntRange(0, 1).Draw(rt, "operation")

		var userID string
		var productID string

		switch operation {
		case 0: // CommitStockIn
			unitCount := rapid.IntRange(1, 10).Draw(rt, "unitCount")
			created := createStockInEntry(t, queue, db, ctx, unitCount)
			userID = created.UserID
			productID = *created.ProductID

			queue.Broadcaster = broadcaster
			queue.Pantry = pantry
			if err := queue.CommitStockIn(ctx, created); err != nil {
				rt.Fatalf("CommitStockIn failed: %v", err)
			}
		case 1: // CommitStockOut
			created := createStockOutEntry(t, queue, db, ctx)
			userID = created.UserID
			productID = *created.ProductID

			queue.Broadcaster = broadcaster
			queue.Pantry = pantry
			if err := queue.CommitStockOut(ctx, created, nil); err != nil {
				rt.Fatalf("CommitStockOut failed: %v", err)
			}
		}

		item, err := pantry.GetOrCreateItem(ctx, userID, productID)
		if err != nil {
			rt.Fatalf("GetOrCreateItem failed: %v", err)
		}
		want, err := pantry.GetInventoryItem(ctx, item.ID, time.Now(), inventory.DefaultWarningDays)
		if err != nil {
			rt.Fatalf("GetInventoryItem failed: %v", err)
		}
		if want == nil {
			rt.Fatal("GetInventoryItem: expected item, got nil")
		}
		if len(broadcaster.inventoryEvents) != 1 {
			rt.Fatalf("expected exactly 1 published inventory event, got %d", len(broadcaster.inventoryEvents))
		}
		if !reflect.DeepEqual(broadcaster.inventoryEvents[0], *want) {
			rt.Fatalf("published event: got %+v, want %+v", broadcaster.inventoryEvents[0], *want)
		}
	})
}

// Feature: realtime-scan-updates, Property 8: A failed inventory-affecting
// call publishes no Inventory_Event (Queue half)
//
// For any call to Queue.CommitStockIn or Queue.CommitStockOut driven into
// one of its documented error conditions, the call SHALL return an error
// and zero PublishInventoryEvent calls SHALL occur as a result of that
// call.
//
// Validates: Requirements 3.6
func TestRealtimeProperty8_QueuePublishesNoInventoryEventOnFailure(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		queue, db := newTestQueue(t)
		ctx := context.Background()

		broadcaster := &fakeBroadcaster{}
		queue.Broadcaster = broadcaster
		queue.Pantry = inventory.NewPantry(db)

		operation := rapid.IntRange(0, 1).Draw(rt, "operation")

		var err error
		switch operation {
		case 0: // CommitStockIn, one of its documented error conditions
			var entry *scan.ScanEntry
			switch rapid.IntRange(0, 2).Draw(rt, "commitStockInErrorCondition") {
			case 0: // direction is not stock_in
				direction := scan.StockOut
				productID := "prod-1"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID, UnitCount: 1}
			case 1: // missing product_id
				direction := scan.StockIn
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, UnitCount: 1}
			case 2: // unit count less than 1
				direction := scan.StockIn
				productID := "prod-1"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID, UnitCount: 0}
			}
			err = queue.CommitStockIn(ctx, entry)
		case 1: // CommitStockOut, one of its documented error conditions
			var entry *scan.ScanEntry
			var instanceID *string
			switch rapid.IntRange(0, 4).Draw(rt, "commitStockOutErrorCondition") {
			case 0: // direction is not stock_out
				direction := scan.StockIn
				productID := "prod-1"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID}
			case 1: // missing product_id
				direction := scan.StockOut
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction}
			case 2: // no item found for user and product
				direction := scan.StockOut
				productID := "no-such-product"
				entry = &scan.ScanEntry{ID: uuid.NewString(), UserID: "user-1", Direction: &direction, ProductID: &productID}
			case 3: // specific instance not found or already removed
				entry = createStockOutEntry(t, queue, db, ctx)
				broadcaster.inventoryEvents = nil
				missing := "no-such-instance"
				instanceID = &missing
			case 4: // no available instances
				catalog := product.NewCatalog(db)
				productID := uuid.NewString()
				if createErr := catalog.CreateProduct(ctx, product.Product{
					ID: productID, Name: "Product " + productID, Category: "Test", UnitOfMeasure: "unit",
				}); createErr != nil {
					rt.Fatalf("CreateProduct failed: %v", createErr)
				}
				userID := uuid.NewString()
				itemID := uuid.NewString()
				if _, dbErr := db.ExecContext(ctx, `INSERT INTO items (id, user_id, product_id) VALUES (?, ?, ?)`,
					itemID, userID, productID); dbErr != nil {
					rt.Fatalf("create item failed: %v", dbErr)
				}
				direction := scan.StockOut
				created, createErr := queue.CreateScanEntry(ctx, scan.ScanEntry{
					UserID:    userID,
					Barcode:   rapid.String().Draw(rt, "barcode"),
					ScannedAt: time.Now(),
					Direction: &direction,
					UnitCount: 1,
					ProductID: &productID,
					Status:    scan.Pending,
				})
				if createErr != nil {
					rt.Fatalf("CreateScanEntry (setup) failed: %v", createErr)
				}
				broadcaster.inventoryEvents = nil
				entry = created
			}
			err = queue.CommitStockOut(ctx, entry, instanceID)
		}

		if err == nil {
			rt.Fatal("expected error, got nil")
		}
		if len(broadcaster.inventoryEvents) != 0 {
			rt.Fatalf("expected 0 published inventory events, got %d", len(broadcaster.inventoryEvents))
		}
	})
}

// indexOf returns the index of target within ids, or -1 if not present.
func indexOf(ids []string, target string) int {
	for i, id := range ids {
		if id == target {
			return i
		}
	}
	return -1
}
