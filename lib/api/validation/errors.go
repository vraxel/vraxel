package validation

import (
	"fmt"
	"strings"
)

// FieldError represents a validation error on a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrorList is a collection of field validation errors.
type ErrorList []FieldError

func (e ErrorList) Error() string {
	var msgs []string
	for _, fe := range e {
		msgs = append(msgs, fmt.Sprintf("%s: %s", fe.Field, fe.Message))
	}
	return strings.Join(msgs, "; ")
}

func (e ErrorList) HasErrors() bool {
	return len(e) > 0
}
