package iam

import (
	"context"
	"net/http"
	"testing"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/oidc"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// --- TestResetPassword_Success ---

func TestResetPassword_Success(t *testing.T) {
	dbUser := testUser(2, "bob", "bob@example.com")

	var capturedHash string
	var revokedUserID int64

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			if id != 2 {
				t.Fatalf("expected id 2, got %d", id)
			}
			return dbUser, nil
		},
		SetPasswordHashFn: func(ctx context.Context, id int64, hash string) error {
			if id != 2 {
				t.Fatalf("expected id 2, got %d", id)
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

	action := NewResetPasswordAction(userStore, refreshStore, hashPasswd)

	// Caller (admin id=1) resets target user (id=2): allowed.
	resp, err := action(callerCtx(1, 2), &ResetPasswordRequest{NewPassword: "ResetPass123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != "Success" {
		t.Errorf("expected status 'Success', got %q", resp.Status)
	}
	if resp.Message != "password reset successfully" {
		t.Errorf("expected message 'password reset successfully', got %q", resp.Message)
	}

	if capturedHash != "newhash-ResetPass123" {
		t.Errorf("expected captured hash 'newhash-ResetPass123', got %q", capturedHash)
	}

	if revokedUserID != 2 {
		t.Errorf("expected revoked user ID 2, got %d", revokedUserID)
	}
}

// --- TestResetPassword_RejectSelf ---

func TestResetPassword_RejectSelf(t *testing.T) {
	action := NewResetPasswordAction(&mockUserStore{}, nil, nil)

	// Caller id=5 tries to reset id=5: rejected before any store call.
	_, err := action(callerCtx(5, 5), &ResetPasswordRequest{NewPassword: "ResetPass123"})
	if err == nil {
		t.Fatal("expected error when resetting self, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
	if statusErr.Message != "cannot reset your own password; use change-password instead" {
		t.Errorf("expected self-reject message, got %q", statusErr.Message)
	}
}

// --- TestResetPassword_WeakNewPassword ---

func TestResetPassword_WeakNewPassword(t *testing.T) {
	action := NewResetPasswordAction(nil, nil, nil)

	_, err := action(actionCtx(2), &ResetPasswordRequest{NewPassword: "weak"})
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

// --- TestResetPassword_UserNotFound ---

func TestResetPassword_UserNotFound(t *testing.T) {
	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return nil, apierrors.NewNotFound("user", "999")
		},
	}

	hashPasswd := func(password string) (string, error) {
		return "hash", nil
	}

	action := NewResetPasswordAction(userStore, nil, hashPasswd)

	_, err := action(callerCtx(1, 999), &ResetPasswordRequest{NewPassword: "ResetPass123"})
	if err == nil {
		t.Fatal("expected error for user not found, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

// --- TestResetPassword_MissingNewPassword ---

func TestResetPassword_MissingNewPassword(t *testing.T) {
	action := NewResetPasswordAction(nil, nil, nil)

	_, err := action(actionCtx(2), &ResetPasswordRequest{NewPassword: ""})
	if err == nil {
		t.Fatal("expected error for missing newPassword, got nil")
	}
	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
	if statusErr.Message != "newPassword is required" {
		t.Errorf("expected 'newPassword is required', got %q", statusErr.Message)
	}
}

// --- TestResetPassword_RevokeFailureNonFatal ---

// SetPasswordHash succeeded but RevokeByUserID failed: handler should still return success
// (token revocation is best-effort, mirrors change-password handler behavior).
func TestResetPassword_RevokeFailureNonFatal(t *testing.T) {
	dbUser := testUser(3, "carol", "carol@example.com")

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return dbUser, nil
		},
		SetPasswordHashFn: func(ctx context.Context, id int64, hash string) error {
			return nil
		},
	}

	refreshStore := &mockRefreshTokenStore{
		RevokeByUserIDFn: func(ctx context.Context, userID int64) error {
			return apierrors.NewInternalError(nil)
		},
	}

	hashPasswd := func(password string) (string, error) {
		return "hash", nil
	}

	action := NewResetPasswordAction(userStore, refreshStore, hashPasswd)

	resp, err := action(callerCtx(1, 3), &ResetPasswordRequest{NewPassword: "ResetPass123"})
	if err != nil {
		t.Fatalf("expected success despite revoke failure, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected *StatusResponse, got nil")
	}
}

// callerCtx builds a typed action context targeting item uid with the
// caller userID injected into context the same way the OIDC auth
// middleware does, so handler-level "reject self" logic is testable.
func callerCtx(callerID, targetUID int64) apiserver.Ctx {
	r, _ := http.NewRequest("POST", "/", nil)
	r = oidc.WithUserID(r, callerID)
	c := apiserver.Ctx{Context: r.Context(), ID: targetUID}
	c.User.ID = callerID
	return c
}
