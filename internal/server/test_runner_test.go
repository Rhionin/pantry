package server

import (
	"net/http"
	"testing"

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

	// Setup phase: optional function to seed database/fake before the request
	setup func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts)

	// The primary HTTP exchange to test
	httpExchange

	// afterRequest is called after the HTTP request completes for additional verification.
	// Can be a plain callback function or use the assert() or exchanges() helpers.
	afterRequest func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts, res *http.Response)
}

// assertion wraps a JSONPath assertion for cleaner test tables.
type assertion struct {
	path  string
	value interface{}
}

// runHandlerTests executes a table of handler test cases.
func runHandlerTests(t *testing.T, tests []handlerTestCase) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, repo, fake := setupTest(t)

			// Run optional setup phase
			if tt.setup != nil {
				tt.setup(t, repo, fake)
			}

			// Execute the primary HTTP exchange and capture response
			test := apitest.New().Handler(handler)
			req := buildRequest(test, tt.httpExchange)
			expect := buildExpectations(req, tt.httpExchange)

			// Capture response if afterRequest callback is provided
			var capturedRes *http.Response
			if tt.afterRequest != nil {
				expect = expect.Assert(func(res *http.Response, req *http.Request) error {
					capturedRes = res
					return nil
				})
			}

			expect.End()

			// Run afterRequest callback if provided
			if tt.afterRequest != nil && capturedRes != nil {
				tt.afterRequest(t, repo, fake, capturedRes)
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

	// Add query parameters
	for key, val := range ex.query {
		req = req.Query(key, val)
	}

	// Add body if present
	if ex.body != "" {
		req = req.JSON(ex.body)
	}

	return req
}

// buildExpectations builds apitest expectations from an httpExchange.
func buildExpectations(req *apitest.Request, ex httpExchange) *apitest.Response {
	expect := req.Expect(nil).Status(ex.expectedStatus)

	// Add JSONPath assertions
	for _, a := range ex.assertions {
		expect = expect.Assert(jsonpath.Equal(a.path, a.value))
	}

	return expect
}

// assert creates an afterRequest callback that performs declarative assertions.
// Supports 0 or more assertion functions that can check database state, fake state, etc.
//
// Example usage:
//
//	afterRequest: assert(
//	    func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts) {
//	        // inline assertion logic
//	    },
//	)
func assert(checks ...func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts)) func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts, res *http.Response) {
	return func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts, res *http.Response) {
		for _, check := range checks {
			check(t, repo, fake)
		}
	}
}

// exchanges creates an afterRequest callback that executes a series of HTTP exchanges.
// This allows tests to be expressed as a sequence of request/response pairs without imperative code.
// All exchanges share the same handler/repo/fake, so state persists between exchanges.
//
// Example usage:
//
//	afterRequest: exchanges(httpExchange{...})  // single exchange
//	afterRequest: exchanges([]httpExchange{{...}, {...}}...)  // multiple exchanges with slice syntax
func exchanges(exs ...httpExchange) func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts, res *http.Response) {
	return func(t *testing.T, repo *product.Repo, fake *fakeOpenFoodFacts, res *http.Response) {
		// Get the DB from setupTestDB (need to pass it through)
		db := setupTestDB(t)

		// Reuse the same handler from the parent test to preserve state
		handler := NewHandler(repo, &product.LookupService{
			Repo:          repo,
			OpenFoodFacts: fake,
		}, db)

		for i, ex := range exs {
			// Use a simple counter for sub-test names
			t.Run("", func(t *testing.T) {
				executeExchange(t, handler, ex)
			})

			// Stop if a sub-test failed
			if t.Failed() {
				t.Logf("exchange %d failed, stopping sequence", i)
				break
			}
		}
	}
}
