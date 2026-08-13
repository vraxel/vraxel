// Package pki owns the platform's master encryption key: the AES-256 key
// loaded from the database on boot, generated on first boot, from which
// other modules derive their own keys (the agent-token signing key today).
//
// It is deliberately NOT a REST module -- no rest.APIGroupInfo, no RBAC.
// The key never leaves the process, so there is nothing to expose over
// HTTP; the module exists to give the assembly layer a business-layer
// entry point instead of reaching into pki/store directly.
package pki

import (
	"context"
	"fmt"

	pkistore "vraxel.io/vraxel/pkg/apis/pki/store"
	"vraxel.io/vraxel/pkg/db"
)

// ModuleResult is what the assembly layer wires up.
type ModuleResult struct {
	// EncryptionKey is the 32-byte platform master key. Modules that need
	// a key of their own derive it from this one rather than storing a
	// second secret.
	EncryptionKey []byte
}

// NewModule loads the master key, generating and persisting one when the
// database has none yet.
func NewModule(ctx context.Context, database *db.DB) (ModuleResult, error) {
	key, err := pkistore.LoadOrGenerateEncryptionKey(ctx, database)
	if err != nil {
		return ModuleResult{}, fmt.Errorf("pki: %w", err)
	}
	return ModuleResult{EncryptionKey: key}, nil
}
