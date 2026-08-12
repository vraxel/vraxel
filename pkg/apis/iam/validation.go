package iam

import (
	"fmt"
	"net/mail"
	"regexp"
	"unicode/utf8"

	"vraxel.io/vraxel/lib/api/validation"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// seg matches a permission code segment: must start with lowercase, allows camelCase for verbs like "deleteCollection".
const seg = `[a-z][a-zA-Z0-9-]*`

var (
	usernameRegexp      = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,50}$`)
	phoneRegexp         = regexp.MustCompile(`^1[3-9]\d{9}$`)
	workspaceNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{1,48}[a-zA-Z0-9]$`)
	namespaceNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{1,48}[a-zA-Z0-9]$`)
	passwordUpperRegexp = regexp.MustCompile(`[A-Z]`)
	passwordLowerRegexp = regexp.MustCompile(`[a-z]`)
	passwordDigitRegexp = regexp.MustCompile(`[0-9]`)

	// validPatternRegexp validates permission rule patterns:
	//   *:*                          → match all
	//   iam:*  iam:namespaces:*      → prefix wildcard
	//   *:list  *:deleteCollection   → suffix wildcard
	//   iam:users:list               → exact match (at least 2 segments)
	validPatternRegexp = regexp.MustCompile(
		`^(\*:\*` +
			`|(\*|` + seg + `)((:` + seg + `)*):\*` +
			`|\*:` + seg + `((:` + seg + `)*)` +
			`|` + seg + `(:` + seg + `)+)$`,
	)
)

// ValidateUserCreate validates a UserSpec for creation.
func ValidateUserCreate(spec *UserSpec) validation.ErrorList {
	var errs validation.ErrorList

	if spec.Username == "" {
		errs = append(errs, validation.FieldError{Field: "spec.username", Message: "is required"})
	} else if !usernameRegexp.MatchString(spec.Username) {
		errs = append(errs, validation.FieldError{Field: "spec.username", Message: "must be 3-50 alphanumeric characters, underscores, or hyphens"})
	}

	if spec.Email == "" {
		errs = append(errs, validation.FieldError{Field: "spec.email", Message: "is required"})
	} else if len(spec.Email) > 255 {
		errs = append(errs, validation.FieldError{Field: "spec.email", Message: "must be at most 255 characters"})
	} else if _, err := mail.ParseAddress(spec.Email); err != nil {
		errs = append(errs, validation.FieldError{Field: "spec.email", Message: "is not a valid email address"})
	}

	if spec.Phone == "" {
		errs = append(errs, validation.FieldError{Field: "spec.phone", Message: "is required"})
	} else if !phoneRegexp.MatchString(spec.Phone) {
		errs = append(errs, validation.FieldError{Field: "spec.phone", Message: "must be a valid Chinese mobile number (e.g. 13800138000)"})
	}

	if utf8.RuneCountInString(spec.DisplayName) > 128 {
		errs = append(errs, validation.FieldError{Field: "spec.displayName", Message: "must be at most 128 characters"})
	}

	if spec.Status != "" && spec.Status != "active" && spec.Status != "inactive" {
		errs = append(errs, validation.FieldError{Field: "spec.status", Message: "must be 'active' or 'inactive'"})
	}

	return errs
}

// ValidateUserUpdate validates a UserSpec for full update.
func ValidateUserUpdate(spec *UserSpec) validation.ErrorList {
	return ValidateUserCreate(spec)
}

// ValidateWorkspaceCreate validates workspace creation.
func ValidateWorkspaceCreate(name string, spec *WorkspaceSpec) validation.ErrorList {
	var errs validation.ErrorList

	if name == "" {
		errs = append(errs, validation.FieldError{Field: "metadata.name", Message: "is required"})
	} else if !workspaceNameRegexp.MatchString(name) {
		errs = append(errs, validation.FieldError{Field: "metadata.name", Message: "must be 3-50 lowercase alphanumeric characters or hyphens"})
	}

	if spec.OwnerID == "" {
		errs = append(errs, validation.FieldError{Field: "spec.ownerId", Message: "is required"})
	}

	if utf8.RuneCountInString(spec.DisplayName) > 128 {
		errs = append(errs, validation.FieldError{Field: "spec.displayName", Message: "must be at most 128 characters"})
	}

	if utf8.RuneCountInString(spec.Description) > 1000 {
		errs = append(errs, validation.FieldError{Field: "spec.description", Message: "must be at most 1000 characters"})
	}

	if spec.Status != "" && spec.Status != "active" && spec.Status != "inactive" {
		errs = append(errs, validation.FieldError{Field: "spec.status", Message: "must be 'active' or 'inactive'"})
	}

	return errs
}

// ValidateNamespaceCreate validates namespace creation.
func ValidateNamespaceCreate(name string, spec *NamespaceSpec) validation.ErrorList {
	var errs validation.ErrorList

	if name == "" {
		errs = append(errs, validation.FieldError{Field: "metadata.name", Message: "is required"})
	} else if !namespaceNameRegexp.MatchString(name) {
		errs = append(errs, validation.FieldError{Field: "metadata.name", Message: "must be 3-50 lowercase alphanumeric characters or hyphens"})
	}

	if spec.WorkspaceID == "" {
		errs = append(errs, validation.FieldError{Field: "spec.workspaceId", Message: "is required"})
	}

	if spec.OwnerID == "" {
		errs = append(errs, validation.FieldError{Field: "spec.ownerId", Message: "is required"})
	}

	if utf8.RuneCountInString(spec.DisplayName) > 128 {
		errs = append(errs, validation.FieldError{Field: "spec.displayName", Message: "must be at most 128 characters"})
	}

	if utf8.RuneCountInString(spec.Description) > 1000 {
		errs = append(errs, validation.FieldError{Field: "spec.description", Message: "must be at most 1000 characters"})
	}

	if spec.Visibility != "" && spec.Visibility != "public" && spec.Visibility != "private" {
		errs = append(errs, validation.FieldError{Field: "spec.visibility", Message: "must be 'public' or 'private'"})
	}

	if spec.MaxMembers < 0 || spec.MaxMembers > 1000000 {
		errs = append(errs, validation.FieldError{Field: "spec.maxMembers", Message: "must be between 0 and 1000000"})
	}

	return errs
}

// ValidateWorkspaceUpdate validates workspace fields for update.
func ValidateWorkspaceUpdate(spec *WorkspaceSpec) validation.ErrorList {
	var errs validation.ErrorList

	if utf8.RuneCountInString(spec.DisplayName) > 128 {
		errs = append(errs, validation.FieldError{Field: "spec.displayName", Message: "must be at most 128 characters"})
	}

	if utf8.RuneCountInString(spec.Description) > 1000 {
		errs = append(errs, validation.FieldError{Field: "spec.description", Message: "must be at most 1000 characters"})
	}

	if spec.Status != "" && spec.Status != "active" && spec.Status != "inactive" {
		errs = append(errs, validation.FieldError{Field: "spec.status", Message: "must be 'active' or 'inactive'"})
	}
	return errs
}

// ValidateNamespaceUpdate validates namespace fields for update.
func ValidateNamespaceUpdate(spec *NamespaceSpec) validation.ErrorList {
	var errs validation.ErrorList

	if utf8.RuneCountInString(spec.DisplayName) > 128 {
		errs = append(errs, validation.FieldError{Field: "spec.displayName", Message: "must be at most 128 characters"})
	}

	if utf8.RuneCountInString(spec.Description) > 1000 {
		errs = append(errs, validation.FieldError{Field: "spec.description", Message: "must be at most 1000 characters"})
	}

	if spec.Visibility != "" && spec.Visibility != "public" && spec.Visibility != "private" {
		errs = append(errs, validation.FieldError{Field: "spec.visibility", Message: "must be 'public' or 'private'"})
	}
	if spec.MaxMembers < 0 || spec.MaxMembers > 1000000 {
		errs = append(errs, validation.FieldError{Field: "spec.maxMembers", Message: "must be between 0 and 1000000"})
	}
	if spec.Status != "" && spec.Status != "active" && spec.Status != "inactive" {
		errs = append(errs, validation.FieldError{Field: "spec.status", Message: "must be 'active' or 'inactive'"})
	}
	return errs
}

var (
	roleNameRegexp  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{1,48}[a-zA-Z0-9]$`)
	validRoleScopes = map[string]bool{modstore.ScopePlatform: true, modstore.ScopeWorkspace: true, modstore.ScopeNamespace: true}
)

// ValidateRoleCreate validates a RoleSpec for creation.
func ValidateRoleCreate(spec *RoleSpec) validation.ErrorList {
	var errs validation.ErrorList
	if spec.Name == "" {
		errs = append(errs, validation.FieldError{Field: "spec.name", Message: "is required"})
	} else if !roleNameRegexp.MatchString(spec.Name) {
		errs = append(errs, validation.FieldError{Field: "spec.name", Message: "must match ^[a-zA-Z0-9][a-zA-Z0-9_-]{1,48}[a-zA-Z0-9]$"})
	}
	if utf8.RuneCountInString(spec.DisplayName) > 128 {
		errs = append(errs, validation.FieldError{Field: "spec.displayName", Message: "must be at most 128 characters"})
	}
	if utf8.RuneCountInString(spec.Description) > 1000 {
		errs = append(errs, validation.FieldError{Field: "spec.description", Message: "must be at most 1000 characters"})
	}
	if !validRoleScopes[spec.Scope] {
		errs = append(errs, validation.FieldError{Field: "spec.scope", Message: "must be platform, workspace, or namespace"})
	}
	if len(spec.Rules) == 0 {
		errs = append(errs, validation.FieldError{Field: "spec.rules", Message: "must not be empty"})
	}
	for i, rule := range spec.Rules {
		if ruleErrs := ValidatePermissionPattern(rule); ruleErrs.HasErrors() {
			for _, e := range ruleErrs {
				errs = append(errs, validation.FieldError{
					Field:   fmt.Sprintf("spec.rules[%d]", i),
					Message: e.Message,
				})
			}
		}
	}
	return errs
}

// ValidateRoleUpdate validates a RoleSpec for update.
func ValidateRoleUpdate(spec *RoleSpec) validation.ErrorList {
	var errs validation.ErrorList
	if utf8.RuneCountInString(spec.DisplayName) > 128 {
		errs = append(errs, validation.FieldError{Field: "spec.displayName", Message: "must be at most 128 characters"})
	}
	if utf8.RuneCountInString(spec.Description) > 1000 {
		errs = append(errs, validation.FieldError{Field: "spec.description", Message: "must be at most 1000 characters"})
	}
	if len(spec.Rules) == 0 {
		errs = append(errs, validation.FieldError{Field: "spec.rules", Message: "must not be empty"})
	}
	for i, rule := range spec.Rules {
		if ruleErrs := ValidatePermissionPattern(rule); ruleErrs.HasErrors() {
			for _, e := range ruleErrs {
				errs = append(errs, validation.FieldError{
					Field:   fmt.Sprintf("spec.rules[%d]", i),
					Message: e.Message,
				})
			}
		}
	}
	return errs
}

// ValidateRuleScopes checks that each rule pattern matches at least one permission
// at the role's scope level. This ensures rules are not dead (matching nothing at the scope).
func ValidateRuleScopes(roleScope string, rules []string, codeScopes []modstore.PermissionCodeScope) validation.ErrorList {
	var errs validation.ErrorList

	// Collect codes at the role's scope
	scopedCodes := make([]string, 0)
	for _, cs := range codeScopes {
		if cs.Scope == roleScope {
			scopedCodes = append(scopedCodes, cs.Code)
		}
	}

	for i, rule := range rules {
		matched := false
		for _, code := range scopedCodes {
			if MatchPermission(rule, code) {
				matched = true
				break
			}
		}
		if !matched {
			errs = append(errs, validation.FieldError{
				Field:   fmt.Sprintf("spec.rules[%d]", i),
				Message: fmt.Sprintf("pattern %q matches no permissions at %s scope", rule, roleScope),
			})
		}
	}
	return errs
}

// ValidatePermissionPattern validates a permission rule pattern string.
func ValidatePermissionPattern(pattern string) validation.ErrorList {
	var errs validation.ErrorList
	if pattern == "" {
		errs = append(errs, validation.FieldError{Field: "pattern", Message: "cannot be empty"})
	} else if !validPatternRegexp.MatchString(pattern) {
		errs = append(errs, validation.FieldError{Field: "pattern", Message: "invalid pattern: " + pattern})
	}
	return errs
}

// ValidatePassword validates a password string.
//
// The 72-byte ceiling matches the hard limit of bcrypt
// (golang.org/x/crypto/bcrypt rejects > 72 bytes). len() is the byte
// length, which is what bcrypt actually counts -- so a multi-byte
// password (CJK etc.) at the boundary is rejected here, not later
// when hashing fails with an opaque error.
func ValidatePassword(password string) validation.ErrorList {
	var errs validation.ErrorList
	if len(password) < 8 || len(password) > 72 {
		errs = append(errs, validation.FieldError{Field: "spec.password", Message: "must be 8-72 bytes"})
		return errs
	}
	if !passwordUpperRegexp.MatchString(password) {
		errs = append(errs, validation.FieldError{Field: "spec.password", Message: "must contain at least one uppercase letter"})
	}
	if !passwordLowerRegexp.MatchString(password) {
		errs = append(errs, validation.FieldError{Field: "spec.password", Message: "must contain at least one lowercase letter"})
	}
	if !passwordDigitRegexp.MatchString(password) {
		errs = append(errs, validation.FieldError{Field: "spec.password", Message: "must contain at least one digit"})
	}
	return errs
}
