package dsauth

import (
	"errors"
	"fmt"
	"net/http"
)

// Machine-readable error codes, returned to API callers so agents and the FE
// can tell "this process has no identity" from "the identity lacks permission".
const (
	CodeNoCredentials    = "no_credentials"
	CodeAssumeRoleFailed = "assume_role_failed"
	CodeTokenFailed      = "token_failed"
	CodeAccessDenied     = "access_denied"
)

type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string {
	return fmt.Sprintf("datasource auth %s: %v", e.Code, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func newError(code string, err error) *Error {
	return &Error{Code: code, Err: err}
}

// CodeOf returns the classification of err, or "" if it is not an auth error.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// StatusCode classifies an upstream response status for a cloud-auth
// datasource. 401/403 with valid credentials almost always means the identity
// is missing a permission (e.g. aps:QueryMetrics) or the role/app is not
// granted on the resource.
func StatusCode(c Config, status int) string {
	if !c.Normalize().IsCloud() {
		return ""
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return CodeAccessDenied
	}
	return ""
}
