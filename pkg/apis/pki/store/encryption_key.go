package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
)

// encryptionKeyBytes is an AES-256 key.
const encryptionKeyBytes = 32

// LoadOrGenerateEncryptionKey loads the AES-256 PKI encryption key from
// the database, generating and storing one on first boot.
//
// The generated key is never returned directly. Several instances can
// boot against an empty database at once and each would generate its own;
// the singleton row lets exactly one insert win, and everyone -- winner
// included -- reads the row back and uses that. Returning the locally
// generated key instead would leave the losing instances signing with a
// key no other instance can verify, and every agent that registered
// against one of them would be locked out for good after the next
// restart.
//
// Kept in the store implementation layer so pgx + generated imports do
// not leak into pki/install.go or any business-layer file.
func LoadOrGenerateEncryptionKey(ctx context.Context, database *db.DB) ([]byte, error) {
	q := database.GetQueries()

	key, err := loadEncryptionKey(ctx, q)
	if err != nil {
		return nil, err
	}
	if key != nil {
		return key, nil
	}

	candidate := make([]byte, encryptionKeyBytes)
	if _, err := rand.Read(candidate); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if err := q.CreatePKIEncryptionKey(ctx, candidate); err != nil {
		return nil, fmt.Errorf("store key: %w", err)
	}

	key, err = loadEncryptionKey(ctx, q)
	if err != nil {
		return nil, err
	}
	if key == nil {
		// Only reachable if something deleted the row between the insert
		// and this read. Failing the boot is right: serving without the
		// stored key would mint agent tokens nothing can verify.
		return nil, errors.New("encryption key disappeared immediately after it was stored")
	}
	return key, nil
}

// loadEncryptionKey returns the stored key, or nil when the table is
// still empty.
func loadEncryptionKey(ctx context.Context, q *generated.Queries) ([]byte, error) {
	row, err := q.GetPKIEncryptionKey(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query encryption key: %w", err)
	}
	if len(row.EncryptionKey) != encryptionKeyBytes {
		return nil, fmt.Errorf("stored encryption key has invalid length %d (expected %d)",
			len(row.EncryptionKey), encryptionKeyBytes)
	}
	return row.EncryptionKey, nil
}
