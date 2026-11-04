// Package apperr defines the typed application error. Every failure that
// can reach the API boundary should be one of these so the app gets a
// stable code instead of a raw error string.
package apperr

import (
	"errors"
	"net/http"

	"stickers-server/internal/models"
)

type Error struct {
	Code       models.ErrorCode
	Message    string
	Detail     string
	StatusCode int
}

func (e *Error) Error() string { return e.Message }

// New builds an error with the default 400 status.
func New(code models.ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message, StatusCode: http.StatusBadRequest}
}

func (e *Error) WithDetail(detail string) *Error {
	c := *e
	c.Detail = detail
	return &c
}

func (e *Error) WithStatus(status int) *Error {
	c := *e
	c.StatusCode = status
	return &c
}

// ToAPI converts to the JSON error shape persisted and returned to clients.
func (e *Error) ToAPI() *models.APIError {
	out := &models.APIError{Code: e.Code, Message: e.Message}
	if e.Detail != "" {
		d := e.Detail
		out.Detail = &d
	}
	return out
}

// From returns err as *Error if it is one, otherwise wraps it as an
// internal error (same fallback the Python job runner uses).
func From(err error) *Error {
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	return New(models.InternalError, "Ocurrió un error inesperado.").
		WithDetail(err.Error()).
		WithStatus(http.StatusInternalServerError)
}
