// Package errors defines typed errors with HTTP status mapping.
package errors

import (
	"errors"
	"net/http"
)

// AppError is a domain error with an HTTP-mappable code.
type AppError struct {
	Code    string // machine-readable, e.g. "tenant_not_found"
	Message string // human-readable
	Status  int    // HTTP status
	Wrapped error
}

func (e *AppError) Error() string {
	if e.Wrapped != nil {
		return e.Code + ": " + e.Message + ": " + e.Wrapped.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error { return e.Wrapped }

// New constructs an AppError.
func New(status int, code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Status: status}
}

// Wrap wraps an underlying error in an AppError.
func Wrap(err error, status int, code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Status: status, Wrapped: err}
}

// Common helpers.
func NotFound(code, msg string) *AppError    { return New(http.StatusNotFound, code, msg) }
func BadRequest(code, msg string) *AppError  { return New(http.StatusBadRequest, code, msg) }
func Conflict(code, msg string) *AppError    { return New(http.StatusConflict, code, msg) }
func Internal(code, msg string) *AppError    { return New(http.StatusInternalServerError, code, msg) }
func Unauthorized(code, msg string) *AppError {
	return New(http.StatusUnauthorized, code, msg)
}

// As is a convenience to extract an *AppError from any error.
func As(err error) (*AppError, bool) {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
