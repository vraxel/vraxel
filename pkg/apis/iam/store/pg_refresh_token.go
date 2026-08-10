package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgRefreshTokenStore struct {
	db.Store
}

func NewPGRefreshTokenStore(d *db.DB) RefreshTokenStore {
	return &pgRefreshTokenStore{Store: db.Store{DB: d}}
}

func refreshTokenToDomain(r *generated.RefreshToken) RefreshTokenRow {
	return RefreshTokenRow{
		ID:        r.ID,
		TokenHash: r.TokenHash,
		UserID:    r.UserID,
		ClientID:  r.ClientID,
		Scope:     r.Scope,
		ExpiresAt: r.ExpiresAt,
		Revoked:   r.Revoked,
		CreatedAt: r.CreatedAt,
	}
}

func (s *pgRefreshTokenStore) Create(ctx context.Context, input RefreshTokenCreateInput) (*RefreshTokenRow, error) {
	row, err := s.Q().CreateRefreshToken(ctx, generated.CreateRefreshTokenParams{
		TokenHash: input.TokenHash,
		UserID:    input.UserID,
		ClientID:  input.ClientID,
		Scope:     input.Scope,
		ExpiresAt: input.ExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", pgerrors.CheckPG(err))
	}
	out := refreshTokenToDomain(&row)
	return &out, nil
}

func (s *pgRefreshTokenStore) GetByHash(ctx context.Context, tokenHash string) (*RefreshTokenRow, error) {
	row, err := s.Q().GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("refresh_token %s: %w", tokenHash, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	out := refreshTokenToDomain(&row)
	return &out, nil
}

func (s *pgRefreshTokenStore) ConsumeByHash(ctx context.Context, tokenHash string) (*RefreshTokenRow, error) {
	row, err := s.Q().ConsumeRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("refresh_token %s: %w", tokenHash, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("consume refresh token: %w", err)
	}
	out := refreshTokenToDomain(&row)
	return &out, nil
}

func (s *pgRefreshTokenStore) Revoke(ctx context.Context, tokenHash string) error {
	if err := s.Q().RevokeRefreshToken(ctx, tokenHash); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (s *pgRefreshTokenStore) RevokeByUserID(ctx context.Context, userID int64) error {
	if err := s.Q().RevokeRefreshTokensByUserID(ctx, userID); err != nil {
		return fmt.Errorf("revoke refresh tokens by user: %w", err)
	}
	return nil
}

func (s *pgRefreshTokenStore) DeleteExpired(ctx context.Context) error {
	if err := s.Q().DeleteExpiredRefreshTokens(ctx); err != nil {
		return fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	return nil
}
