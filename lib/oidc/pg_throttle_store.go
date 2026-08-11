package oidc

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"vraxel.io/vraxel/pkg/db/generated"
)

type pgLoginThrottleStore struct {
	queries *generated.Queries
}

// NewPGLoginThrottleStore creates the PostgreSQL-backed failure-counter
// store shared by all instances.
func NewPGLoginThrottleStore(queries *generated.Queries) LoginThrottleStore {
	return &pgLoginThrottleStore{queries: queries}
}

func (s *pgLoginThrottleStore) Get(ctx context.Context, key string) (int, time.Time, bool, error) {
	row, err := s.queries.GetLoginThrottle(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, false, nil
	}
	if err != nil {
		return 0, time.Time{}, false, err
	}
	return int(row.FailCount), row.WindowStart, true, nil
}

func (s *pgLoginThrottleStore) Bump(ctx context.Context, key string, windowSeconds int) error {
	_, err := s.queries.BumpLoginThrottle(ctx, generated.BumpLoginThrottleParams{
		Key:           key,
		WindowSeconds: int32(windowSeconds),
	})
	return err
}

func (s *pgLoginThrottleStore) Reset(ctx context.Context, key string) error {
	return s.queries.ResetLoginThrottle(ctx, key)
}

func (s *pgLoginThrottleStore) Sweep(ctx context.Context, windowSeconds int) error {
	return s.queries.SweepLoginThrottle(ctx, int32(windowSeconds))
}
