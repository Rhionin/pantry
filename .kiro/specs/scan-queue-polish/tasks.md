# Implementation Plan: Scan Queue Polish

## Overview

Convert the feature design into a series of prompts for a code-generation LLM that will implement each step with incremental progress. Make sure that each prompt builds on the previous prompts, and ends with wiring things together. There should be no hanging or orphaned code that isn't integrated into a previous step. Focus ONLY on tasks that involve writing, modifying, or testing code.

Work proceeds bottom-up: shared `queueUtils.ts` helpers first (including the new `getEntriesForView` view-partitioning helper), then the two consumers that are independent of each other (`ScanQueuePage`'s Queue_Direction_Tabs plus its view-scoped select-all control and grid, and the commit-to-approve rename), then the `ScanEntryCard` feature set in the same incremental order the design lays it out (remove → unit count → expiration date → checkbox → change indicator), then the `BatchReviewPanel` simplification that depends on cards owning their own expiration date.

## Tasks

- [x] 1. Add `queueUtils.ts` pure functions for batch eligibility, select-all, unit-count validation, and queue view partitioning
  - [x] 1.1 Implement `isBatchEligible`, `toggleSelectAll`, and `isValidUnitCount` in `frontend/src/components/queue/queueUtils.ts`
    - Add `isBatchEligible` per the design's definition (`status === 'pending' && direction !== null`)
    - Add `toggleSelectAll(entries, selectedIds)` per the design's implementation, preserving ids of non-eligible entries
    - Add `isValidUnitCount(value)` per the design's implementation (integer, `>= 1`)
    - _Requirements: 2.6, 3.2, 3.3, 3.4_

  - [x] 1.2 Write property test for `isValidUnitCount`
    - **Property 1: Unit count validity**
    - **Validates: Requirements 2.6**
    - Use `fast-check`, ≥100 iterations, following the existing Property 9/10 format in `queueUtils.test.ts`

  - [x] 1.3 Write property test for `toggleSelectAll`
    - **Property 2: Select-all toggles exactly the eligible entries and preserves the rest**
    - **Validates: Requirements 3.2, 3.3, 3.4**
    - Use `fast-check`, ≥100 iterations, following the existing Property 9/10 format in `queueUtils.test.ts`

  - [x] 1.4 Write unit tests for `isBatchEligible` and concrete `isValidUnitCount` examples
    - Cover `isBatchEligible` for pending+direction-set, pending+direction-null, and non-pending statuses
    - Cover concrete examples such as `isValidUnitCount(0) === false` and `isValidUnitCount(1) === true`
    - _Requirements: 2.6, 3.2_

  - [x] 1.5 Implement `QueueView` and `getEntriesForView` in `frontend/src/components/queue/queueUtils.ts`
    - Add the `QueueView` union type (`'stock_in' | 'stock_out'`)
    - Add `getEntriesForView(entries, view)` per the design's implementation: `Stock_In_View` is exactly `direction === 'stock_in'`; `Stock_Out_View` is the catchall for `direction === 'stock_out'` and `direction === null`
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

  - [x] 1.6 Write property test for `getEntriesForView`
    - **Property 3: Every entry belongs to exactly one queue view**
    - **Validates: Requirements 9.1, 9.2, 9.3, 9.4**
    - Use `fast-check`, ≥100 iterations, generating entries with all three `direction` values (`stock_in`, `stock_out`, `null`) and asserting the two returned lists are disjoint and together cover the input

- [x] 2. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 3. Add Queue_Direction_Tabs to `ScanQueuePage` and scope batch controls to the Active_View
  - [x] 3.1 Implement `Queue_Direction_Tabs` and derive `viewEntries`
    - Add `activeView` state via `useState<QueueView>('stock_out')` so it always initializes to `Stock_Out_View` fresh on every mount, with no read from or write to `localStorage` or any other persistence
    - Render a Mantine `Tabs` above the grid with `Stock_In_View`/`Stock_Out_View` tabs; its `onChange` handler validates the incoming value, calls `setActiveView`, and calls `setSelectedIds([])`
    - Derive `viewEntries = getEntriesForView(entries, activeView)` once per render for the rest of the page to consume
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 11.1, 11.2_

  - [x] 3.2 Update the `Select_All_Control` to consume `viewEntries` instead of `entries`
    - Compute `eligibleEntries`/`allEligibleSelected` from `viewEntries` and `selectedIds` using `isBatchEligible`
    - Disable the checkbox when there are no eligible entries in the Active_View; keep `aria-label="Select all eligible scans for batch approval"`
    - On change, call `setSelectedIds((current) => toggleSelectAll(viewEntries, current))`
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 10.1, 10.2, 10.3_

  - [x] 3.3 Scope the entry grid and `BatchReviewPanel`'s `selectedIds` to the Active_View
    - Map the grid over `viewEntries` instead of `entries`
    - Compute `viewEntryIds` from `viewEntries` and pass `BatchReviewPanel` `selectedIds={selectedIds.filter((id) => viewEntryIds.has(id))}`
    - _Requirements: 8.6, 9.1, 9.2, 9.3, 9.4, 10.4, 10.5_

  - [x] 3.4 Write RTL tests for default/reset-to-Stock_Out_View-on-mount behavior
    - Assert `Stock_Out_View` is the Active_View on initial mount, and remains so on a fresh mount even after a prior instance was switched to `Stock_In_View` (via unmount/remount, no persisted storage involved)
    - _Requirements: 8.2, 8.3_

  - [x] 3.5 Write RTL tests for per-tab filtered rendering
    - Assert activating each tab renders only that view's entries: `Stock_In_View` shows exactly `direction === 'stock_in'` entries, `Stock_Out_View` shows `direction === 'stock_out'` and `direction === null` entries
    - _Requirements: 8.4, 8.5, 8.6, 9.1, 9.2, 9.3, 9.4_

  - [x] 3.6 Write RTL tests for the view-scoped `Select_All_Control`
    - Assert unchecked/disabled state with no eligible entries in the Active_View, checked state once every eligible entry in the Active_View is selected, and that activating it toggles only eligible ids within the Active_View while leaving out-of-view and ineligible entries' selection untouched
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 10.1, 10.2, 10.3_

  - [x] 3.7 Write RTL tests for selection clearing on tab switch
    - Select an entry, switch tabs, switch back, and assert the batch selection is empty
    - _Requirements: 11.1, 11.2_

  - [x] 3.8 Write RTL test for view-scoped batch approval
    - Assert an approve action from `BatchReviewPanel` only ever includes entries from the Active_View
    - _Requirements: 10.5_

- [x] 4. Rename "commit" to "approve" across `ScanEntryCard` and `BatchReviewPanel`
  - [x] 4.1 Rename `ScanEntryCard`'s single-scan approve control and error copy
    - Rename internal state `committing`/`commitError` to `approving`/`approveError`
    - Change the button text from `Commit scan` to `Approve scan`, add `aria-label="Approve scan"`
    - Change the error fallback message from `'Unable to commit scan.'` to `'Unable to approve scan.'`
    - Keep the `commitScanEntry` API function name unchanged (backend/client detail, not UI copy)
    - _Requirements: 4.1, 4.2, 4.6_

  - [x] 4.2 Rename `BatchReviewPanel`'s heading, approve control, and error copy
    - Change the heading text from `Batch review` to `Approve scans`
    - Change the button text from `Commit {n} selected` to `Approve {n} selected`, add `aria-label={\`Approve ${selectedIds.length} selected scans\`}`
    - Change the error fallback message from `'Unable to commit selected scans.'` to `'Unable to approve selected scans.'`
    - _Requirements: 4.3, 4.4, 4.5, 4.6_

  - [x] 4.3 Write RTL tests for `ScanEntryCard`'s approve rename
    - Assert the button's visible text and `aria-label`, and the error message copy on a simulated failure
    - _Requirements: 4.1, 4.2, 4.6_

  - [x] 4.4 Write RTL tests for `BatchReviewPanel`'s approve rename
    - Assert the heading text, the button's visible text and `aria-label`, and the error message copy on a simulated failure
    - _Requirements: 4.3, 4.4, 4.5, 4.6_

- [x] 5. Add the remove control to `ScanEntryCard`
  - [x] 5.1 Implement the remove control
    - Show a `Button`/`ActionIcon` (subtle, red, `size="xs"`) when `entry.status === 'pending' || entry.status === 'flagged'`
    - On click, call `updateScanEntry(entry.id, { status: 'cancelled' })`, then `onChanged()` on success
    - On failure, show an inline error `Alert` and leave the entry displayed (no optimistic removal)
    - _Requirements: 1.1, 1.2, 1.4_

  - [x] 5.2 Write RTL tests for the remove control on `ScanEntryCard`
    - Assert the control is shown for `pending` and `flagged` entries, that activating it sends the `cancelled` PATCH, and that a failed PATCH shows an error and keeps the card rendered
    - _Requirements: 1.1, 1.2, 1.4_

  - [x] 5.3 Write an RTL test for the remove-then-reload flow on `ScanQueuePage`
    - Simulate a successful remove PATCH from a rendered card and assert the entry disappears from `ScanQueuePage`'s displayed list via the existing `onChanged`/`loadQueue` reload path
    - _Requirements: 1.3_

- [x] 6. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Add the unit-count editor to `ScanEntryCard`
  - [x] 7.1 Implement the increment and decrement controls
    - Show increment/decrement `Button`s and the current count when `entry.status === 'pending'`
    - Increment calls `updateScanEntry(entry.id, { unitCount: entry.unitCount + 1 })`; decrement calls the analogous `- 1` update; both call `onChanged()` on success
    - Disable the decrement control when `entry.unitCount <= 1`
    - _Requirements: 2.1, 2.2, 2.3, 2.4_

  - [x] 7.2 Implement the direct numeric input with draft state and validation
    - Add a `NumberInput` (matching the sizing convention in `SuggestionPanel`/`ShoppingListPage`) holding local draft state derived from `entry.unitCount`
    - On blur or Enter, if the draft parses to an integer and `isValidUnitCount(draft)` is true, PATCH `{ unitCount: draft }` and call `onChanged()`; otherwise reset the draft to `entry.unitCount` without sending a request
    - On PATCH failure, show an inline error and reset the draft to `entry.unitCount`
    - _Requirements: 2.1, 2.5, 2.6, 2.7_

  - [x] 7.3 Write RTL tests for increment and decrement
    - Assert the increment/decrement PATCH bodies, and that the decrement control is disabled when the unit count is 1
    - _Requirements: 2.1, 2.2, 2.3, 2.4_

  - [x] 7.4 Write RTL tests for the direct numeric input
    - Assert a valid confirmed value sends the expected PATCH, an invalid value (`< 1` or non-integer) is rejected client-side with the displayed count unchanged, and a failed PATCH shows an error while reverting to the previously confirmed count
    - _Requirements: 2.5, 2.6, 2.7_

- [x] 8. Add the per-card expiration date input to `ScanEntryCard`
  - [x] 8.1 Implement the expiration date input
    - Add a `TextInput type="date"` shown when `entry.status === 'pending'`, defaulting to `''` when `entry.expiresAt === null`
    - On blur, if the value changed, PATCH `updateScanEntry(entry.id, { expiresAt: expiryDateToISOString(value) })` and call `onChanged()` on success; on failure, show an inline error and reset the draft to the entry's current `expiresAt`
    - _Requirements: 7.1, 7.2, 7.3_

  - [x] 8.2 Write RTL tests for the expiration date input
    - Assert the input defaults to empty for a `null` `expiresAt`, that confirming a value sends the expected PATCH, and that a failed PATCH shows an error and resets the draft
    - _Requirements: 7.1, 7.2, 7.3_

- [x] 9. Update the `Batch_Selection_Checkbox` to be unlabeled and accessible
  - [x] 9.1 Remove the visible label and add an `aria-label`
    - Remove the `label` prop from the existing selection `Checkbox` and add `aria-label="Select scan for batch approval"`
    - _Requirements: 5.1, 5.2_

  - [x] 9.2 Write RTL tests for the accessible checkbox
    - Assert no visible label text is rendered, the `aria-label` is present, and toggling the checkbox reports the selection change through `onSelectedChange`
    - _Requirements: 5.1, 5.2, 5.3_

- [x] 10. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Add the motion-aware change indicator to `ScanEntryCard`
  - [x] 11.1 Implement `showChangeIndicator` state and CSS classes
    - Add the `useEffect` from the design keyed on `entry.unitCount` that sets `showChangeIndicator` true and clears it after 600ms
    - Add the `prefersReducedMotion` check via `matchMedia` and derive `changeIndicatorClass` (`scan-entry-card--changed-animated` vs `scan-entry-card--changed-static`), applying it to the card's `className`
    - Add `.scan-entry-card--changed-animated`, `.scan-entry-card--changed-static`, and the `scan-entry-changed-flash` keyframes to `frontend/src/index.css`
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

  - [x] 11.2 Write RTL tests for the change indicator
    - Using `vi.useFakeTimers()`/`vi.advanceTimersByTime(600)` (as in `ScanDirectionToggle.test.tsx`) and `vi.stubGlobal('matchMedia', ...)`, assert the indicator appears on mount and on a `unitCount` change, clears after ~600ms, and renders the static class instead of the animated one when reduced motion is preferred
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

- [x] 12. Strip shared controls from `BatchReviewPanel` and send a non-overriding batch approve request
  - [x] 12.1 Remove the direction/expiry controls and simplify the approve request
    - Remove the `NativeSelect` (direction) and `TextInput` (expiration date) controls and their state
    - Change `handleApprove`/`batchCommitScanEntries` to call with only `{ scanEntryIds: selectedIds, commit: true }`, omitting `direction`, `unitCount`, and `expiresAt`
    - _Requirements: 7.4, 7.5_

  - [x] 12.2 Update `BatchReviewPanel.test.tsx` for the stripped controls and new request body
    - Replace the existing test (which asserts the removed direction/expiry fields) with assertions that no direction/expiry controls are rendered and that the approve request body is exactly `{ scanEntryIds, commit: true }`
    - _Requirements: 7.4, 7.5_

- [x] 13. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate the three universal correctness properties in the design (`isValidUnitCount`, `toggleSelectAll`, `getEntriesForView`)
- Unit/RTL tests validate specific examples and edge cases per acceptance criterion
- `queueUtils.ts` is written by both 1.1 and 1.5 (independent pure-function groups), and `queueUtils.test.ts` is written by 1.2, 1.3, 1.4, and 1.6 in sequence; none of these have a logical dependency on each other beyond sharing a file, so they're spread across waves purely to avoid concurrent edits to the same file
- `ScanQueuePage.tsx` is written by 3.1 → 3.2 → 3.3 in order, since 3.2 and 3.3 both depend on `viewEntries` from 3.1; its test file `ScanQueuePage.test.tsx` is written by 5.3 (independent of task 3, gated only on 5.1) and then 3.4 → 3.5 → 3.6 → 3.7 → 3.8 once 3.3 lands, each in its own wave since they share a file
- `ScanEntryCard.tsx` and `ScanEntryCard.test.tsx` are each touched by several tasks in sequence (4 → 5 → 7 → 8 → 9 → 11); apply them in task order since each builds on the previous edit to the same file
- `BatchReviewPanel.tsx`/`BatchReviewPanel.test.tsx` are touched by 4.2/4.4 (rename) before 12.1/12.2 (control removal), since the rename lands first per the design's ordering

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "4.1", "4.2"] },
    { "id": 1, "tasks": ["1.2", "1.5", "4.3", "4.4", "5.1", "12.1"] },
    { "id": 2, "tasks": ["1.3", "3.1", "5.2", "5.3", "7.1", "12.2"] },
    { "id": 3, "tasks": ["1.4", "3.2", "7.2"] },
    { "id": 4, "tasks": ["1.6", "3.3", "7.3", "8.1"] },
    { "id": 5, "tasks": ["3.4", "7.4", "9.1"] },
    { "id": 6, "tasks": ["3.5", "8.2", "11.1"] },
    { "id": 7, "tasks": ["3.6", "9.2"] },
    { "id": 8, "tasks": ["3.7", "11.2"] },
    { "id": 9, "tasks": ["3.8"] }
  ]
}
```
