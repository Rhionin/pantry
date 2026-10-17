package server

import (
	"net/http"
	"testing"
	"time"
)

func TestRebuyAfterStockInAndUseAgain(t *testing.T) {
	now := time.Now().UTC()
	fill := httpExchange{method: "POST", path: "/api/shopping-list/fill", expectedStatus: http.StatusOK}
	runHandlerTests(t, []handlerTestCase{{
		name: "bought, restocked, used again comes back",
		setup: func(env testEnv) {
			beginUsing(env.T, env.DB, now.Add(-29*24*time.Hour))
			createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-cyc", "Cycle", "item-cyc")
			insertConsumptionEvent(env.T, env.DB, "ce-cyc-1", "item-cyc", now.Add(-2*24*time.Hour))
		},
		httpExchange: httpExchange{method: "POST", path: "/api/shopping-list/fill", expectedStatus: http.StatusOK,
			assertions: []assertion{{path: "$[0].itemId", value: "item-cyc"}, {path: "$[0].note", value: "replacing 1 you used"}}},
		afterRequest: func(env testEnv) {
			if _, err := env.DB.Exec(`UPDATE shopping_list_items SET purchased_at = ? WHERE item_id = 'item-cyc'`, now.Add(-90*time.Minute)); err != nil {
				env.T.Fatal(err)
			}
			insertStockIn(env.T, env.DB, "prod-cyc", now.Add(-60*time.Minute))
			exchanges(fill)(env)
			exchanges(httpExchange{method: "GET", path: "/api/shopping-list", expectedStatus: http.StatusOK,
				assertions: []assertion{{path: "$[0]", absent: true}}})(env)
			insertConsumptionEvent(env.T, env.DB, "ce-cyc-2", "item-cyc", now.Add(-30*time.Minute))
			exchanges(fill)(env)
			var open, bought int
			env.DB.QueryRow(`SELECT COUNT(*) FROM shopping_list_items WHERE item_id='item-cyc' AND purchased_at IS NULL`).Scan(&open)
			env.DB.QueryRow(`SELECT COUNT(*) FROM shopping_list_items WHERE item_id='item-cyc' AND purchased_at IS NOT NULL`).Scan(&bought)
			if open != 1 {
				env.T.Fatalf("used again after restock, but no open shopping line (open=%d purchased=%d)", open, bought)
			}
		},
	}})
}
