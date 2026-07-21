# Requirements Document

## Introduction

The Pantry Management app helps users track their household pantry inventory. It provides item scanning via barcode, inventory level tracking with smart suggestions for target quantities, and shopping list management to help users restock to their desired inventory levels. Each physical unit of a product is tracked as a separate instance with its own expiration date, enabling accurate use-oldest-first consumption ordering and expiration warnings. The app may also integrate with online shopping carts to streamline restocking.

## Glossary

- **App**: The Pantry Management application.
- **User**: A person managing their household pantry inventory.
- **Item**: A pantry product type identified by barcode, name, or category. An Item describes what the product is, not how many physical units are on hand.
- **Item instance**: A single physical unit of an Item in the pantry. Each item instance has its own expiration date and stock-in timestamp, and is tracked independently of other instances of the same Item.
- **Inventory**: The current collection of item instances tracked by the app, grouped by Item.
- **Target quantity**: The desired number of item instances for each Item that the user wants to maintain.
- **Shopping list**: A list of Items and quantities the user needs to purchase to reach their target quantity.
- **Scan queue**: A list of pending scan entries that have been captured but not yet committed to the Inventory; each entry holds the barcode, timestamp, optional pre-selected scan direction, and expiration date.
- **Scan entry**: A single item in the scan queue, containing a barcode value, timestamp, optional scan direction, unit count, optional expiration date, and resolution status. When committed as a stock-in, a scan entry with a unit count of N results in N new item instances, all sharing the same expiration date.
- **Scan direction**: The user-declared direction of a scan: stock-in (item entering the pantry) or stock-out (item leaving the pantry).
- **Pending scan**: A scan entry whose scan direction has not yet been confirmed by the user.
- **Flagged entry**: A scan entry whose barcode does not match any known product and requires the user to identify or create the product.
- **Product override**: A user-defined mapping that associates a specific barcode with a specific product within the user's pantry context, taking precedence over any global product database match.
- **Use-oldest-first order**: The ordering of item instances by expiration date ascending (soonest to expire first), used when suggesting which instance to consume next.
- **Expiry warning period**: The number of days before an item instance's expiration date at which the app considers the instance "near expiry". The default value is 7 days.

## Requirements

### Requirement 1: Barcode Scanning

**User Story:** As a user, I want to scan item barcodes and later review each scan to confirm whether it was a stock-in or stock-out, specify expiration dates when stocking in, and choose which instance to remove when stocking out, so that I can capture per-instance inventory changes quickly during real-world activity and resolve the details at a convenient time.

#### Acceptance Criteria

**Scanning**

1. WHEN the user scans a barcode, THE App SHALL read the barcode from the device camera or connected input device and add a new scan entry to the scan queue without requiring any mode selection beforehand.
2. WHEN a scan entry is added to the scan queue, THE App SHALL record the barcode value and the timestamp of the scan.
3. IF a barcode cannot be read, THEN THE App SHALL present the user with a manual barcode entry option.
4. WHERE the user has pre-selected a scan direction before scanning, WHEN a barcode is successfully read, THE App SHALL record that scan direction on the resulting scan entry and display it as the default during review.
5. WHEN the user pre-selects a scan direction, THE App SHALL apply that scan direction to all subsequent scan entries until the user clears or changes it, or until 5 minutes have elapsed since the last scan was recorded, whichever occurs first.

**Scan Queue Review**

6. THE App SHALL display all pending scans in the scan queue, showing each entry's product name (if known), barcode, timestamp, current scan direction (if pre-set), unit count, and expiration date (if provided).
7. WHEN the user opens the scan queue, THE App SHALL present pending scans in chronological order with the oldest entry first.
8. WHEN the user reviews a pending scan with scan direction stock-in, THE App SHALL allow the user to set or change the scan direction, adjust the unit count, and enter an expiration date for the batch before committing.
9. WHEN the user reviews a pending scan with scan direction stock-out, THE App SHALL present the available item instances for that product sorted in use-oldest-first order and allow the user to select a different instance before committing.
10. THE App SHALL support batch review, allowing the user to apply the same scan direction and expiration date to multiple selected pending scans simultaneously.
11. WHEN the user commits a scan entry with scan direction stock-in and a unit count of N, THE App SHALL create N new item instances for the corresponding Item, each with the stock-in timestamp and the expiration date from the scan entry.
12. WHEN the user commits a scan entry with scan direction stock-out, THE App SHALL remove the selected item instance from the Inventory; IF the user has not selected a specific instance, THEN THE App SHALL remove the item instance with the earliest expiration date (use-oldest-first order).
13. IF committing a stock-out scan entry would remove the last item instance of an Item, THEN THE App SHALL warn the user before completing the removal.
14. WHEN a scan entry is committed, THE App SHALL remove it from the pending scans list and retain it in a read-only scan history.

**Unrecognized Barcodes**

15. IF a barcode is read but no matching product is found in the product database or the user's product overrides, THEN THE App SHALL add a flagged entry to the scan queue and display a visual indicator distinguishing it from standard pending scans.
16. WHEN the user reviews a flagged entry, THE App SHALL allow the user to search for and select an existing product, or enter new product details to create a new product record.
17. WHEN the user resolves a flagged entry by selecting or creating a product, THE App SHALL associate that product with the barcode as a product override and convert the flagged entry to a pending scan for normal commit review.

**Barcode Conflicts**

18. IF a barcode matches more than one product (including both database entries and product overrides), THEN THE App SHALL present the user with a disambiguation prompt listing the matching products before adding the scan entry to the scan queue.
19. WHEN the user selects a product during disambiguation, THE App SHALL offer the user the option to save that choice as a product override so that future scans of the same barcode resolve automatically without prompting.

### Requirement 2: Inventory Tracking

**User Story:** As a user, I want to view and manage my current pantry inventory at both the product level and the individual instance level, so that I know what I have on hand, can track expiration dates per unit, and receive warnings before items expire.

#### Acceptance Criteria

**Inventory Display**

1. THE App SHALL maintain a persistent record of all item instances, including each instance's Item, stock-in timestamp, and expiration date.
2. WHEN the user views the Inventory, THE App SHALL display each Item's name, category, unit of measure, and the total count of its item instances currently in the pantry.
3. WHEN the user selects an Item in the Inventory view, THE App SHALL display all item instances for that Item, each showing its stock-in timestamp and expiration date, sorted in use-oldest-first order.
4. THE App SHALL allow the user to search and filter Inventory items by name or category.

**Instance Management**

5. WHEN the user manually adds an item instance, THE App SHALL create a new instance with the user-provided expiration date and record the current time as the stock-in timestamp.
6. WHEN the user manually removes an item instance, THE App SHALL delete that specific instance's record.
7. IF the user attempts to remove an item instance that does not exist, THEN THE App SHALL reject the operation and display an error message.

**Expiration Tracking**

8. WHILE an item instance's expiration date is within the expiry warning period, THE App SHALL display a visual warning indicator on that instance and on its Item in the Inventory view.
9. WHILE an item instance's expiration date is in the past, THE App SHALL display a distinct expired indicator on that instance and on its Item in the Inventory view.
10. WHEN the user views the Inventory, THE App SHALL list any Item with at least one near-expiry or expired item instance in a dedicated "Needs Attention" section at the top of the Inventory view.
11. WHEN the Inventory is updated, THE App SHALL re-evaluate expiration status for all affected item instances and update the "Needs Attention" section.

### Requirement 3: Inventory Size Suggestions

**User Story:** As a user, I want the app to suggest target inventory levels for my items, so that I can set appropriate restocking thresholds without guessing.

#### Acceptance Criteria

1. WHEN the user requests suggestions for an Item's target quantity, THE App SHALL analyze the Item's instance consumption history and generate a recommended target instance count.
2. WHEN a suggestion is produced, THE App SHALL display the suggested target instance count and the reasoning behind it.
3. WHEN the user accepts a suggestion, THE App SHALL save the suggested value as the Item's target quantity.
4. WHEN the user declines a suggestion, THE App SHALL allow the user to enter a custom target quantity.
5. IF an Item has fewer than 3 recorded consumption events, THEN THE App SHALL indicate that insufficient data exists and SHALL prompt the user to set a manual target.
6. THE App SHALL recalculate suggestions when an Item's consumption history changes significantly.

### Requirement 4: Shopping List and Target Inventory Maintenance

**User Story:** As a user, I want the app to generate a shopping list based on my target inventory levels, so that I can efficiently restock my pantry to the instance counts I want to maintain.

#### Acceptance Criteria

1. THE App SHALL generate a shopping list by comparing each Item's current item instance count against its target quantity.
2. WHEN an Item's current item instance count falls below its target quantity, THE App SHALL add the Item to the shopping list with a purchase quantity equal to the difference between the target quantity and the current item instance count.
3. WHEN the user views the shopping list, THE App SHALL display each Item's name, required purchase quantity, and unit of measure.
4. WHEN the user manually adds an Item to the shopping list, THE App SHALL include the Item with the user-specified quantity.
5. WHEN the user removes an Item from the shopping list, THE App SHALL remove the Item without modifying the Item's target quantity.
6. WHEN the user marks a shopping list item as purchased, THE App SHALL mark the item as purchased and remove it from the active shopping list without modifying the Inventory.
7. IF an Item on the shopping list does not have a target quantity set, THEN THE App SHALL display the Item with its manually specified quantity and SHALL prompt the user to set a target quantity for future automation.
8. THE App SHALL refresh the shopping list each time the Inventory is updated.
9. WHERE online cart integration is enabled, WHEN the user initiates cart export, THE App SHALL submit the shopping list items to the configured online shopping cart service.
10. WHERE online cart integration is enabled, IF the cart export fails to submit one or more items, THEN THE App SHALL notify the user of the failed items and SHALL retain those items in the shopping list.
