// Package domain defines shared domain models and typed errors.
//
// Author: Miroslav Pašek
package domain

import "fmt"

type ErrorCode string

const (
	ErrConfig    ErrorCode = "E_CONFIG"
	ErrIngest    ErrorCode = "E_INGEST"
	ErrTransform ErrorCode = "E_TRANSFORM"
	ErrValidate  ErrorCode = "E_VALIDATE"
	ErrOutput    ErrorCode = "E_OUTPUT"
)

type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

// Error formats a typed domain error.
func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}

	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
}

// Unwrap returns the wrapped underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// NewError creates a typed domain error value.
func NewError(code ErrorCode, message string, err error) error {
	return &Error{Code: code, Message: message, Err: err}
}
