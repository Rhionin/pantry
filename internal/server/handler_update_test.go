package server

import (
	"net/http"
	"testing"
)

func TestUpdateHandler(t *testing.T) {
	tests := []handlerTestCase{
		{
			name:  "successful update",
			setup: setupProduct("prod-1", "Old Name", "Old Category"),
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/products/prod-1",
				body:           `{"name":"New Name","category":"New Category","unitOfMeasure":"kg"}`,
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.Name", value: "New Name"},
					{path: "$.Category", value: "New Category"},
					{path: "$.UnitOfMeasure", value: "kg"},
					{path: "$.ID", value: "prod-1"},
				},
			},
		},
		{
			name: "product not found",
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/products/nonexistent",
				body:           `{"name":"New Name"}`,
				expectedStatus: http.StatusInternalServerError,
			},
		},
		{
			name:  "missing name",
			setup: setupProduct("prod-1", "Old Name", ""),
			httpExchange: httpExchange{
				method:         "PUT",
				path:           "/api/products/prod-1",
				body:           `{"category":"New Category"}`,
				expectedStatus: http.StatusBadRequest,
			},
		},
	}

	runHandlerTests(t, tests)
}
