package iam

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"vraxel.io/vraxel/lib/config"
	"vraxel.io/vraxel/lib/logger"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// SeedAdmin ensures the initial admin user exists. If the user already exists, it is a no-op.
func SeedAdmin(ctx context.Context, userStore modstore.UserStore, cfg config.AdminConfig) error {
	_, err := userStore.GetByUsername(ctx, cfg.Username)
	if err == nil {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Password), 10)
	if err != nil {
		return err
	}

	user, err := userStore.Create(ctx, modstore.UserCreateInput{
		Username:    cfg.Username,
		Email:       cfg.Email,
		DisplayName: cfg.DisplayName,
		Phone:       cfg.Phone,
		Status:      "active",
	})
	if err != nil {
		return err
	}

	if err := userStore.SetPasswordHash(ctx, user.ID, string(hash)); err != nil {
		return err
	}

	// Mark the seed admin as builtin so the delete path refuses removal.
	// The migration covers existing installs; this keeps fresh installs
	// in sync without relying on a follow-up migration.
	if err := userStore.SetBuiltin(ctx, user.ID, true); err != nil {
		return err
	}

	logger.Infof("seeded admin user — username: %s, password: %s", cfg.Username, cfg.Password)
	return nil
}
