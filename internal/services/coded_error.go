package services

import (
	"errors"
	"fmt"
)

// CodedError wraps an underlying error with a stable machine-oriented code for APIs and DB rows.
type CodedError struct {
	Code string
	Err  error
}

func (e *CodedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *CodedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ErrCode wraps err with a stable code (code should be snake_case).
func ErrCode(code string, err error) error {
	if err == nil {
		return nil
	}
	return &CodedError{Code: code, Err: err}
}

// PublicCode returns the outermost CodedError code, or "internal_error".
func PublicCode(err error) string {
	if err == nil {
		return ""
	}
	var coded *CodedError
	if errors.As(err, &coded) && coded != nil && coded.Code != "" {
		return coded.Code
	}
	return "internal_error"
}

// FirstPublicCode walks the error chain and returns the innermost CodedError code found
// (the last code encountered while unwrapping), so leaf failures win over wrapper codes.
func FirstPublicCode(err error) string {
	if err == nil {
		return ""
	}
	var last string
	cur := err
	seen := map[error]struct{}{}
	for cur != nil {
		if _, dup := seen[cur]; dup {
			break
		}
		seen[cur] = struct{}{}
		var coded *CodedError
		if errors.As(cur, &coded) && coded != nil && coded.Code != "" {
			last = coded.Code
		}
		cur = errors.Unwrap(cur)
	}
	if last != "" {
		return last
	}
	return "internal_error"
}

// ErrorfCode is fmt.Errorf with a stable code wrapper.
func ErrorfCode(code string, format string, args ...any) error {
	return ErrCode(code, fmt.Errorf(format, args...))
}
