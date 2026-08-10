package iam

import (
	"context"
	"fmt"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/logger"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// SyncPermissions persists apiserver-derived permission records through
// the per-module SyncModule pipeline (upsert + module-prefix-scoped prune
// in one transaction). Codes, methods, paths and scopes are derived at
// registration by lib/apiserver, byte-compatible with the historical
// URL-tree derivation.
func SyncPermissions(ctx context.Context, permStore modstore.PermissionStore, byModule map[string][]apiserver.PermRecord) error {
	for module, records := range byModule {
		inputs := make([]modstore.PermissionUpsertInput, len(records))
		for i, rec := range records {
			inputs[i] = modstore.PermissionUpsertInput{
				Code:        rec.Code,
				Method:      rec.Method,
				Path:        rec.Path,
				Scope:       rec.Scope,
				Description: rec.Description,
			}
		}
		if err := permStore.SyncModule(ctx, module+":", inputs); err != nil {
			return fmt.Errorf("sync v2 permissions for module %s: %w", module, err)
		}
		logger.Infof("synced %d permissions for module %q", len(inputs), module)
	}
	return nil
}
