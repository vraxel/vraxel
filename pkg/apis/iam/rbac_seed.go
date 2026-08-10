package iam

import (
	"context"

	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/pkg/apis/iam/store"
)

// SeedRBAC upserts platform built-in roles, their permission rules, creates
// the initial platform-admin binding for the admin user, and ensures scoped
// built-in roles exist for all workspaces and namespaces - all in a single
// transaction. Idempotent: repeated calls update roles/rules and skip existing bindings.
func SeedRBAC(ctx context.Context, roleStore store.RoleStore) error {
	if err := roleStore.SeedRBAC(ctx, store.PlatformBuiltinRoles(), "admin"); err != nil {
		return err
	}
	logger.Infof("seeded platform built-in roles with initial bindings")
	return nil
}
