package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	BadRequest          Code = "BAD_REQUEST"
	NotFound            Code = "NOT_FOUND"
	Conflict            Code = "CONFLICT"
	Validation          Code = "VALIDATION"
	SQLUnsupported      Code = "SQL_UNSUPPORTED"
	SQLSemantic         Code = "SQL_SEMANTIC"
	ResultExpired       Code = "RESULT_EXPIRED"
	InsufficientMemory  Code = "INSUFFICIENT_MEMORY"
	UploadTooLarge      Code = "UPLOAD_TOO_LARGE"
	UnsupportedFormat   Code = "UNSUPPORTED_FORMAT"
	JobInterrupted      Code = "JOB_INTERRUPTED"
	TableCorrupted      Code = "TABLE_CORRUPTED"
	QueryCanceled       Code = "QUERY_CANCELED"
	QueryTimeout        Code = "QUERY_TIMEOUT"
	Internal            Code = "INTERNAL"
	Unauthorized        Code = "UNAUTHORIZED"
)

type Error struct {
	Code       Code
	Message    string
	Details    map[string]any
	HTTPStatus int
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func New(code Code, status int, msg string) *Error {
	return &Error{Code: code, HTTPStatus: status, Message: msg, Details: map[string]any{}}
}

func (e *Error) With(k string, v any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[k] = v
	return e
}

func (e *Error) Wrap(msg string) *Error {
	e.Message = fmt.Sprintf("%s: %s", msg, e.Message)
	return e
}

func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

func StatusOf(err error) int {
	if e, ok := As(err); ok && e.HTTPStatus != 0 {
		return e.HTTPStatus
	}
	return http.StatusInternalServerError
}

func Bad(msg string) *Error {
	return New(BadRequest, http.StatusBadRequest, msg)
}

func Miss(msg string) *Error {
	return New(NotFound, http.StatusNotFound, msg)
}

func Sem(msg string) *Error {
	return New(SQLSemantic, http.StatusBadRequest, msg)
}

func Unsup(feature, hint string) *Error {
	return New(SQLUnsupported, http.StatusBadRequest, "unsupported SQL feature: "+feature).
		With("feature", feature).With("hint", hint)
}

func Mem(msg string) *Error {
	return New(InsufficientMemory, http.StatusInsufficientStorage, msg)
}
