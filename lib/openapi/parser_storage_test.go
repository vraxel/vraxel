package openapi

import (
	"path/filepath"
	"testing"
)

// TestMergeStorageAnnotations characterizes mergeStorageAnnotations: scanning
// *Storage types, their methods, and standalone +openapi: functions, then
// merging derived/explicit paths, tag overrides, operation/action/customverb
// summaries, and path-operations into the matching TypeInfo. It is the safety
// net for decomposing that function (cognitive complexity 214) into helpers.
func TestMergeStorageAnnotations(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "demo", "doc.go"), `// +openapi:groupName=demo
// +openapi:groupVersion=v1
package demo
`)
	mustWrite(t, filepath.Join(root, "demo", "types.go"), `package demo

// +openapi:schema
type User struct{}

// +openapi:schema
type Member struct{}

// +openapi:schema
type Setting struct{}

// +openapi:tag=Accounts
type userStorage struct{}

// +openapi:summary=List all users
func (s *userStorage) List() {}

// +openapi:summary=Get a user
func (s *userStorage) Get() {}

type projectMemberStorage struct{}

// +openapi:summary=Create a member
func (s *projectMemberStorage) Create() {}

// +openapi:path=/custom/settings
// +openapi:noDerive
// +openapi:resource=Setting
type settingStorage struct{}

// +openapi:summary=Update a setting
func (s *settingStorage) Update() {}

// +openapi:action=reset-password
// +openapi:resource=User
// +openapi:summary=Reset a user password
func ResetPassword() {}

// +openapi:customverb=login
// +openapi:resource=User
// +openapi:summary=Log in
// +openapi:response=LoginResult
func Login() {}
`)

	groups, err := NewParser(root).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	user := findType(t, groups, "User")
	member := findType(t, groups, "Member")
	setting := findType(t, groups, "Setting")

	// String-valued outputs across all four merge blocks: tag override,
	// method summaries (empty / qualified / noDerive prefix), action, customverb.
	for _, c := range []struct{ name, got, want string }{
		{"User.Tag (tag override)", user.Tag, "Accounts"},
		{"User list summary (empty prefix)", user.OperationSummary["list"], "List all users"},
		{"User get summary (empty prefix)", user.OperationSummary["get"], "Get a user"},
		{"User action summary", user.ActionSummary["reset-password"], "Reset a user password"},
		{"User customverb summary", user.CustomVerbSummary["login"], "Log in"},
		{"User customverb response", user.CustomVerbResponse["login"], "LoginResult"},
		{"Member summary (qualified prefix)", member.OperationSummary["projects.members.create"], "Create a member"},
		{"Setting summary (noDerive prefix)", setting.OperationSummary["custom.settings.update"], "Update a setting"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	// Path merging: derived single-segment, derived multi-segment, and
	// noDerive (explicit path only, derived path suppressed).
	if !containsStr(user.Paths, "/users") {
		t.Errorf("User.Paths = %v, want to contain /users", user.Paths)
	}
	if !containsStr(member.Paths, "/projects/{projectId}/members") {
		t.Errorf("Member.Paths = %v, want to contain /projects/{projectId}/members", member.Paths)
	}
	if len(setting.Paths) != 1 || setting.Paths[0] != "/custom/settings" {
		t.Errorf("Setting.Paths = %v, want exactly [/custom/settings]", setting.Paths)
	}
}

func findType(t *testing.T, groups []GroupInfo, name string) TypeInfo {
	t.Helper()
	for _, g := range groups {
		for _, ti := range g.Types {
			if ti.Name == name {
				return ti
			}
		}
	}
	t.Fatalf("type %q not found in parsed groups", name)
	return TypeInfo{}
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
