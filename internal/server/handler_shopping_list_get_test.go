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
					{path: "$[1]", absent: true},
				},
			},
		},
		{
			name: "store brands of the same product share one replenishment line",
			setup: func(env testEnv) {
				// One can of each brand is already in stock. Each target is a
				// full supply of 4, so buying each brand separately would ask
				// for 3+3+3. The shared need is 4-3=1, plus corn on its own.
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-gv", "Great Value Cut Green Beans", "item-gv")
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-kr", "Kroger Cut Green Beans", "item-kr")
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-wf", "Western Family Cut Green Beans", "item-wf")
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-corn", "Kroger Whole Kernel Corn", "item-corn")
				setTargetQuantity(env.T, env.DB, "item-gv", 4)
				setTargetQuantity(env.T, env.DB, "item-kr", 4)
				setTargetQuantity(env.T, env.DB, "item-wf", 4)
				setTargetQuantity(env.T, env.DB, "item-corn", 2)
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-gv"},
					{path: "$[0].quantity", value: float64(1)},
					{path: "$[0].source", value: "auto"},
					{path: "$[1].itemId", value: "item-corn"},
					{path: "$[1].quantity", value: float64(1)},
					{path: "$[1].source", value: "auto"},
					{path: "$[2]", absent: true},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					// Nothing is sent while no provider is configured. The pooled
					// lines above are what a configured provider would receive.
					{path: "$.exported", value: float64(0)},
				},
			}),
		},
		{
			name: "equivalent stock without a target counts toward the shared supply",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-gv-notarget", "Great Value Cut Green Beans", "item-gv-notarget")
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-kr-target", "Kroger Cut Green Beans", "item-kr-target")
				setTargetQuantity(env.T, env.DB, "item-kr-target", 4)
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-kr-target"},
					{path: "$[0].quantity", value: float64(2)},
					{path: "$[0].source", value: "auto"},
					{path: "$[1]", absent: true},
				},
			},
		},
		{
			name: "manual line for one brand suppresses the shared auto gap",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-gv-manual", "Great Value Cut Green Beans", "item-gv-manual")
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-kr-manual", "Kroger Cut Green Beans", "item-kr-manual")
				setTargetQuantity(env.T, env.DB, "item-gv-manual", 4)
				setTargetQuantity(env.T, env.DB, "item-kr-manual", 4)
				insertManualShoppingItem(env.T, env.DB, "sli-beans", "user-1", "item-kr-manual", 5)
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-kr-manual"},
					{path: "$[0].quantity", value: float64(5)},
					{path: "$[0].source", value: "manual"},
					{path: "$[1]", absent: true},
				},
			},
		},
		{
			name: "unlisted national brand is its own replenishment need",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-delmonte", "Del Monte Cut Green Beans", "item-delmonte")
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-kroger-nat", "Kroger Cut Green Beans", "item-kroger-nat")
				setTargetQuantity(env.T, env.DB, "item-delmonte", 4)
				setTargetQuantity(env.T, env.DB, "item-kroger-nat", 4)
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-delmonte"},
					{path: "$[0].quantity", value: float64(3)},
					{path: "$[1].itemId", value: "item-kroger-nat"},
					{path: "$[1].quantity", value: float64(3)},
					{path: "$[2]", absent: true},
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
