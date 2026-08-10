package oidc

import (
	"context"
	"errors"
	"time"

	"vraxel.io/vraxel/pkg/db/generated"
)

type pgPendingStore struct {
	queries *generated.Queries
}

// NewPGPendingStore creates a PostgreSQL-backed pending authorization request store.
func NewPGPendingStore(queries *generated.Queries) PendingStore {
	return &pgPendingStore{queries: queries}
}

func (s *pgPendingStore) Store(ctx context.Context, req *AuthorizeRequest) (string, error) {
	id, err := GenerateCode()
	if err != nil {
		return "", err
	}
	err = s.queries.CreateOIDCPendingRequest(ctx, generated.CreateOIDCPendingRequestParams{
		ID:                  id,
		ResponseType:        req.ResponseType,
		ClientID:            req.ClientID,
		RedirectUri:         req.RedirectURI,
		Scope:               req.Scope,
		State:               req.State,
		Nonce:               req.Nonce,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		ExpiresAt:           time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *pgPendingStore) Consume(ctx context.Context, id string) (*AuthorizeRequest, error) {
	row, err := s.queries.ConsumeOIDCPendingRequest(ctx, id)
	if err != nil {
		return nil, errors.New("pending request not found")
	}
	return &AuthorizeRequest{
		ResponseType:        row.ResponseType,
		ClientID:            row.ClientID,
		RedirectURI:         row.RedirectUri,
		Scope:               row.Scope,
		State:               row.State,
		Nonce:               row.Nonce,
		CodeChallenge:       row.CodeChallenge,
		CodeChallengeMethod: row.CodeChallengeMethod,
	}, nil
}
