package oidc

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"vraxel.io/vraxel/pkg/db/generated"
)

// DBKeyStore implements KeyStore by persisting signing keys in PostgreSQL.
// On LoadOrGenerate, it first attempts to load an existing key for the
// given algorithm. If none exists, it generates a new key pair and stores it.
type DBKeyStore struct {
	pool    *pgxpool.Pool
	queries *generated.Queries
}

// NewDBKeyStore creates a new DBKeyStore backed by the given connection pool.
func NewDBKeyStore(pool *pgxpool.Pool, queries *generated.Queries) *DBKeyStore {
	return &DBKeyStore{pool: pool, queries: queries}
}

// LoadOrGenerate loads an existing key for the algorithm from the database,
// or generates and stores a new one if none exists.
func (s *DBKeyStore) LoadOrGenerate(algorithm string) (*KeySet, error) {
	ctx := context.Background()

	// Try to load existing key
	row, err := s.queries.GetOIDCKey(ctx, algorithm)
	if err == nil {
		return ParseKeySetPEM(row.PrivateKey, row.PublicKey, row.Algorithm)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("query oidc key: %w", err)
	}

	// Generate new key
	ks, err := GenerateKeySet(algorithm)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	privPEM, pubPEM, err := MarshalKeySetPEM(ks)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	_, err = s.queries.CreateOIDCKey(ctx, generated.CreateOIDCKeyParams{
		KeyID:      ks.KeyID,
		PrivateKey: privPEM,
		PublicKey:  pubPEM,
		Algorithm:  algorithm,
	})
	if err != nil {
		return nil, fmt.Errorf("store key: %w", err)
	}

	return ks, nil
}
