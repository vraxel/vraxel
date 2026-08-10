package iam

import "testing"

func TestValidatePermissionPattern(t *testing.T) {
	valid := []string{
		"*:*",
		"iam:*",
		"iam:namespaces:*",
		"iam:namespaces:users:*",
		"*:list",
		"*:get",
		"*:deleteCollection",
		"*:change-password",
		"iam:users:list",
		"iam:namespaces:users:create",
		"app:services:delete",
		"iam:users:change-password",
	}
	for _, p := range valid {
		if errs := ValidatePermissionPattern(p); len(errs) > 0 {
			t.Errorf("ValidatePermissionPattern(%q) = %v, want valid", p, errs)
		}
	}

	invalid := []string{
		"",
		"*",
		"iam:",
		":list",
		"**:*",
		"IAM:*",
		"iam",
		"iam:users:",
		"iam::users:list",
		"*:*:*",
		"iam:users:list:extra:*:bad",
	}
	for _, p := range invalid {
		if errs := ValidatePermissionPattern(p); len(errs) == 0 {
			t.Errorf("ValidatePermissionPattern(%q) = valid, want invalid", p)
		}
	}
}
