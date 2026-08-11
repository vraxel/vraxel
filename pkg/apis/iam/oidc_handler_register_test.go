package iam

import (
	"strings"
	"testing"
)

func TestValidateRegistration(t *testing.T) {
	cases := []struct {
		name            string
		username, email string
		password        string
		displayName     string
		wantErr         bool
	}{
		{"ok", "alice", "alice@vraxel.io", "Str0ngPass", "Alice", false},
		{"short username", "ab", "a@b.io", "Str0ngPass", "", true},
		{"bad username chars", "al ice", "a@b.io", "Str0ngPass", "", true},
		{"empty email", "alice", "", "Str0ngPass", "", true},
		{"bad email", "alice", "not-an-email", "Str0ngPass", "", true},
		{"weak password no digit", "alice", "a@b.io", "Password", "", true},
		{"weak password too short", "alice", "a@b.io", "Ab1", "", true},
		{"displayName too long", "alice", "a@b.io", "Str0ngPass", strings.Repeat("名", 129), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := validateRegistration(c.username, c.email, c.password, c.displayName)
			if c.wantErr && msg == "" {
				t.Fatalf("expected validation error, got none")
			}
			if !c.wantErr && msg != "" {
				t.Fatalf("expected no error, got %q", msg)
			}
		})
	}
}

func TestSocialUsername(t *testing.T) {
	cases := []struct{ provider, subject, want string }{
		{"github", "12345", "github_12345"},
		{"google", "abc-DEF_9", "google_abc-DEF_9"},
		{"github", "a/b|c", "github_a-b-c"},
	}
	for _, c := range cases {
		if got := socialUsername(c.provider, c.subject); got != c.want {
			t.Fatalf("socialUsername(%q,%q) = %q, want %q", c.provider, c.subject, got, c.want)
		}
	}
	// Long subjects are truncated to the username length limit.
	long := socialUsername("github", string(make([]byte, 100)))
	if len(long) > 50 {
		t.Fatalf("username not truncated: len %d", len(long))
	}
}
