package server

import (
	"net/http"
	"testing"

	"github.com/Rhionin/pantry/internal/buildinfo"
)

func TestBuildInfoEndpoint(t *testing.T) {
	original := buildinfo.Commit
	t.Cleanup(func() { buildinfo.Commit = original })
	buildinfo.Commit = "0123456789abcdef0123456789abcdef01234567"

	runHandlerTests(t, []handlerTestCase{
		{
			name: "GET /api/build returns the stamped commit",
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.commit", value: "0123456789abcdef0123456789abcdef01234567"},
				},
			},
			afterRequest: assertContentTypePrefix("application/json"),
		},
		{
			name: "POST /api/build is not a build lookup",
			httpExchange: httpExchange{
				method:         "POST",
				url:            "/api/build",
				expectedStatus: http.StatusMethodNotAllowed,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
	})
}
