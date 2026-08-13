package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/pkg/db"
)

// LoadOrGenerateEncryptionKey loads the AES-256 PKI encryption key
// from the database, or generates and stores a new one if none exists.
// Kept in the store implementation layer so pgx + generated imports do
// not leak into pki/v1/install.go or any business-layer file.
func LoadOrGenerateEncryptionKey(ctx context.Context, database *db.DB) ([]byte, error) {
	q := database.GetQueries()

	row, err := q.GetPKIEncryptionKey(ctx)
	if err == nil {
		if len(row.EncryptionKey) != 32 {
			return nil, fmt.Errorf("stored encryption key has invalid length %d (expected 32)", len(row.EncryptionKey))
		}
		return row.EncryptionKey, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("query encryption key: %w", err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	if _, err := q.CreatePKIEncryptionKey(ctx, key); err != nil {
		return nil, fmt.Errorf("store key: %w", err)
	}

	return key, nil
}
