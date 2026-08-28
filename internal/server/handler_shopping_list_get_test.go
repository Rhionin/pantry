package server

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
)

func TestShoppingListGet(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "empty list when no items",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$", value: []interface{}{}},
				},
			},
		},
		{
			name: "returns manual item",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-sl-1", "Milk", "item-sl-1")
				insertManualShoppingItem(env.T, env.DB, "sli-1", "user-1", "item-sl-1", 2)
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-sl-1"},
					{path: "$[0].quantity", value: float64(2)},
					{path: "$[0].source", value: "manual"},
				},
			},
		},
		{
			name: "returns derived entry when item is below target quantity",
			setup: func(env testEnv) {
				// createItemViaStockIn creates one instance; setting target=3 → gap of 2
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-sl-2", "Eggs", "item-sl-2")
				setTargetQuantity(env.T, env.DB, "item-sl-2", 3)
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-sl-2"},
					{path: "$[0].quantity", value: float64(2)},
					{path: "$[0].source", value: "auto"},
					{path: "$[0].id", value: ""},
				},
			},
		},
	}

	runHandlerTests(t, tests)
}

// insertManualShoppingItem inserts a manual shopping list item directly into the DB.
func insertManualShoppingItem(t *testing.T, db *sql.DB, id, userID, itemID string, quantity int) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO shopping_list_items (id, user_id, item_id, quantity, source) VALUES (?, ?, ?, ?, 'manual')`,
		id, userID, itemID, quantity,
	); err != nil {
		t.Fatalf("insertManualShoppingItem: %v", err)
	}
}

// setTargetQuantity sets the target_quantity for an item in the DB.
func setTargetQuantity(t *testing.T, db *sql.DB, itemID string, qty int) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`UPDATE items SET target_quantity = ? WHERE id = ?`, qty, itemID,
	); err != nil {
		t.Fatalf("setTargetQuantity: %v", err)
	}
}
