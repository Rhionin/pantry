package server

import (
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/Rhionin/pantry/internal/product"
	"github.com/steinfletcher/apitest"
	jsonpath "github.com/steinfletcher/apitest-jsonpath"
)

// httpExchange represents a single HTTP request/response pair for declarative testing.
type httpExchange struct {
	method         string
	path           string
	query          map[string]string
	body           string
	expectedStatus int
	assertions     []assertion
}

// handlerTestCase defines a single HTTP handler test case for table-driven testing.
type handlerTestCase struct {
	name string

	// setup seeds database state before the primary request.
	setup func(env testEnv)

	// The primary HTTP exchange to test.
	httpExchange

	// afterRequest verifies behavior after the primary request.
	// Prefer exchanges() over direct DB queries.
	afterRequest func(env testEnv)
}

// assertion wraps a JSONPath assertion for cleaner test tables.
type assertion struct {
	path  string
	value interface{}
}

// testEnv holds everything a setup or afterRequest callback needs.
// All fields share the same underlying database, so writes in setup are
// immediately visible to the handler and to afterRequest exchanges.
type testEnv struct {
	T             *testing.T
	DB            *sql.DB
	ProductStore  *product.Catalog
	OpenFoodFacts *fakeOpenFoodFacts
	Refresher     *product.Refresher
	Clock         *fakeClock
	Res           *http.Response // populated only inside afterRequest callbacks
}

// runHandlerTests executes a table of handler test cases.
func runHandlerTests(t *testing.T, tests []handlerTestCase) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, catalog, fake, db, refresher, clock := setupTestWithDB(t)
			env := testEnv{T: t, DB: db, ProductStore: catalog, OpenFoodFacts: fake, Refresher: refresher, Clock: clock}

			if tt.setup != nil {
				tt.setup(env)
			}

			test := apitest.New().Handler(handler)
			req := buildRequest(test, tt.httpExchange)
			expect := buildExpectations(req, tt.httpExchange)

			var capturedRes *http.Response
			if tt.afterRequest != nil {
				expect = expect.Assert(func(res *http.Response, _ *http.Request) error {
					capturedRes = res
					return nil
				})
			}

			expect.End()

			if tt.afterRequest != nil && capturedRes != nil {
				env.Res = capturedRes
				tt.afterRequest(env)
			}
		})
	}
}

// executeExchange executes a single HTTP exchange against a handler.
func executeExchange(t *testing.T, handler http.Handler, ex httpExchange) {
	test := apitest.New().Handler(handler)
	req := buildRequest(test, ex)
	expect := buildExpectations(req, ex)
	expect.End()
}

// buildRequest builds an apitest request from an httpExchange.
func buildRequest(test *apitest.APITest, ex httpExchange) *apitest.Request {
	var req *apitest.Request
	switch ex.method {
	case "GET":
		req = test.Get(ex.path)
	case "POST":
		req = test.Post(ex.path)
	case "PUT":
		req = test.Put(ex.path)
	case "PATCH":
		req = test.Patch(ex.path)
	case "DELETE":
		req = test.Delete(ex.path)
	default:
		panic("unsupported method: " + ex.method)
	}

	for key, val := range ex.query {
		req = req.Query(key, val)
	}

	if ex.body != "" {
		req = req.JSON(ex.body)
	}

	return req
}

// buildExpectations builds apitest expectations from an httpExchange.
func buildExpectations(req *apitest.Request, ex httpExchange) *apitest.Response {
	expect := req.Expect(nil).Status(ex.expectedStatus)

	for _, a := range ex.assertions {
		expect = expect.Assert(jsonpath.Equal(a.path, a.value))
	}

	return expect
}

// exchanges returns an afterRequest callback that fires a sequence of HTTP requests
// against the same handler and database, verifying state through the HTTP contract.
func exchanges(exs ...httpExchange) func(env testEnv) {
	return func(env testEnv) {
		// Reuse env.Refresher rather than building a fresh one: a second
		// Refresher would own a different WaitGroup and in-flight map, so
		// env.Refresher.Wait() would return without awaiting goroutines this
		// handler's exchanges started, and tests would flake silently.
		var now func() time.Time
		if env.Clock != nil {
			now = env.Clock.Now
		}
		handler := NewHandler(env.ProductStore, &product.LookupService{
			Catalog:       env.ProductStore,
			OpenFoodFacts: env.OpenFoodFacts,
			Refresher:     env.Refresher,
			Now:           now,
		}, env.Refresher, env.DB)

		for i, ex := range exs {
			env.T.Run("", func(t *testing.T) {
				executeExchange(t, handler, ex)
			})

			if env.T.Failed() {
				env.T.Logf("exchange %d failed, stopping sequence", i)
				break
			}
		}
	}
}
