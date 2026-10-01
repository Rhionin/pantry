package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/cart"
	"github.com/Rhionin/pantry/internal/cart/carttest"
	"github.com/Rhionin/pantry/internal/cart/connection"
	"github.com/Rhionin/pantry/internal/cart/kroger"
)

func TestCartHTTP(t *testing.T) {
	tests := []handlerTestCase{
		{
			name: "providers list is empty when nothing is registered",
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/providers",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$", value: []interface{}{}},
				},
			},
		},
		{
			name: "unconfigured kroger is listed as disconnected",
			setup: func(env testEnv) {
				if err := env.Registry.Register(kroger.NewUnconfigured()); err != nil {
					env.T.Fatalf("register: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/providers",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].id", value: "kroger"},
					{path: "$[0].displayName", value: "Kroger"},
					{path: "$[0].connectionState", value: "disconnected"},
					{path: "$[0].credentialsConfigured", value: false},
					{path: "$[0].capabilities.auth", value: "oauth2_authorization_code"},
					{path: "$[0].capabilities.confirmation", value: "per_request"},
				},
			},
		},
		{
			name: "authorize refuses an unconfigured provider",
			setup: func(env testEnv) {
				if err := env.Registry.Register(kroger.NewUnconfigured()); err != nil {
					env.T.Fatalf("register: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/providers/kroger/authorize",
				expectedStatus: http.StatusConflict,
				bodyContains:   []string{"not configured"},
			},
		},
		{
			name: "authorize returns the provider authorization URL",
			setup: func(env testEnv) {
				adapter, err := kroger.New("my-client", "secret", "https://app.example/cb", "PICKUP")
				if err != nil {
					env.T.Fatalf("kroger.New: %v", err)
				}
				if err := env.Registry.Register(adapter, cart.WithCredentialsConfigured(true)); err != nil {
					env.T.Fatalf("register: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "GET",
				path:           "/api/providers/kroger/authorize",
				expectedStatus: http.StatusOK,
				bodyContains: []string{
					"https://api.kroger.com/v1/connect/oauth2/authorize",
					"client_id=my-client",
					"response_type=code",
					"cart.basic:write",
				},
			},
		},
		{
			name: "export names an unknown provider as a bad request",
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				body:           `{"provider":"nope"}`,
				expectedStatus: http.StatusBadRequest,
				bodyContains:   []string{"not registered"},
			},
		},
		{
			name: "export names an unconfigured provider as a conflict",
			setup: func(env testEnv) {
				if err := env.Registry.Register(kroger.NewUnconfigured()); err != nil {
					env.T.Fatalf("register: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				body:           `{"provider":"kroger"}`,
				expectedStatus: http.StatusConflict,
				bodyContains:   []string{"not configured"},
			},
		},
		{
			name: "replenishment mode rejects an unknown value",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-mode", "Oats", "item-mode")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-mode/replenishment-mode",
				body:           `{"mode":"weekly"}`,
				expectedStatus: http.StatusUnprocessableEntity,
			},
		},
		{
			name: "replenishment mode records replenish",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-mode-ok", "Rice", "item-mode-ok")
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/items/item-mode-ok/replenishment-mode",
				body:           `{"mode":"replenish"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.mode", value: "replenish"},
				},
			},
		},
		{
			name: "adjustment rejects a quantity above 999",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-adj", "Beans", "item-adj")
				insertManualShoppingItem(env.T, env.DB, "sli-adj", "user-1", "item-adj", 2)
			},
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/items/sli-adj/adjustment",
				query:          map[string]string{"provider": "kroger"},
				body:           `{"adjustment":1000}`,
				expectedStatus: http.StatusUnprocessableEntity,
			},
		},
		{
			name: "adjustment is the provision quantity on the next list read",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-adj-ok", "Beans", "item-adj-ok")
				insertManualShoppingItem(env.T, env.DB, "sli-adj-ok", "user-1", "item-adj-ok", 2)
			},
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/shopping-list/items/sli-adj-ok/adjustment",
				query:          map[string]string{"provider": "kroger"},
				body:           `{"adjustment":4}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.adjustment", value: float64(4)},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/shopping-list",
				query:          map[string]string{"provider": "kroger"},
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].itemId", value: "item-adj-ok"},
					{path: "$[0].quantity", value: float64(4)},
					{path: "$[0].computedQuantity", value: float64(2)},
					{path: "$[0].adjustment", value: float64(4)},
					{path: "$[0].provider", value: "kroger"},
					{path: "$[0].replenishmentMode", value: "target"},
				},
			}),
		},
		{
			name: "disconnect clears the connection and leaves the provider listed",
			setup: func(env testEnv) {
				adapter, err := kroger.New("my-client", "secret", "https://app.example/cb", "PICKUP")
				if err != nil {
					env.T.Fatalf("kroger.New: %v", err)
				}
				if err := env.Registry.Register(adapter, cart.WithCredentialsConfigured(true)); err != nil {
					env.T.Fatalf("register: %v", err)
				}
				if err := connection.NewDirectory(env.DB).Write(context.Background(), &connection.Connection{
					Provider:    "kroger",
					State:       connection.StateConnected,
					AccessToken: "should-not-leak",
				}); err != nil {
					env.T.Fatalf("connect: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "DELETE",
				path:           "/api/providers/kroger/connection",
				expectedStatus: http.StatusCreated,
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/providers",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$[0].connectionState", value: "disconnected"},
					{path: "$[0].credentialsConfigured", value: true},
				},
				bodyContains: []string{"disconnected"},
			}),
		},
		{
			name: "provision accepts a line and advances the ledger",
			setup: func(env testEnv) {
				createItemViaStockIn(env.T, env.DB, env.ProductStore, "user-1", "prod-prov", "Milk", "item-prov")
				insertManualShoppingItem(env.T, env.DB, "sli-prov", "user-1", "item-prov", 2)
				if err := env.ProductStore.UpsertBarcodeMapping(context.Background(), "111", "prod-prov", "global", ""); err != nil {
					env.T.Fatalf("barcode: %v", err)
				}
				script := carttest.NewScript().
					WithDispositions(cart.DispositionAccepted).
					WithIdentityLookup("111", cart.ProductIdentity("0001111111111"))
				provider := carttest.NewFake(cart.Capabilities{
					Auth:         cart.AuthNone,
					Delivery:     cart.DeliveryServerPush,
					Confirmation: cart.ConfirmPerRequest,
					Mutation:     cart.MutateAddOnly,
					Identity:     cart.IdentityDerived,
				}, script)
				if err := env.Registry.Register(provider, cart.WithCredentialsConfigured(true)); err != nil {
					env.T.Fatalf("register: %v", err)
				}
			},
			httpExchange: httpExchange{
				method:         "POST",
				path:           "/api/shopping-list/export",
				body:           `{"provider":"test-none-server_push"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.exported", value: float64(1)},
					{path: "$.provider", value: "test-none-server_push"},
					{path: "$.entries[0].outcome", value: "confirmed"},
					{path: "$.entries[0].name", value: "Milk"},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         "GET",
				path:           "/api/providers/test-none-server_push/ledger",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.entries[0].itemId", value: "item-prov"},
					{path: "$.entries[0].requested", value: float64(2)},
				},
			}),
		},
	}

	runHandlerTests(t, tests)
}
