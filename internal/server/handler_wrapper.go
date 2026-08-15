package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/go-json-experiment/json"
)

var pathParamRegExp = regexp.MustCompile(`\/{(.*?)}`)

// Request wraps handler input with context, body, and path parameters.
type Request[TBody, TPathParams any] struct {
	Context    context.Context
	Body       TBody
	PathParams TPathParams
	RawRequest *http.Request
}

// Created wraps a response value to signal a 201 Created status.
type Created struct {
	Value any
}

// handlerFunc is a handler function that takes a parsed request and returns a response.
type handlerFunc[TBody, TPathParams, TResp any] func(req Request[TBody, TPathParams]) (TResp, error)

// HandleJSON wraps a handler function with JSON parsing and response writing.
// Success defaults to 200 OK, or 201 Created if the response type is Created.
// Errors are converted to HTTP status codes via httpStatusFromError.
// The pattern is extracted from r.Pattern at runtime (Go 1.22+).
func HandleJSON[TBody, TPathParams, TResp any](fn handlerFunc[TBody, TPathParams, TResp]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body TBody

		if r.ContentLength > 0 {
			if err := json.UnmarshalRead(r.Body, &body); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
				return
			}
		}

		pattern := r.Pattern
		pathParams, err := parsePathParams[TPathParams](r, pattern)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid path parameters: %v", err))
			return
		}

		req := Request[TBody, TPathParams]{
			Context:    r.Context(),
			Body:       body,
			PathParams: pathParams,
			RawRequest: r,
		}

		resp, err := fn(req)
		if err != nil {
			status := httpStatusFromError(err)
			writeError(w, status, err.Error())
			return
		}

		status := http.StatusOK
		var respData any = resp

		if created, ok := any(resp).(Created); ok {
			status = http.StatusCreated
			respData = created.Value
		}

		writeJSON(w, status, respData)
	}
}

// parsePathParams extracts path parameters from the request using the pattern.
func parsePathParams[Pp any](r *http.Request, pattern string) (Pp, error) {
	matches := pathParamRegExp.FindAllStringSubmatch(pattern, -1)

	rawMap := map[string]string{}
	for _, match := range matches {
		rawMap[match[1]] = r.PathValue(match[1])
	}
	var pp Pp

	d, err := json.Marshal(rawMap)
	if err != nil {
		return pp, err
	}

	if err := json.Unmarshal(d, &pp); err != nil {
		return pp, err
	}
	return pp, nil
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, data); err != nil {
		fmt.Printf("error encoding JSON response: %v\n", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
