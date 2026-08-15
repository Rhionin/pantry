package inventory_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
	"github.com/google/uuid"
	"pgregory.net/rapid"
)

// Feature: pantry-management, Property 10: Needs Attention section contains exactly the right items
// For any inventory state, the Needs Attention section SHALL contain exactly those items
// that have at least one instance with status near_expiry or expired, and SHALL contain
// no items that have only ok instances.
//
// Validates: Requirements 2.10, 2.11
func TestProperty10_NeedsAttentionExactMatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, productRepo, _ := newTestRepo(t)
		ctx := context.Background()
		userID := uuid.NewString()
		now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		warningDays := 7

		numItems := rapid.IntRange(0, 10).Draw(rt, "numItems")

		type itemInfo struct {
			item                *inventory.Item
			hasNearExpiry       bool
			hasExpired          bool
			shouldNeedAttention bool
		}

		itemsCreated := make([]itemInfo, 0, numItems)

		for i := 0; i < numItems; i++ {
			prodID := uuid.NewString()
			prodName := rapid.StringMatching(`[A-Z][a-z]+`).Draw(rt, "prodName")
			category := rapid.StringMatching(`[A-Z][a-z]+`).Draw(rt, "category")

			err := productRepo.CreateProduct(ctx, product.Product{
				ID:            prodID,
				Name:          prodName,
				Category:      category,
				UnitOfMeasure: "unit",
			})
			if err != nil {
				rt.Fatalf("CreateProduct failed: %v", err)
			}

			item, err := repo.GetOrCreateItem(ctx, userID, prodID)
			if err != nil {
				rt.Fatalf("GetOrCreateItem failed: %v", err)
			}

			numInstances := rapid.IntRange(0, 5).Draw(rt, "numInstances")
			hasNearExpiry := false
			hasExpired := false

			for j := 0; j < numInstances; j++ {
				expiryType := rapid.IntRange(0, 3).Draw(rt, "expiryType")

				var expiresAt *time.Time
				switch expiryType {
				case 0: // OK - beyond warning window
					future := now.Add(time.Duration(warningDays+1+rapid.IntRange(1, 30).Draw(rt, "okDays")) * 24 * time.Hour)
					expiresAt = &future
				case 1: // Near expiry (within warning window)
					daysUntil := rapid.IntRange(0, warningDays).Draw(rt, "nearExpiryDays")
					nearExpiry := now.Add(time.Duration(daysUntil) * 24 * time.Hour)
					expiresAt = &nearExpiry
					hasNearExpiry = true
				case 2: // Expired
					daysAgo := rapid.IntRange(1, 30).Draw(rt, "expiredDays")
					expired := now.Add(-time.Duration(daysAgo) * 24 * time.Hour)
					expiresAt = &expired
					hasExpired = true
				case 3: // No expiry date
					expiresAt = nil
				}

				_, err := repo.AddInstance(ctx, inventory.ItemInstance{
					ItemID:    item.ID,
					StockInAt: now.Add(-24 * time.Hour),
					ExpiresAt: expiresAt,
				})
				if err != nil {
					rt.Fatalf("AddInstance failed: %v", err)
				}
			}

			itemsCreated = append(itemsCreated, itemInfo{
				item:                item,
				hasNearExpiry:       hasNearExpiry,
				hasExpired:          hasExpired,
				shouldNeedAttention: hasNearExpiry || hasExpired,
			})
		}

		invItems, err := repo.GetInventoryList(ctx, userID, now, warningDays, "")
		if err != nil {
			rt.Fatalf("GetInventoryList failed: %v", err)
		}

		needsAttentionMap := make(map[string]bool)
		for _, invItem := range invItems {
			needsAttentionMap[invItem.Item.ID] = invItem.NeedsAttention
		}

		for _, info := range itemsCreated {
			actual, found := needsAttentionMap[info.item.ID]
			if !found {
				rt.Fatalf("item %s not found in inventory list", info.item.ID)
			}
			if actual != info.shouldNeedAttention {
				rt.Fatalf("item %q: expected NeedsAttention=%v (nearExpiry=%v, expired=%v), got %v",
					info.item.Product.Name, info.shouldNeedAttention,
					info.hasNearExpiry, info.hasExpired, actual)
			}
		}
	})
}

// Feature: pantry-management, Property 11: Inventory search filter returns exactly matching items
// For any search query string and inventory state, the filtered result SHALL contain every item
// whose name or category includes the query string (case-insensitive), and SHALL exclude every
// item whose name and category both do not include the query string.
//
// Validates: Requirements 2.4
func TestProperty11_SearchFilterExactMatch(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		repo, productRepo, _ := newTestRepo(t)
		ctx := context.Background()
		userID := uuid.NewString()
		now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		warningDays := 7

		nameFragments := []string{"Milk", "Bread", "Cheese", "Butter", "Yogurt", "Eggs", "Rice", "Pasta"}
		categoryFragments := []string{"Dairy", "Bakery", "Grain", "Protein", "Vegetable"}

		numItems := rapid.IntRange(1, 15).Draw(rt, "numItems")

		type itemData struct {
			name     string
			category string
			itemID   string
		}
		items := make([]itemData, 0, numItems)

		for i := 0; i < numItems; i++ {
			name := nameFragments[rapid.IntRange(0, len(nameFragments)-1).Draw(rt, "nameIdx")]
			category := categoryFragments[rapid.IntRange(0, len(categoryFragments)-1).Draw(rt, "categoryIdx")]

			prodID := uuid.NewString()
			err := productRepo.CreateProduct(ctx, product.Product{
				ID:            prodID,
				Name:          name,
				Category:      category,
				UnitOfMeasure: "unit",
			})
			if err != nil {
				rt.Fatalf("CreateProduct failed: %v", err)
			}

			item, err := repo.GetOrCreateItem(ctx, userID, prodID)
			if err != nil {
				rt.Fatalf("GetOrCreateItem failed: %v", err)
			}

			items = append(items, itemData{name: name, category: category, itemID: item.ID})
		}

		// Generate a query: empty, full word, or partial substring, with randomized case
		var query string
		switch rapid.IntRange(0, 3).Draw(rt, "queryType") {
		case 0: // empty — matches everything
			query = ""
		case 1: // full name fragment
			query = randomizeCase(nameFragments[rapid.IntRange(0, len(nameFragments)-1).Draw(rt, "queryNameIdx")], rt)
		case 2: // full category fragment
			query = randomizeCase(categoryFragments[rapid.IntRange(0, len(categoryFragments)-1).Draw(rt, "queryCatIdx")], rt)
		case 3: // partial substring of a word
			allWords := append(nameFragments, categoryFragments...)
			word := allWords[rapid.IntRange(0, len(allWords)-1).Draw(rt, "wordIdx")]
			if len(word) > 2 {
				start := rapid.IntRange(0, len(word)-2).Draw(rt, "substringStart")
				end := rapid.IntRange(start+1, len(word)).Draw(rt, "substringEnd")
				query = randomizeCase(word[start:end], rt)
			} else {
				query = randomizeCase(word, rt)
			}
		}

		invItems, err := repo.GetInventoryList(ctx, userID, now, warningDays, query)
		if err != nil {
			rt.Fatalf("GetInventoryList failed: %v", err)
		}

		returnedIDs := make(map[string]bool)
		for _, invItem := range invItems {
			returnedIDs[invItem.Item.ID] = true
		}

		lowerQuery := strings.ToLower(query)
		for _, item := range items {
			shouldMatch := query == "" ||
				strings.Contains(strings.ToLower(item.name), lowerQuery) ||
				strings.Contains(strings.ToLower(item.category), lowerQuery)
			isReturned := returnedIDs[item.itemID]

			if shouldMatch && !isReturned {
				rt.Fatalf("query %q: item %q (category %q) should match but was not returned",
					query, item.name, item.category)
			}
			if !shouldMatch && isReturned {
				rt.Fatalf("query %q: item %q (category %q) should not match but was returned",
					query, item.name, item.category)
			}
		}
	})
}

// randomizeCase randomly uppercases or lowercases each rune in s.
func randomizeCase(s string, rt *rapid.T) string {
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if rapid.Bool().Draw(rt, "upperCase") {
			result = append(result, []rune(strings.ToUpper(string(r)))...)
		} else {
			result = append(result, []rune(strings.ToLower(string(r)))...)
		}
	}
	return string(result)
}
