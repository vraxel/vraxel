package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"vraxel.io/vraxel/lib/api/validation"
	"vraxel.io/vraxel/lib/runtime"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

// StatusError represents an API error with HTTP status code.
type StatusError struct {
	runtime.TypeMeta `json:",inline"`
	Status           int    `json:"status"`
	Reason           string `json:"reason"`
	Message          string `json:"message"`
	Details          any    `json:"details,omitempty"`
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: %s", e.Reason, e.Message)
}

func (e *StatusError) GetTypeMeta() *runtime.TypeMeta {
	return &e.TypeMeta
}

// GetStatus returns the HTTP status code.
func (e *StatusError) GetStatus() int {
	return e.Status
}

func newStatusError(status int, reason, message string, details any) *StatusError {
	return &StatusError{
		TypeMeta: runtime.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   status,
		Reason:   reason,
		Message:  message,
		Details:  details,
	}
}

func NewBadRequest(message string, details any) *StatusError {
	return newStatusError(http.StatusBadRequest, "BadRequest", message, details)
}

// NewBadRequestWithReason returns a 400 StatusError carrying a stable
// machine-readable reason instead of the generic "BadRequest". The
// frontend keys i18n lookups on this string, so values must remain
// stable across releases (PascalCase, no spaces).
func NewBadRequestWithReason(reason, message string, details any) *StatusError {
	return newStatusError(http.StatusBadRequest, reason, message, details)
}

// NewInvalidJSONBody returns a 400 InvalidJSONBody StatusError translated
// from a json.Unmarshal error. The Go-internal struct/field type names
// (e.g. "kafkaResetOffsetsRequest", "int64") are stripped from the message;
// only the JSON field name from the request payload is preserved. When the
// underlying error identifies a specific field, it is also placed in
// Details so the frontend can highlight it. Use this everywhere a handler
// reads a JSON request body to keep the wire-level error shape consistent.
func NewInvalidJSONBody(err error) *StatusError {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		field := typeErr.Field
		if field == "" {
			return newStatusError(http.StatusBadRequest, "InvalidJSONBody",
				"invalid value in request body", nil)
		}
		return newStatusError(http.StatusBadRequest, "InvalidJSONBody",
			fmt.Sprintf("invalid value for field '%s'", field),
			validation.ErrorList{{Field: field, Message: "invalid value or out of range"}})
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return newStatusError(http.StatusBadRequest, "InvalidJSONBody",
			"malformed JSON in request body", nil)
	}
	return newStatusError(http.StatusBadRequest, "InvalidJSONBody",
		"invalid request body", nil)
}

func NewNotFound(resource, name string) *StatusError {
	return newStatusError(http.StatusNotFound, "NotFound",
		fmt.Sprintf("%s %s not found", resource, name), nil)
}

// NewNotFoundWithReason returns a 404 StatusError carrying a stable
// machine-readable reason instead of the generic "NotFound", with the
// caller-supplied message used verbatim. The frontend keys i18n lookups
// on this string, so values must remain stable across releases (PascalCase,
// no spaces).
func NewNotFoundWithReason(reason, message string) *StatusError {
	return newStatusError(http.StatusNotFound, reason, message, nil)
}

func NewConflict(resource, name string) *StatusError {
	return newStatusError(http.StatusConflict, "Conflict",
		fmt.Sprintf("%s %s already exists", resource, name), nil)
}

func NewConflictMessage(message string) *StatusError {
	return newStatusError(http.StatusConflict, "Conflict", message, nil)
}

// NewConflictWithDetails returns a 409 Conflict carrying structured details.
// Used for e.g. optimistic concurrency failures where the caller needs to
// surface the current server-side state back to the client for reconciliation.
func NewConflictWithDetails(message string, details any) *StatusError {
	return newStatusError(http.StatusConflict, "Conflict", message, details)
}

func NewForbidden(message string) *StatusError {
	return newStatusError(http.StatusForbidden, "Forbidden", message, nil)
}

func NewInternalError(err error) *StatusError {
	msg := "internal server error"
	if err != nil {
		msg = err.Error()
	}
	return newStatusError(http.StatusInternalServerError, "InternalError", msg, nil)
}

func NewBadGateway(message string) *StatusError {
	return newStatusError(http.StatusBadGateway, "BadGateway", message, nil)
}

// NewTooManyRequests returns a 429 TooManyRequests. Used for upstream
// rate-limit pass-through (lib/ai ErrCodeRateLimit) and local quota
// enforcement when the limit is per-minute / per-second, not per-period.
func NewTooManyRequests(message string) *StatusError {
	return newStatusError(http.StatusTooManyRequests, "TooManyRequests", message, nil)
}

// NewClientClosedRequest returns a 499 Client Closed Request (Nginx
// extension; widely interpreted by load balancers and SDKs). Used when
// the client aborted before the upstream finished, so retry semantics
// differ from a real server-side failure.
func NewClientClosedRequest(message string) *StatusError {
	return newStatusError(499, "ClientClosedRequest", message, nil)
}

func NewServiceUnavailable(message string) *StatusError {
	return newStatusError(http.StatusServiceUnavailable, "ServiceUnavailable", message, nil)
}

// NewRequestEntityTooLarge returns a 413 RequestEntityTooLarge.
func NewRequestEntityTooLarge(message string) *StatusError {
	return newStatusError(http.StatusRequestEntityTooLarge, "RequestEntityTooLarge", message, nil)
}

func NewGatewayTimeout(message string) *StatusError {
	return newStatusError(http.StatusGatewayTimeout, "GatewayTimeout", message, nil)
}

func IsNotFound(err error) bool {
	if se, ok := errors.AsType[*StatusError](err); ok {
		return se.Status == http.StatusNotFound
	}
	return false
}

func IsConflict(err error) bool {
	if se, ok := errors.AsType[*StatusError](err); ok {
		return se.Status == http.StatusConflict
	}
	return false
}

func IsForbidden(err error) bool {
	if se, ok := errors.AsType[*StatusError](err); ok {
		return se.Status == http.StatusForbidden
	}
	return false
}

// FromDomain translates a domain-level error (pgerrors sentinel, optionally
// wrapped around a *pgconn.PgError) into a HTTP *StatusError. Returns nil when
// err does not match any known domain category.
//
// If err's chain carries a *pgconn.PgError, the mapping reproduces the legacy
// CheckPGError output byte-for-byte using resource as the human-readable name.
// Otherwise the sentinel tag drives the HTTP status and err.Error() becomes the
// response message (callers are expected to have attached their own context at
// the call site, e.g. fmt.Errorf("namespace %d: %w", id, pgerrors.ErrNotFound)).
func FromDomain(err error, resource string) *StatusError {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "22001":
			return NewBadRequest("value too long for "+resource, nil)
		case "23001", "23503":
			if pgErr.Detail != "" {
				detail := strings.ReplaceAll(pgErr.Detail, "\"", "")
				return NewConflictMessage(fmt.Sprintf("cannot delete %s: %s", resource, detail))
			}
			return NewConflictMessage(fmt.Sprintf("cannot delete %s: still referenced by other resources", resource))
		case "23505":
			return NewConflict(resource, pgErr.ConstraintName)
		}
	}
	switch {
	case errors.Is(err, pgerrors.ErrNotFound):
		return newStatusError(http.StatusNotFound, "NotFound", err.Error(), nil)
	case errors.Is(err, pgerrors.ErrConflict):
		return NewConflictMessage(err.Error())
	case errors.Is(err, pgerrors.ErrBadRequest):
		return NewBadRequest(err.Error(), nil)
	case errors.Is(err, pgerrors.ErrForbidden):
		return NewForbidden(err.Error())
	}
	return nil
}

// CheckPGError converts common PostgreSQL errors to user-friendly API errors.
// If the error is not a recognized PG error, it is returned unchanged.
//
// Deprecated: stores should return errors tagged with pgerrors sentinels (via
// pgerrors.CheckPG) and let the REST layer call FromDomain. This function
// remains for existing call sites during the staged migration described in
// docs/plans/2026-04-23-apis-clean-issues.md (issue 4).
func CheckPGError(err error, resource string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "22001": // string_data_right_truncation
			return NewBadRequest("value too long for "+resource, nil)
		case "23001", "23503": // restrict_violation, foreign_key_violation
			if pgErr.Detail != "" {
				detail := strings.ReplaceAll(pgErr.Detail, "\"", "")
				return NewConflictMessage(fmt.Sprintf("cannot delete %s: %s", resource, detail))
			}
			return NewConflictMessage(fmt.Sprintf("cannot delete %s: still referenced by other resources", resource))
		case "23505": // unique_violation
			return NewConflict(resource, pgErr.ConstraintName)
		}
	}
	return err
}
