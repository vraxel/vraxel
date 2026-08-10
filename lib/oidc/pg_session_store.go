package oidc

import (
	"context"
	"errors"

	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/pkg/db/generated"
)

type pgSessionStore struct {
	queries *generated.Queries
}

// NewPGSessionStore creates a PostgreSQL-backed session store.
func NewPGSessionStore(queries *generated.Queries) SessionStore {
	return &pgSessionStore{queries: queries}
}

func (s *pgSessionStore) Create(ctx context.Context, session *Session) {
	if err := s.queries.CreateOIDCSession(ctx, generated.CreateOIDCSessionParams{
		SessionID: session.SessionID,
		UserID:    session.UserID,
		AuthTime:  session.AuthTime,
		ExpiresAt: session.ExpiresAt,
	}); err != nil {
		logger.Errorf("oidc: failed to persist session %s: %v", session.SessionID, err)
	}
}

func (s *pgSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	row, err := s.queries.GetOIDCSession(ctx, sessionID)
	if err != nil {
		return nil, errors.New("session not found")
	}
	return &Session{
		SessionID: row.SessionID,
		UserID:    row.UserID,
		AuthTime:  row.AuthTime,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

func (s *pgSessionStore) Delete(ctx context.Context, sessionID string) {
	_, _ = s.queries.DeleteOIDCSession(ctx, sessionID)
}
