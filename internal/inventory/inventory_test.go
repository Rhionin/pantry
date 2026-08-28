package inventory_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Rhionin/pantry/internal/app"
	"github.com/Rhionin/pantry/internal/inventory"
	"github.com/Rhionin/pantry/internal/product"
)

// newTestPantry opens an in-memory SQLite database, applies all migrations,
// and returns an inventory Pantry.
func newTestPantry(t *testing.T) (*inventory.Pantry, *product.Catalog, *sql.DB) {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory SQLite: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := app.RunMigrations(conn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return inventory.NewPantry(conn), product.NewCatalog(conn), conn
}

// createTestProduct is a helper that creates a product for testing.
func createTestProduct(t *testing.T, catalog *product.Catalog, ctx context.Context, id, name string) {
	t.Helper()
	p := product.Product{ID: id, Name: name, Category: "Test"}
	if err := catalog.CreateProduct(ctx, p); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
}

// --------------------------------------------------------------------------
// TestGetOrCreateItem
// --------------------------------------------------------------------------

func TestGetOrCreateItem(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		productID string
		setup     func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context)
		wantNew   bool // true if we expect a new item to be created
	}{
		{
			name:      "creates new item when none exists",
			userID:    "user-1",
			productID: "prod-1",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) {
				createTestProduct(t, catalog, ctx, "prod-1", "Milk")
			},
			wantNew: true,
		},
		{
			name:      "returns existing item",
			userID:    "user-1",
			productID: "prod-1",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) {
				createTestProduct(t, catalog, ctx, "prod-1", "Milk")
				// Create item first
				_, err := pantry.GetOrCreateItem(ctx, "user-1", "prod-1")
				if err != nil {
					t.Fatalf("GetOrCreateItem setup: %v", err)
				}
			},
			wantNew: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pantry, catalog, _ := newTestPantry(t)
			ctx := context.Background()
			tt.setup(t, pantry, catalog, ctx)

			// First call
			item1, err := pantry.GetOrCreateItem(ctx, tt.userID, tt.productID)
			if err != nil {
				t.Fatalf("GetOrCreateItem (first): %v", err)
			}
			if item1 == nil {
				t.Fatal("expected item, got nil")
			}
			if item1.UserID != tt.userID {
				t.Errorf("UserID: want %q, got %q", tt.userID, item1.UserID)
			}
			if item1.ProductID != tt.productID {
				t.Errorf("ProductID: want %q, got %q", tt.productID, item1.ProductID)
			}

			// Second call should return the same item
			item2, err := pantry.GetOrCreateItem(ctx, tt.userID, tt.productID)
			if err != nil {
				t.Fatalf("GetOrCreateItem (second): %v", err)
			}
			if item2 == nil {
				t.Fatal("expected item, got nil")
			}
			if item1.ID != item2.ID {
				t.Errorf("expected same item ID, got %q then %q", item1.ID, item2.ID)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestListItems
// --------------------------------------------------------------------------

func TestListItems(t *testing.T) {
	tests := []struct {
		name        string
		userID      string
		setup       func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context)
		expectCount int
		expectOrder []string // product names in expected order
	}{
		{
			name:        "empty inventory",
			userID:      "user-1",
			setup:       func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) {},
			expectCount: 0,
			expectOrder: []string{},
		},
		{
			name:   "single item",
			userID: "user-1",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) {
				createTestProduct(t, catalog, ctx, "p1", "Apple Juice")
				_, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem: %v", err)
				}
			},
			expectCount: 1,
			expectOrder: []string{"Apple Juice"},
		},
		{
			name:   "multiple items ordered by product name",
			userID: "user-1",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) {
				createTestProduct(t, catalog, ctx, "p1", "Cheese")
				createTestProduct(t, catalog, ctx, "p2", "Apple Juice")
				createTestProduct(t, catalog, ctx, "p3", "Butter")

				for _, pid := range []string{"p1", "p2", "p3"} {
					_, err := pantry.GetOrCreateItem(ctx, "user-1", pid)
					if err != nil {
						t.Fatalf("GetOrCreateItem: %v", err)
					}
				}
			},
			expectCount: 3,
			expectOrder: []string{"Apple Juice", "Butter", "Cheese"},
		},
		{
			name:   "filters by user",
			userID: "user-1",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) {
				createTestProduct(t, catalog, ctx, "p1", "Milk")
				createTestProduct(t, catalog, ctx, "p2", "Bread")

				// Create items for user-1
				_, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem user-1: %v", err)
				}

				// Create items for user-2 (should not be returned)
				_, err = pantry.GetOrCreateItem(ctx, "user-2", "p2")
				if err != nil {
					t.Fatalf("GetOrCreateItem user-2: %v", err)
				}
			},
			expectCount: 1,
			expectOrder: []string{"Milk"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pantry, catalog, _ := newTestPantry(t)
			ctx := context.Background()
			tt.setup(t, pantry, catalog, ctx)

			items, err := pantry.ListItems(ctx, tt.userID)
			if err != nil {
				t.Fatalf("ListItems: %v", err)
			}
			if len(items) != tt.expectCount {
				t.Fatalf("expected %d items, got %d", tt.expectCount, len(items))
			}

			// Verify order by product name
			for i, wantName := range tt.expectOrder {
				if items[i].Product.Name != wantName {
					t.Errorf("[%d] Product.Name: want %q, got %q", i, wantName, items[i].Product.Name)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestListItemInstances
// --------------------------------------------------------------------------

func TestListItemInstances(t *testing.T) {
	now := time.Now()
	tomorrow := now.Add(24 * time.Hour)
	nextWeek := now.Add(7 * 24 * time.Hour)

	tests := []struct {
		name              string
		setup             func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string // returns itemID
		expectCount       int
		expectExpiryOrder []bool // true if expires_at is not nil, in expected order
	}{
		{
			name: "empty",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				createTestProduct(t, catalog, ctx, "p1", "Milk")
				item, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem: %v", err)
				}
				return item.ID
			},
			expectCount:       0,
			expectExpiryOrder: []bool{},
		},
		{
			name: "single instance",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				createTestProduct(t, catalog, ctx, "p1", "Milk")
				item, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem: %v", err)
				}

				_, err = pantry.AddInstance(ctx, inventory.ItemInstance{
					ItemID:    item.ID,
					StockInAt: now,
					ExpiresAt: &tomorrow,
				})
				if err != nil {
					t.Fatalf("AddInstance: %v", err)
				}
				return item.ID
			},
			expectCount:       1,
			expectExpiryOrder: []bool{true},
		},
		{
			name: "multiple instances ordered by expiry (use-oldest-first)",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				createTestProduct(t, catalog, ctx, "p1", "Milk")
				item, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem: %v", err)
				}

				// Add instances in random order
				instances := []inventory.ItemInstance{
					{ItemID: item.ID, StockInAt: now, ExpiresAt: &nextWeek},
					{ItemID: item.ID, StockInAt: now, ExpiresAt: &tomorrow},
					{ItemID: item.ID, StockInAt: now, ExpiresAt: nil}, // no expiry should come last
				}

				for _, inst := range instances {
					_, err = pantry.AddInstance(ctx, inst)
					if err != nil {
						t.Fatalf("AddInstance: %v", err)
					}
				}
				return item.ID
			},
			expectCount:       3,
			expectExpiryOrder: []bool{true, true, false}, // sorted by expiry, nulls last
		},
		{
			name: "excludes removed instances",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				createTestProduct(t, catalog, ctx, "p1", "Milk")
				item, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem: %v", err)
				}

				// Add two instances
				inst1, err := pantry.AddInstance(ctx, inventory.ItemInstance{
					ItemID:    item.ID,
					StockInAt: now,
				})
				if err != nil {
					t.Fatalf("AddInstance 1: %v", err)
				}

				_, err = pantry.AddInstance(ctx, inventory.ItemInstance{
					ItemID:    item.ID,
					StockInAt: now,
				})
				if err != nil {
					t.Fatalf("AddInstance 2: %v", err)
				}

				// Remove the first instance
				err = pantry.RemoveInstance(ctx, inst1.ID, "manual")
				if err != nil {
					t.Fatalf("RemoveInstance: %v", err)
				}

				return item.ID
			},
			expectCount:       1, // only one remaining
			expectExpiryOrder: []bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pantry, catalog, _ := newTestPantry(t)
			ctx := context.Background()
			itemID := tt.setup(t, pantry, catalog, ctx)

			instances, err := pantry.ListItemInstances(ctx, itemID)
			if err != nil {
				t.Fatalf("ListItemInstances: %v", err)
			}
			if len(instances) != tt.expectCount {
				t.Fatalf("expected %d instances, got %d", tt.expectCount, len(instances))
			}

			// Verify expiry order
			for i, wantHasExpiry := range tt.expectExpiryOrder {
				hasExpiry := instances[i].ExpiresAt != nil
				if hasExpiry != wantHasExpiry {
					t.Errorf("[%d] ExpiresAt != nil: want %v, got %v", i, wantHasExpiry, hasExpiry)
				}
			}

			// Verify instances are sorted by expiry date ascending
			if len(instances) > 1 {
				for i := 0; i < len(instances)-1; i++ {
					curr := instances[i]
					next := instances[i+1]

					// If current has no expiry, all following should have no expiry
					if curr.ExpiresAt == nil && next.ExpiresAt != nil {
						t.Errorf("instances not sorted correctly: nil expiry should come after non-nil")
					}

					// If both have expiry, current should be <= next
					if curr.ExpiresAt != nil && next.ExpiresAt != nil {
						if curr.ExpiresAt.After(*next.ExpiresAt) {
							t.Errorf("instances not sorted correctly: %v > %v", curr.ExpiresAt, next.ExpiresAt)
						}
					}
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestAddInstance
// --------------------------------------------------------------------------

func TestAddInstance(t *testing.T) {
	now := time.Now()
	tomorrow := now.Add(24 * time.Hour)

	tests := []struct {
		name          string
		instance      inventory.ItemInstance
		wantID        bool // true if we expect ID to be generated
		wantExpiresAt bool // true if we expect ExpiresAt to be set
	}{
		{
			name: "with explicit ID and expiry",
			instance: inventory.ItemInstance{
				ID:        "inst-1",
				StockInAt: now,
				ExpiresAt: &tomorrow,
			},
			wantID:        false,
			wantExpiresAt: true,
		},
		{
			name: "generate UUID when ID empty",
			instance: inventory.ItemInstance{
				StockInAt: now,
			},
			wantID:        true,
			wantExpiresAt: false,
		},
		{
			name: "with no expiry date",
			instance: inventory.ItemInstance{
				StockInAt: now,
				ExpiresAt: nil,
			},
			wantID:        true,
			wantExpiresAt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pantry, catalog, _ := newTestPantry(t)
			ctx := context.Background()

			// Setup: create product and item
			createTestProduct(t, catalog, ctx, "p1", "Milk")
			item, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
			if err != nil {
				t.Fatalf("GetOrCreateItem: %v", err)
			}

			tt.instance.ItemID = item.ID

			// Add instance
			added, err := pantry.AddInstance(ctx, tt.instance)
			if err != nil {
				t.Fatalf("AddInstance: %v", err)
			}
			if added == nil {
				t.Fatal("expected instance, got nil")
			}

			// Verify ID
			if tt.wantID && added.ID == "" {
				t.Error("expected generated UUID, got empty string")
			}
			if !tt.wantID && added.ID != tt.instance.ID {
				t.Errorf("ID: want %q, got %q", tt.instance.ID, added.ID)
			}

			// Verify ItemID
			if added.ItemID != item.ID {
				t.Errorf("ItemID: want %q, got %q", item.ID, added.ItemID)
			}

			// Verify ExpiresAt
			if tt.wantExpiresAt && added.ExpiresAt == nil {
				t.Error("expected ExpiresAt to be set, got nil")
			}
			if !tt.wantExpiresAt && added.ExpiresAt != nil {
				t.Errorf("expected ExpiresAt to be nil, got %v", added.ExpiresAt)
			}

			// Roundtrip: verify GetInstance retrieves the same instance
			retrieved, err := pantry.GetInstance(ctx, added.ID)
			if err != nil {
				t.Fatalf("GetInstance: %v", err)
			}
			if retrieved == nil {
				t.Fatal("expected instance, got nil")
			}
			if retrieved.ID != added.ID {
				t.Errorf("ID: want %q, got %q", added.ID, retrieved.ID)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestRemoveInstance
// --------------------------------------------------------------------------

func TestRemoveInstance(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		setup       func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string // returns instanceID
		reason      string
		expectError error
	}{
		{
			name: "successfully removes existing instance",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				createTestProduct(t, catalog, ctx, "p1", "Milk")
				item, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem: %v", err)
				}
				inst, err := pantry.AddInstance(ctx, inventory.ItemInstance{
					ItemID:    item.ID,
					StockInAt: now,
				})
				if err != nil {
					t.Fatalf("AddInstance: %v", err)
				}
				return inst.ID
			},
			reason:      "manual",
			expectError: nil,
		},
		{
			name: "returns error for non-existent instance",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				return "non-existent-id"
			},
			reason:      "manual",
			expectError: inventory.ErrInstanceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pantry, catalog, _ := newTestPantry(t)
			ctx := context.Background()
			instanceID := tt.setup(t, pantry, catalog, ctx)

			err := pantry.RemoveInstance(ctx, instanceID, tt.reason)

			if tt.expectError != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.expectError)
				}
				if !errors.Is(err, tt.expectError) {
					t.Fatalf("expected error %v, got %v", tt.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("RemoveInstance: %v", err)
			}

			// Verify instance is marked as removed
			inst, err := pantry.GetInstance(ctx, instanceID)
			if err != nil {
				t.Fatalf("GetInstance: %v", err)
			}
			if inst == nil {
				t.Fatal("expected instance, got nil")
			}
			if inst.RemovedAt == nil {
				t.Error("expected RemovedAt to be set, got nil")
			}
			if inst.RemovalReason == nil || *inst.RemovalReason != tt.reason {
				t.Errorf("RemovalReason: want %q, got %v", tt.reason, inst.RemovalReason)
			}
		})
	}
}

// --------------------------------------------------------------------------
// TestGetInstance
// --------------------------------------------------------------------------

func TestGetInstance(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		setup       func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string // returns instanceID
		expectFound bool
	}{
		{
			name: "found",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				createTestProduct(t, catalog, ctx, "p1", "Milk")
				item, err := pantry.GetOrCreateItem(ctx, "user-1", "p1")
				if err != nil {
					t.Fatalf("GetOrCreateItem: %v", err)
				}
				inst, err := pantry.AddInstance(ctx, inventory.ItemInstance{
					ItemID:    item.ID,
					StockInAt: now,
				})
				if err != nil {
					t.Fatalf("AddInstance: %v", err)
				}
				return inst.ID
			},
			expectFound: true,
		},
		{
			name: "not found",
			setup: func(t *testing.T, pantry *inventory.Pantry, catalog *product.Catalog, ctx context.Context) string {
				return "non-existent-id"
			},
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pantry, catalog, _ := newTestPantry(t)
			ctx := context.Background()
			instanceID := tt.setup(t, pantry, catalog, ctx)

			inst, err := pantry.GetInstance(ctx, instanceID)
			if err != nil {
				t.Fatalf("GetInstance: %v", err)
			}

			if tt.expectFound && inst == nil {
				t.Fatal("expected instance, got nil")
			}
			if !tt.expectFound && inst != nil {
				t.Errorf("expected nil, got %+v", inst)
			}
		})
	}
}
