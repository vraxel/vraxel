package store_test

import (
	"context"
	"testing"
	"time"

	"vraxel.io/vraxel/pkg/apis/iam/store"
	"vraxel.io/vraxel/pkg/db/dbtest"
)

func seedViewerRole(t *testing.T, s store.Stores) {
	t.Helper()
	ctx := context.Background()
	r, err := s.Role.Create(ctx, store.RoleCreateInput{
		Name: store.RolePlatformViewer, DisplayName: "Platform Viewer", Scope: "platform", Builtin: true,
	})
	if err != nil {
		t.Fatalf("seed viewer role: %v", err)
	}
	if err := s.Role.SetPermissionRules(ctx, r.ID, []string{"*:list", "*:get"}); err != nil {
		t.Fatalf("set rules: %v", err)
	}
}

func TestRegisterLocalBindsDefaultRole(t *testing.T) {
	s := store.NewStores(dbtest.New(t))
	ctx := context.Background()
	seedViewerRole(t, s)

	u, err := s.Registration.RegisterLocal(ctx, store.RegisterLocalInput{
		Username: "newbie", Email: "newbie@vraxel.io", DisplayName: "Newbie",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuv", DefaultRoleName: store.RolePlatformViewer,
	})
	if err != nil {
		t.Fatalf("RegisterLocal: %v", err)
	}
	if u.Username != "newbie" || u.Status != "active" {
		t.Fatalf("unexpected user: %+v", u)
	}

	rules, err := s.RoleBinding.LoadUserPermissionRules(ctx, u.ID)
	if err != nil || len(rules) == 0 {
		t.Fatalf("expected default-role permission rules, got %v (err %v)", rules, err)
	}

	// Duplicate username surfaces as a conflict the handler maps to 409.
	_, err = s.Registration.RegisterLocal(ctx, store.RegisterLocalInput{
		Username: "newbie", Email: "other@vraxel.io", PasswordHash: "x", DefaultRoleName: store.RolePlatformViewer,
	})
	if !store.IsConflict(err) {
		t.Fatalf("duplicate username: want conflict, got %v", err)
	}
}

func TestFindOrCreateSocial(t *testing.T) {
	s := store.NewStores(dbtest.New(t))
	ctx := context.Background()
	seedViewerRole(t, s)

	in := store.SocialLoginInput{
		Provider: "github", Subject: "42", Email: "ghuser@example.com",
		Username: "github_42", DisplayName: "GH User", DefaultRoleName: store.RolePlatformViewer,
	}
	u1, err := s.Registration.FindOrCreateSocial(ctx, in)
	if err != nil {
		t.Fatalf("first social login: %v", err)
	}

	// Second login with the same identity returns the same user, no duplicate.
	u2, err := s.Registration.FindOrCreateSocial(ctx, in)
	if err != nil {
		t.Fatalf("second social login: %v", err)
	}
	if u1.ID != u2.ID {
		t.Fatalf("expected same user on re-login: %d vs %d", u1.ID, u2.ID)
	}

	// A different provider identity with the same verified email links to the
	// existing user instead of creating a duplicate.
	u3, err := s.Registration.FindOrCreateSocial(ctx, store.SocialLoginInput{
		Provider: "google", Subject: "g-1", Email: "ghuser@example.com",
		Username: "google_g-1", DefaultRoleName: store.RolePlatformViewer,
	})
	if err != nil {
		t.Fatalf("link by email: %v", err)
	}
	if u3.ID != u1.ID {
		t.Fatalf("expected email link to reuse user: %d vs %d", u3.ID, u1.ID)
	}

	// Builtin (seeded admin) accounts must never auto-link by email: the
	// default admin email is a placeholder the deployer may not own.
	admin := newUser(t, s, "admin-it")
	if err := s.User.SetBuiltin(ctx, admin.ID, true); err != nil {
		t.Fatalf("set builtin: %v", err)
	}
	_, err = s.Registration.FindOrCreateSocial(ctx, store.SocialLoginInput{
		Provider: "google", Subject: "g-evil", Email: admin.Email,
		Username: "google_g-evil", DefaultRoleName: store.RolePlatformViewer,
	})
	if err == nil {
		t.Fatalf("expected refusal to link social identity to builtin user")
	}
}

func TestOAuthStateRoundtrip(t *testing.T) {
	s := store.NewStores(dbtest.New(t))
	ctx := context.Background()

	if err := s.OAuthState.Create(ctx, "st-1", "github", "req-1", time.Minute); err != nil {
		t.Fatalf("create state: %v", err)
	}
	row, err := s.OAuthState.Consume(ctx, "st-1")
	if err != nil || row.RequestID != "req-1" || row.Provider != "github" {
		t.Fatalf("consume: %+v, %v", row, err)
	}
	// Single-use: a second consume of the same state fails.
	if _, err := s.OAuthState.Consume(ctx, "st-1"); err == nil {
		t.Fatalf("expected error on second consume")
	}
}
