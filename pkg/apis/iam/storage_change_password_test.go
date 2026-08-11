package iam

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// actionCtx builds a typed action context targeting item uid.
func actionCtx(uid int64) apiserver.Ctx {
	c := testCtx()
	c.ID = uid
	return c
}

// --- TestChangePassword_Success ---

func TestChangePassword_Success(t *testing.T) {
	dbUser := testUser(1, "alice", "alice@example.com")

	var capturedHash string
	var revokedUserID int64

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			if id != 1 {
				t.Fatalf("expected id 1, got %d", id)
			}
			return dbUser, nil
		},
		GetUserForAuthFn: func(ctx context.Context, identifier string) (*modstore.UserAuthRow, error) {
			if identifier != "alice" {
				t.Fatalf("expected identifier 'alice', got %q", identifier)
			}
			return &modstore.UserAuthRow{
				ID:           1,
				Username:     "alice",
				PasswordHash: "oldhash",
				Status:       "active",
			}, nil
		},
		SetPasswordHashFn: func(ctx context.Context, id int64, hash string) error {
			if id != 1 {
				t.Fatalf("expected id 1, got %d", id)
			}
			capturedHash = hash
			return nil
		},
	}

	refreshStore := &mockRefreshTokenStore{
		RevokeByUserIDFn: func(ctx context.Context, userID int64) error {
			revokedUserID = userID
			return nil
		},
	}

	hashPasswd := func(password string) (string, error) {
		return "newhash-" + password, nil
	}
	verifyPasswd := func(password, hash string) error {
		if hash != "oldhash" {
			return errors.New("wrong hash")
		}
		return nil
	}

	action := NewChangePasswordAction(userStore, refreshStore, hashPasswd, verifyPasswd)

	resp, err := action(actionCtx(1), &ChangePasswordRequest{
		OldPassword: "OldPass123",
		NewPassword: "NewPass123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "Success" {
		t.Errorf("expected status 'Success', got %q", resp.Status)
	}
	if resp.Message != "password changed successfully" {
		t.Errorf("expected message 'password changed successfully', got %q", resp.Message)
	}

	// Verify new password was hashed and stored
	if capturedHash != "newhash-NewPass123" {
		t.Errorf("expected captured hash 'newhash-NewPass123', got %q", capturedHash)
	}

	// Verify refresh tokens were revoked
	if revokedUserID != 1 {
		t.Errorf("expected revoked user ID 1, got %d", revokedUserID)
	}
}

// --- TestChangePassword_WrongOldPassword ---

func TestChangePassword_WrongOldPassword(t *testing.T) {
	dbUser := testUser(1, "alice", "alice@example.com")

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return dbUser, nil
		},
		GetUserForAuthFn: func(ctx context.Context, identifier string) (*modstore.UserAuthRow, error) {
			return &modstore.UserAuthRow{
				ID:           1,
				Username:     "alice",
				PasswordHash: "oldhash",
				Status:       "active",
			}, nil
		},
	}

	hashPasswd := func(password string) (string, error) {
		return "newhash", nil
	}
	verifyPasswd := func(password, hash string) error {
		return errors.New("password mismatch")
	}

	action := NewChangePasswordAction(userStore, nil, hashPasswd, verifyPasswd)

	_, err := action(actionCtx(1), &ChangePasswordRequest{
		OldPassword: "WrongPass1",
		NewPassword: "NewPass123",
	})
	if err == nil {
		t.Fatal("expected error for wrong old password, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
	if statusErr.Message != "old password is incorrect" {
		t.Errorf("expected message 'old password is incorrect', got %q", statusErr.Message)
	}
}

// --- TestChangePassword_WeakNewPassword ---

func TestChangePassword_WeakNewPassword(t *testing.T) {
	// "weak" is too short (<8 chars), no uppercase, no digit — fails ValidatePassword
	action := NewChangePasswordAction(nil, nil, nil, nil)

	_, err := action(actionCtx(1), &ChangePasswordRequest{
		OldPassword: "OldPass123",
		NewPassword: "weak",
	})
	if err == nil {
		t.Fatal("expected error for weak password, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
	if statusErr.Message != "validation failed" {
		t.Errorf("expected message 'validation failed', got %q", statusErr.Message)
	}
}

// --- TestChangePassword_UserNotFound ---

func TestChangePassword_UserNotFound(t *testing.T) {
	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return nil, apierrors.NewNotFound("user", "999")
		},
	}

	hashPasswd := func(password string) (string, error) {
		return "hash", nil
	}
	verifyPasswd := func(password, hash string) error {
		return nil
	}

	action := NewChangePasswordAction(userStore, nil, hashPasswd, verifyPasswd)

	_, err := action(actionCtx(999), &ChangePasswordRequest{
		OldPassword: "OldPass123",
		NewPassword: "NewPass123",
	})
	if err == nil {
		t.Fatal("expected error for user not found, got nil")
	}

	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

// --- TestChangePassword_MissingFields ---

func TestChangePassword_MissingFields(t *testing.T) {
	// A missing newPassword is a pure validation failure (no store access), so
	// nil stores are fine. Whether oldPassword is required is decided later
	// against the account's stored hash and is covered by the store-backed
	// tests below, not here.
	action := NewChangePasswordAction(nil, nil, nil, nil)

	tests := []struct {
		name string
		req  ChangePasswordRequest
	}{
		{
			name: "empty newPassword",
			req:  ChangePasswordRequest{OldPassword: "OldPass123", NewPassword: ""},
		},
		{
			name: "both empty",
			req:  ChangePasswordRequest{OldPassword: "", NewPassword: ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := action(actionCtx(1), &tc.req)
			if err == nil {
				t.Fatal("expected error for missing fields, got nil")
			}

			statusErr, ok := err.(*apierrors.StatusError)
			if !ok {
				t.Fatalf("expected *StatusError, got %T", err)
			}
			if statusErr.Status != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", statusErr.Status)
			}
			if statusErr.Message != "newPassword is required" {
				t.Errorf("expected message 'newPassword is required', got %q", statusErr.Message)
			}
		})
	}
}

// --- TestChangePassword_PasswordlessSetsInitial ---
// A social-login user (empty password_hash) sets a first password with no old
// password. Without the passwordless carve-out the empty-hash verify would
// always fail and lock them out of ever having a password.
func TestChangePassword_PasswordlessSetsInitial(t *testing.T) {
	dbUser := testUser(1, "gh-user", "gh@example.com")

	var capturedHash string
	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return dbUser, nil
		},
		GetUserForAuthFn: func(ctx context.Context, identifier string) (*modstore.UserAuthRow, error) {
			return &modstore.UserAuthRow{ID: 1, Username: "gh-user", PasswordHash: "", Status: "active"}, nil
		},
		SetPasswordHashFn: func(ctx context.Context, id int64, hash string) error {
			capturedHash = hash
			return nil
		},
	}

	hashPasswd := func(password string) (string, error) { return "newhash-" + password, nil }
	verifyPasswd := func(password, hash string) error {
		t.Fatal("verifyPasswd must not be called for a passwordless account")
		return nil
	}

	action := NewChangePasswordAction(userStore, nil, hashPasswd, verifyPasswd)

	resp, err := action(actionCtx(1), &ChangePasswordRequest{OldPassword: "", NewPassword: "NewPass123"})
	if err != nil {
		t.Fatalf("unexpected error setting initial password: %v", err)
	}
	if resp.Status != "Success" {
		t.Errorf("expected Success, got %q", resp.Status)
	}
	if capturedHash != "newhash-NewPass123" {
		t.Errorf("expected password to be set, got hash %q", capturedHash)
	}
}

// --- TestChangePassword_ExistingRequiresOld ---
// An account that already has a password must still supply a non-empty old one.
func TestChangePassword_ExistingRequiresOld(t *testing.T) {
	dbUser := testUser(1, "alice", "alice@example.com")
	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) { return dbUser, nil },
		GetUserForAuthFn: func(ctx context.Context, identifier string) (*modstore.UserAuthRow, error) {
			return &modstore.UserAuthRow{ID: 1, Username: "alice", PasswordHash: "oldhash", Status: "active"}, nil
		},
	}
	action := NewChangePasswordAction(userStore, nil, func(string) (string, error) { return "x", nil }, func(string, string) error { return nil })

	_, err := action(actionCtx(1), &ChangePasswordRequest{OldPassword: "", NewPassword: "NewPass123"})
	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T (%v)", err, err)
	}
	if statusErr.Message != "oldPassword is required" {
		t.Errorf("expected 'oldPassword is required', got %q", statusErr.Message)
	}
}
