package oidc

import (
	"context"
	"strings"

	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/pkg/db/generated"
)

type pgAuthCodeStore struct {
	queries *generated.Queries
}

// NewPGAuthCodeStore creates a PostgreSQL-backed auth code store.
func NewPGAuthCodeStore(queries *generated.Queries) AuthCodeStore {
	return &pgAuthCodeStore{queries: queries}
}

func (s *pgAuthCodeStore) Store(ctx context.Context, code *AuthorizationCode) {
	if err := s.queries.CreateOIDCAuthCode(ctx, generated.CreateOIDCAuthCodeParams{
		Code:                code.Code,
		ClientID:            code.ClientID,
		UserID:              code.UserID,
		RedirectUri:         code.RedirectURI,
		Scopes:              strings.Join(code.Scopes, " "),
		Nonce:               code.Nonce,
		CodeChallenge:       code.CodeChallenge,
		CodeChallengeMethod: code.CodeChallengeMethod,
		AuthTime:            code.AuthTime,
		ExpiresAt:           code.ExpiresAt,
	}); err != nil {
		logger.Errorf("oidc: failed to persist auth code %s: %v", code.Code, err)
	}
}

func (s *pgAuthCodeStore) Consume(ctx context.Context, codeStr string) (*AuthorizationCode, error) {
	row, err := s.queries.ConsumeOIDCAuthCode(ctx, codeStr)
	if err != nil {
		return nil, ErrCodeNotFound
	}
	return &AuthorizationCode{
		Code:                row.Code,
		ClientID:            row.ClientID,
		UserID:              row.UserID,
		RedirectURI:         row.RedirectUri,
		Scopes:              strings.Split(row.Scopes, " "),
		Nonce:               row.Nonce,
		CodeChallenge:       row.CodeChallenge,
		CodeChallengeMethod: row.CodeChallengeMethod,
		AuthTime:            row.AuthTime,
		Consumed:            true,
	}, nil
}
