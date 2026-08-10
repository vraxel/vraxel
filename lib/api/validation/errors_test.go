package validation

import "testing"

func TestErrorListError(t *testing.T) {
	errs := ErrorList{
		{Field: "spec.email", Message: "is required"},
		{Field: "spec.username", Message: "is too short"},
	}
	msg := errs.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestErrorListHasErrors(t *testing.T) {
	var errs ErrorList
	if errs.HasErrors() {
		t.Error("empty list should not have errors")
	}
	errs = append(errs, FieldError{Field: "f", Message: "m"})
	if !errs.HasErrors() {
		t.Error("non-empty list should have errors")
	}
}
