package server

import (
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/product"
)

func TestGroupHandlers(t *testing.T) {
	runHandlerTests(t, []handlerTestCase{
		{
			name: "product cannot join two groups",
			setup: func(env testEnv) {
				mustGroupProduct(env, "p1", "Beans")
				mustGroupProduct(env, "p2", "Other")
				insertGroup(env, "group-a", "First")
				insertMember(env, "group-a", "p1")
				insertGroup(env, "group-b", "Second")
			},
			httpExchange: httpExchange{
				method:         http.MethodPost,
				path:           "/api/groups/group-b/members",
				body:           `{"productIds":["p1"]}`,
				expectedStatus: http.StatusConflict,
				assertions:     []assertion{{path: "$.error", value: "That product is already in another group."}},
			},
			afterRequest: exchanges(httpExchange{
				method:         http.MethodGet,
				path:           "/api/groups/group-a",
				expectedStatus: http.StatusOK,
				assertions:     []assertion{{path: "$.members[0].productId", value: "p1"}},
			}),
		},
		{
			name: "pin that is not a member is rejected",
			setup: func(env testEnv) {
				mustGroupProduct(env, "p1", "Beans")
				insertGroup(env, "group-a", "Beans")
				insertMember(env, "group-a", "p1")
			},
			httpExchange: httpExchange{
				method:         http.MethodPut,
				path:           "/api/groups/group-a/rule",
				body:           `{"rule":"favorite","pinnedProductId":"missing","confirm":true}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
		{
			name: "member overrides without a target write nothing",
			setup: func(env testEnv) {
				mustGroupProduct(env, "p1", "Small")
				mustGroupProduct(env, "p2", "Large")
				mustExec(env, `INSERT INTO supply_overrides (product_id, quantity) VALUES ('p1', 4)`)
				mustExec(env, `INSERT INTO supply_overrides (product_id, window_months) VALUES ('p2', 2)`)
			},
			httpExchange: httpExchange{
				method:         http.MethodPost,
				path:           "/api/groups",
				body:           `{"name":"Beans","productIds":["p1","p2"]}`,
				expectedStatus: http.StatusConflict,
				assertions: []assertion{
					{path: "$.code", value: "target_decision_required"},
					{path: "$.members[0].productId", value: "p1"},
					{path: "$.members[0].quantity", value: float64(4)},
					{path: "$.members[1].windowMonths", value: float64(2)},
				},
			},
			afterRequest: exchanges(
				httpExchange{
					method:         http.MethodGet,
					path:           "/api/groups",
					expectedStatus: http.StatusOK,
					bodyContains:   []string{"[]"},
					bodyExcludes:   []string{"Beans"},
				},
				httpExchange{
					method:         http.MethodPost,
					path:           "/api/groups",
					body:           `{"name":"Beans","productIds":["p1","p2"],"target":{"quantity":24}}`,
					expectedStatus: http.StatusCreated,
					assertions: []assertion{
						{path: "$.quantity", value: float64(24)},
						{path: "$.dimension", value: "mass"},
					},
				},
				httpExchange{
					method:         http.MethodGet,
					path:           "/api/products/p1/supply-override",
					expectedStatus: http.StatusOK,
					assertions:     []assertion{{path: "$.quantity", absent: true}},
				},
				httpExchange{
					method:         http.MethodGet,
					path:           "/api/products/p2/supply-override",
					expectedStatus: http.StatusOK,
					assertions:     []assertion{{path: "$.windowMonths", absent: true}},
				},
			),
		},
		{
			name: "dismissed suggestion stays gone",
			setup: func(env testEnv) {
				mustGroupProduct(env, "p1", "A")
				mustGroupProduct(env, "p2", "B")
				mustExec(env, `INSERT INTO group_suggestions (id, user_id, kind, title, status) VALUES ('sug-1', 'user-1', 'looks_alike', 'Beans', 'open')`)
				mustExec(env, `INSERT INTO group_suggestion_members (suggestion_id, product_id, included) VALUES ('sug-1', 'p1', 1), ('sug-1', 'p2', 1)`)
			},
			httpExchange: httpExchange{
				method:         http.MethodPost,
				path:           "/api/group-suggestions/sug-1/dismiss",
				expectedStatus: http.StatusNoContent,
			},
			afterRequest: exchanges(httpExchange{
				method:         http.MethodGet,
				path:           "/api/group-suggestions",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{"[]"},
				bodyExcludes:   []string{"sug-1"},
			}),
		},
		{
			name: "preview sentences",
			httpExchange: httpExchange{
				method: http.MethodPost,
				path:   "/api/groups/preview",
				body: `{
					"rule":"same_as_ran_out",
					"members":[
						{"productId":"hunts","name":"Hunt's 14.5 oz","lastConsumedAt":"2026-01-02T00:00:00Z"},
						{"productId":"gv","name":"Great Value 14.5 oz","lastConsumedAt":"2026-03-02T00:00:00Z"}
					]
				}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.productId", value: "gv"},
					{path: "$.because", value: "The last one used up was Great Value 14.5 oz."},
				},
			},
		},
		{
			name: "preview favorite and best deal",
			httpExchange: httpExchange{
				method: http.MethodPost,
				path:   "/api/groups/preview",
				body: `{
					"rule":"favorite",
					"pinnedProductId":"a",
					"members":[{"productId":"a","name":"Starred"},{"productId":"b","name":"Other"}]
				}`,
				expectedStatus: http.StatusOK,
				assertions:     []assertion{{path: "$.because", value: "This is the one with the star."}},
			},
			afterRequest: exchanges(httpExchange{
				method: http.MethodPost,
				path:   "/api/groups/preview",
				body: `{
					"rule":"best_deal",
					"members":[{"productId":"gv","name":"Great Value 14.5 oz"}],
					"deals":[{"productId":"gv","priceCents":59,"regularPriceCents":89,"notedAt":"2026-10-05T15:00:00Z"}]
				}`,
				expectedStatus: http.StatusOK,
				assertions:     []assertion{{path: "$.because", value: "On sale for $0.59, usually $0.89. You noted this sale on Oct 5."}},
			}),
		},
		{
			name: "rename delete remove and target",
			setup: func(env testEnv) {
				mustGroupProduct(env, "p1", "Beans")
				mustGroupProduct(env, "p2", "Corn")
				insertGroup(env, "group-a", "Beans")
				insertMember(env, "group-a", "p1")
			},
			httpExchange: httpExchange{
				method:         http.MethodPatch,
				path:           "/api/groups/group-a",
				body:           `{"name":"Pantry beans"}`,
				expectedStatus: http.StatusOK,
				assertions:     []assertion{{path: "$.name", value: "Pantry beans"}},
			},
			afterRequest: exchanges(
				httpExchange{
					method:         http.MethodPut,
					path:           "/api/groups/group-a/target",
					body:           `{"windowMonths":6}`,
					expectedStatus: http.StatusOK,
					assertions:     []assertion{{path: "$.windowMonths", value: float64(6)}},
				},
				httpExchange{
					method:         http.MethodPost,
					path:           "/api/groups/group-a/members",
					body:           `{"productIds":["p2"],"target":{"quantity":10}}`,
					expectedStatus: http.StatusOK,
					assertions:     []assertion{{path: "$.quantity", value: float64(10)}},
				},
				httpExchange{
					method:         http.MethodDelete,
					path:           "/api/groups/group-a/members/p1",
					expectedStatus: http.StatusOK,
					assertions:     []assertion{{path: "$.deleted", value: false}},
				},
				httpExchange{
					method:         http.MethodDelete,
					path:           "/api/groups/group-a",
					expectedStatus: http.StatusNoContent,
				},
				httpExchange{
					method:         http.MethodGet,
					path:           "/api/groups/group-a",
					expectedStatus: http.StatusNotFound,
				},
			),
		},
		{
			name: "accept and skip suggestions",
			setup: func(env testEnv) {
				mustGroupProduct(env, "p1", "A")
				mustGroupProduct(env, "p2", "B")
				mustExec(env, `INSERT INTO group_suggestions (id, user_id, kind, title, proposed_rule, pinned_product_id, status) VALUES ('sug-ok', 'user-1', 'looks_alike', 'Beans', 'favorite', 'p1', 'open')`)
				mustExec(env, `INSERT INTO group_suggestion_members (suggestion_id, product_id, included) VALUES ('sug-ok', 'p1', 1), ('sug-ok', 'p2', 1)`)
				mustExec(env, `INSERT INTO group_suggestions (id, user_id, kind, title, status) VALUES ('sug-skip', 'user-1', 'from_scan', 'Later', 'open')`)
			},
			httpExchange: httpExchange{
				method:         http.MethodPost,
				path:           "/api/group-suggestions/sug-ok/accept",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.name", value: "Beans"},
					{path: "$.rule", value: "favorite"},
					{path: "$.pinnedProductId", value: "p1"},
				},
			},
			afterRequest: exchanges(httpExchange{
				method:         http.MethodPost,
				path:           "/api/group-suggestions/sug-skip/skip",
				expectedStatus: http.StatusNoContent,
			}, httpExchange{
				method:         http.MethodGet,
				path:           "/api/group-suggestions",
				expectedStatus: http.StatusOK,
				assertions:     []assertion{{path: "$[0].id", value: "sug-skip"}},
			}),
		},
		{
			name: "preview a saved group",
			setup: func(env testEnv) {
				mustGroupProduct(env, "p1", "Hunt's 14.5 oz")
				insertGroup(env, "group-a", "Beans")
				insertMember(env, "group-a", "p1")
			},
			httpExchange: httpExchange{
				method:         http.MethodPost,
				path:           "/api/groups/preview",
				body:           `{"groupId":"group-a","rule":"same_as_ran_out"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.productId", value: "p1"},
					{path: "$.because", value: "Nothing has run out yet."},
				},
			},
		},
		{
			name: "default rule is copied onto a new group",
			httpExchange: httpExchange{
				method:         http.MethodPut,
				path:           "/api/settings/group-rule",
				body:           `{"rule":"favorite"}`,
				expectedStatus: http.StatusOK,
				assertions:     []assertion{{path: "$.rule", value: "favorite"}},
			},
			afterRequest: exchanges(
				httpExchange{
					method:         http.MethodPost,
					path:           "/api/groups",
					body:           `{"name":"Empty"}`,
					expectedStatus: http.StatusCreated,
					assertions: []assertion{
						{path: "$.rule", value: "favorite"},
						{path: "$.ruleConfirmed", value: false},
					},
				},
				httpExchange{
					method:         http.MethodGet,
					path:           "/api/settings/group-rule",
					expectedStatus: http.StatusOK,
					assertions:     []assertion{{path: "$.rule", value: "favorite"}},
				},
			),
		},
	})
}

func mustGroupProduct(env testEnv, id, name string) {
	env.T.Helper()
	if err := env.ProductStore.CreateProduct(env.T.Context(), product.Product{ID: id, Name: name}); err != nil {
		env.T.Fatal(err)
	}
}

func insertGroup(env testEnv, id, name string) {
	env.T.Helper()
	mustExec(env, `INSERT INTO product_groups (id, user_id, name, name_key, rule) VALUES (?, 'user-1', ?, lower(?), 'same_as_ran_out')`, id, name, name)
}

func insertMember(env testEnv, groupID, productID string) {
	env.T.Helper()
	mustExec(env, `INSERT INTO product_group_members (product_id, group_id) VALUES (?, ?)`, productID, groupID)
}

func mustExec(env testEnv, query string, args ...any) {
	env.T.Helper()
	if _, err := env.DB.Exec(query, args...); err != nil {
		env.T.Fatal(err)
	}
}
