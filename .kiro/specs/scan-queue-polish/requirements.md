# Requirements Document

## Introduction

The scan queue at `frontend/src/components/queue/` (ScanQueuePage, ScanEntryCard, BatchReviewPanel) currently offers no way to remove an unwanted scan, no way to correct a mis-scanned unit count, and a batch-review workflow whose "Select for batch review" checkbox and "Commit" terminology add visual and semantic noise. Its select-all-equivalent behavior does not exist, and its expiration date is entered once for the whole batch rather than per scanned item. This feature adds a soft-cancel removal control and a unit-count editor to ScanEntryCard, adds a select-all control to ScanQueuePage that targets only batch-eligible entries, renames "commit" to "approve" throughout the queue UI, replaces the labeled batch-selection checkbox with an unlabeled, accessible one, adds a brief motion-aware visual indicator for unit-count changes and newly arrived cards, and moves the expiration date field from BatchReviewPanel (applied identically to every selected entry) to ScanEntryCard (set independently per entry). No backend endpoint is added; all changes route through the existing `PATCH /api/scans/{id}` (`updateScanEntry`) and `POST /api/scans/batch-commit` (`batchCommitScanEntries`) API functions in `frontend/src/api/client.ts`.

This feature also splits ScanQueuePage's single combined list into two mutually exclusive tabbed views, Stock_In_View and Stock_Out_View, so a user reviewing one direction is not visually distracted by the other. Every existing per-entry and batch capability described above (Select_All_Control, Batch_Selection_Checkbox, BatchReviewPanel's approve action, and per-card unit-count and expiration editing) continues to work exactly as specified, but now operates only against the scan entries displayed within whichever view is currently active. The Queue_Direction_Tabs control this adds to ScanQueuePage is distinct from the existing ScanDirectionToggle SegmentedControl (`frontend/src/components/scanner/ScanDirectionToggle.tsx`), which pre-selects the `direction` recorded on a *new* scan entry created by a barcode scan; the two controls coexist without conflict.

## Glossary

- **ScanQueuePage**: The existing frontend component at `frontend/src/components/queue/ScanQueuePage.tsx` that displays pending and flagged scan entries and holds the current batch selection.
- **ScanEntryCard**: The existing frontend component at `frontend/src/components/queue/ScanEntryCard.tsx` that displays and lets a user act on one scan entry.
- **BatchReviewPanel**: The existing frontend component at `frontend/src/components/queue/BatchReviewPanel.tsx` that approves every scan entry currently in the batch selection.
- **Scan_Entry**: An object matching the `ScanEntry` type in `frontend/src/types/index.ts`, with fields including `id`, `status`, `direction`, `unitCount`, and `expiresAt`.
- **Batch_Eligible_Entry**: A Scan_Entry displayed by ScanQueuePage whose `status` field equals `pending` and whose `direction` field is not null.
- **Select_All_Control**: The control this feature adds to ScanQueuePage that adds or removes Batch_Eligible_Entry ids from the current batch selection in one activation.
- **Batch_Selection_Checkbox**: The checkbox displayed on a ScanEntryCard for a scan entry with `status` equal to `pending` that adds or removes that entry's id from the current batch selection.
- **Change_Indicator**: The temporary visual treatment this feature adds to a ScanEntryCard to mark that its unit count just changed or that the card was just added to the displayed list.
- **Reduced_Motion_Preference**: The user's operating-system or browser setting exposed to the frontend through the `prefers-reduced-motion` CSS/JavaScript media feature.
- **Queue_Direction_Tabs**: The Mantine Tabs control this feature adds to ScanQueuePage, containing exactly two tabs, Stock_In_View and Stock_Out_View, of which exactly one is the Active_View at any time.
- **Stock_In_View**: The tab of Queue_Direction_Tabs that displays every scan entry whose `direction` field equals `stock_in`.
- **Stock_Out_View**: The tab of Queue_Direction_Tabs that displays every scan entry whose `direction` field equals `stock_out` and every scan entry whose `direction` field is null.
- **Active_View**: The Stock_In_View or the Stock_Out_View currently selected by Queue_Direction_Tabs and displayed by ScanQueuePage.

## Requirements

### Requirement 1: Remove a scan entry from the queue

**User Story:** As a user reviewing the scan queue, I want to remove a scan entry I don't want to process, so that it stops appearing in my queue.

#### Acceptance Criteria

1. THE ScanEntryCard SHALL display a remove control for a scan entry with a `status` field equal to `pending` or `flagged`.
2. WHEN a user activates the remove control on a ScanEntryCard, THE ScanEntryCard SHALL send a `PATCH /api/scans/{id}` request with the `status` field set to `cancelled`.
3. WHEN a `PATCH /api/scans/{id}` request setting `status` to `cancelled` succeeds, THE ScanQueuePage SHALL remove the corresponding scan entry from its displayed list.
4. IF a `PATCH /api/scans/{id}` request setting `status` to `cancelled` fails, THEN THE ScanEntryCard SHALL display an error message and SHALL retain the scan entry in the displayed list.

### Requirement 2: Edit the unit count on a scan entry card

**User Story:** As a user reviewing a scanned item, I want to increment, decrement, or directly edit its unit count, so that I can correct the count before approving it.

#### Acceptance Criteria

1. THE ScanEntryCard SHALL display an increment control, a decrement control, and a direct numeric input for the unit count of a scan entry with a `status` field equal to `pending`.
2. WHEN a user activates the increment control on a ScanEntryCard, THE ScanEntryCard SHALL send a `PATCH /api/scans/{id}` request with the `unitCount` field set to the displayed unit count plus one.
3. WHEN a user activates the decrement control on a ScanEntryCard while the displayed unit count is greater than one, THE ScanEntryCard SHALL send a `PATCH /api/scans/{id}` request with the `unitCount` field set to the displayed unit count minus one.
4. WHILE the displayed unit count of a scan entry equals one, THE ScanEntryCard SHALL disable the decrement control for that scan entry.
5. WHEN a user enters a value in the direct numeric input for unit count and confirms it, THE ScanEntryCard SHALL send a `PATCH /api/scans/{id}` request with the `unitCount` field set to the entered value.
6. IF a user confirms a value less than one in the direct numeric input for unit count, THEN THE ScanEntryCard SHALL reject the entered value and SHALL retain the previously displayed unit count.
7. IF a `PATCH /api/scans/{id}` request updating `unitCount` fails, THEN THE ScanEntryCard SHALL display an error message and SHALL retain the previously confirmed unit count.

### Requirement 3: Select all batch-eligible cards

**User Story:** As a user with several scanned items ready for batch approval, I want to select all of them at once, so that I don't have to check each card individually.

#### Acceptance Criteria

1. THE ScanQueuePage SHALL display a Select_All_Control alongside the displayed scan entries.
2. WHEN a user activates the Select_All_Control while at least one displayed Batch_Eligible_Entry is unselected, THE ScanQueuePage SHALL add every displayed Batch_Eligible_Entry's id to the current batch selection.
3. WHEN a user activates the Select_All_Control while every displayed Batch_Eligible_Entry is selected, THE ScanQueuePage SHALL remove every Batch_Eligible_Entry's id from the current batch selection.
4. WHEN a user activates the Select_All_Control, THE ScanQueuePage SHALL preserve the selection state of any displayed scan entry that is not a Batch_Eligible_Entry.

### Requirement 4: Rename "commit" to "approve" throughout the queue UI

**User Story:** As a user reviewing scans, I want the queue's controls and messages to consistently say "approve," so that the labeling matches the action I'm taking.

#### Acceptance Criteria

1. THE ScanEntryCard SHALL label the control that commits a single scan entry with text containing the word "approve" rather than "commit".
2. THE ScanEntryCard SHALL set the aria-label of the control that commits a single scan entry to text containing the word "approve" rather than "commit".
3. THE BatchReviewPanel SHALL label its heading with text containing the word "approve" rather than "commit".
4. THE BatchReviewPanel SHALL label the control that commits every selected scan entry with text containing the word "approve" rather than "commit".
5. THE BatchReviewPanel SHALL set the aria-label of the control that commits every selected scan entry to text containing the word "approve" rather than "commit".
6. IF a commit request sent by ScanEntryCard or BatchReviewPanel fails, THEN THE ScanEntryCard or BatchReviewPanel SHALL display an error message containing the word "approve" rather than "commit".

### Requirement 5: Unlabeled, accessible batch-selection checkbox

**User Story:** As a user scanning many items, I want the batch-selection checkbox to be compact and unlabeled, so that the card stays visually tidy without losing accessibility for screen reader users.

#### Acceptance Criteria

1. THE ScanEntryCard SHALL display the Batch_Selection_Checkbox for a scan entry with a `status` field equal to `pending` without visible label text.
2. THE ScanEntryCard SHALL set an aria-label on the Batch_Selection_Checkbox describing the checkbox's purpose.
3. WHEN a user activates the Batch_Selection_Checkbox, THE ScanQueuePage SHALL add or remove that scan entry's id from the current batch selection according to the checkbox's checked state.

### Requirement 6: Motion-aware change indicator

**User Story:** As a user scanning items in quick succession, I want a brief visual cue when a card's count changes or a new card appears, so that I can tell what changed without scanning the whole list, while still respecting my reduced-motion preference.

#### Acceptance Criteria

1. WHEN the unit count displayed on a ScanEntryCard changes, THE ScanEntryCard SHALL display a Change_Indicator on that card for approximately 600 milliseconds.
2. WHEN a scan entry is added to the list displayed by ScanQueuePage, THE ScanEntryCard rendered for that scan entry SHALL display a Change_Indicator for approximately 600 milliseconds.
3. THE ScanEntryCard SHALL stop displaying a Change_Indicator once approximately 600 milliseconds have elapsed since that Change_Indicator began being displayed.
4. WHERE the Reduced_Motion_Preference is enabled, THE ScanEntryCard SHALL display the Change_Indicator using a non-animated presentation instead of an animated fade or flash.

### Requirement 7: Per-card expiration date and non-overriding batch approval

**User Story:** As a user approving several scanned items together, I want to set each item's expiration date individually on its own card, so that items scanned together can carry different expiration dates without a shared field forcing them to match.

#### Acceptance Criteria

1. THE ScanEntryCard SHALL display an optional expiration date input for a scan entry with a `status` field equal to `pending`.
2. THE ScanEntryCard SHALL default the expiration date input to empty for a scan entry whose `expiresAt` field is null.
3. WHEN a user enters a value in the expiration date input on a ScanEntryCard and confirms it, THE ScanEntryCard SHALL send a `PATCH /api/scans/{id}` request with the `expiresAt` field set to the entered date.
4. THE BatchReviewPanel SHALL limit its displayed controls to the control that approves every selected scan entry, excluding any expiration date input.
5. WHEN a user activates the approve control on the BatchReviewPanel, THE BatchReviewPanel SHALL send a batch commit request that omits direction, unit count, and expiration date overrides, so that each selected scan entry is approved using its own existing direction, unit count, and expiration date.

### Requirement 8: Tabbed Stock In / Stock Out views

**User Story:** As a user reviewing the scan queue, I want stock-in and stock-out entries separated into their own tabs, so that only one direction's entries are visible at a time.

#### Acceptance Criteria

1. THE ScanQueuePage SHALL display Queue_Direction_Tabs containing a Stock_In_View tab and a Stock_Out_View tab.
2. WHEN ScanQueuePage mounts, THE ScanQueuePage SHALL set the Active_View to the Stock_Out_View.
3. THE ScanQueuePage SHALL set the Active_View to the Stock_Out_View on every mount of ScanQueuePage, regardless of which view was the Active_View during a prior mount.
4. WHEN a user activates the Stock_In_View tab, THE ScanQueuePage SHALL set the Active_View to the Stock_In_View.
5. WHEN a user activates the Stock_Out_View tab, THE ScanQueuePage SHALL set the Active_View to the Stock_Out_View.
6. THE ScanQueuePage SHALL display, at any time, only the scan entries belonging to the Active_View.

### Requirement 9: Direction-based entry filtering per view

**User Story:** As a user reviewing the scan queue, I want flagged entries with no assigned direction to show up alongside my stock-out items, so that I don't have to check a separate tab to find them.

#### Acceptance Criteria

1. WHILE the Stock_In_View is the Active_View, THE ScanQueuePage SHALL display every displayable scan entry whose `direction` field equals `stock_in` and SHALL exclude every displayable scan entry whose `direction` field does not equal `stock_in`.
2. WHILE the Stock_Out_View is the Active_View, THE ScanQueuePage SHALL display every displayable scan entry whose `direction` field equals `stock_out`.
3. WHILE the Stock_Out_View is the Active_View, THE ScanQueuePage SHALL display every displayable scan entry whose `direction` field is null.
4. WHILE the Stock_Out_View is the Active_View, THE ScanQueuePage SHALL exclude every displayable scan entry whose `direction` field equals `stock_in`.

### Requirement 10: View-scoped batch selection controls

**User Story:** As a user batch-approving items, I want the select-all control and batch approval to act only on the entries I'm currently looking at, so that I don't accidentally select or approve entries from the other direction.

#### Acceptance Criteria

1. WHEN a user activates the Select_All_Control while at least one Batch_Eligible_Entry within the Active_View is unselected, THE ScanQueuePage SHALL add every Batch_Eligible_Entry id within the Active_View to the current batch selection.
2. WHEN a user activates the Select_All_Control while every Batch_Eligible_Entry within the Active_View is selected, THE ScanQueuePage SHALL remove every Batch_Eligible_Entry id within the Active_View from the current batch selection.
3. WHEN a user activates the Select_All_Control, THE ScanQueuePage SHALL preserve the selection state of any scan entry that is not within the Active_View.
4. THE ScanQueuePage SHALL restrict the Batch_Selection_Checkbox, per-card unit-count editing, and per-card expiration date editing described in Requirements 2, 5, and 7 to scan entries within the Active_View.
5. WHEN a user activates the approve control on the BatchReviewPanel, THE BatchReviewPanel SHALL approve only the scan entries in the current batch selection that are within the Active_View.

### Requirement 11: Selection clears on view switch

**User Story:** As a user switching between stock-in and stock-out tabs, I want my batch selection to reset, so that I don't accidentally approve entries I selected in the other tab without reviewing them there.

#### Acceptance Criteria

1. WHEN a user activates the Stock_In_View tab while the Stock_Out_View is the Active_View, THE ScanQueuePage SHALL clear the current batch selection.
2. WHEN a user activates the Stock_Out_View tab while the Stock_In_View is the Active_View, THE ScanQueuePage SHALL clear the current batch selection.
