package server

import (
	"errors"
	"net/http"
)

// HTTPError represents an error with an associated HTTP status code.
type HTTPError struct {
	Code    int
	Message string
	Err     error
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return http.StatusText(e.Code)
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

// Common HTTP error constructors.

func BadRequest(message string) error {
	return &HTTPError{Code: http.StatusBadRequest, Message: message}
}

func NotFound(message string) error {
	return &HTTPError{Code: http.StatusNotFound, Message: message}
}

func Conflict(message string) error {
	return &HTTPError{Code: http.StatusConflict, Message: message}
}

func InternalError(err error) error {
	return &HTTPError{Code: http.StatusInternalServerError, Err: err}
}

// httpStatusFromError determines the HTTP status code from an error.
func httpStatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Code
	}

	return http.StatusInternalServerError
}
