# Agent Guidelines

This document provides guidelines for AI coding assistants working on this project.

## Testing

### Running Tests

Run tests with coverage across all packages:

```bash
go test -cover -coverpkg=./... ./...
```

This project heavily relies on API-level integration tests in `internal/server/` to provide coverage across the entire codebase, regardless of package boundaries.

### Test Framework

Handler tests use a declarative table-driven framework with HTTP exchanges:

- **Primary exchange**: The main HTTP request/response to test (embedded in `handlerTestCase`)
- **Setup functions**: Named with `setup` prefix (e.g., `setupProduct`, `setupProductWithBarcode`)
- **Follow-up exchanges**: Use `exchanges()` for additional HTTP verification
  - Single exchange: `exchanges(httpExchange{...})`
  - Multiple exchanges: `exchanges([]httpExchange{{...}, {...}}...)`

Example:
```go
{
    name: "successful create",
    httpExchange: httpExchange{
        method:         "POST",
        path:           "/api/products",
        body:           `{"name":"Orange"}`,
        expectedStatus: http.StatusCreated,
    },
    // Verify via HTTP that the product was created
    afterRequest: exchanges(httpExchange{
        method:         "GET",
        path:           "/api/products",
        expectedStatus: http.StatusOK,
        assertions: []assertion{
            {path: "$[0].Name", value: "Orange"},
        },
    }),
}
```

### Test Philosophy

- **Test the HTTP contract, not implementation details**: Use `afterRequest: exchanges()` to verify behavior through HTTP requests rather than direct database queries
- **Prefer declarative over imperative**: Express tests as HTTP exchanges whenever possible
- **Use slice syntax for multiple exchanges**: Makes test tables cleaner and less repetitive

## Code Coverage

Always run coverage with `-coverpkg=./...` to measure coverage across all packages, not just the package being tested. API-level tests in `internal/server/` are designed to cover functionality across multiple packages.
