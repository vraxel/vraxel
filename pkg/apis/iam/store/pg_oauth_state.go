package store

import (
	"context"
	"errors"
	"time"

	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
)

// OAuthStateRow is a consumed social-login state entry.
type OAuthStateRow struct {
	State     string
	Provider  string
	RequestID string
}

type pgOAuthStateStore struct {
	db.Store
}

// NewPGOAuthStateStore creates the short-lived CSRF/state store for the
// outbound social-login OAuth2 flow.
func NewPGOAuthStateStore(d *db.DB) OAuthStateStore {
	return &pgOAuthStateStore{Store: db.Store{DB: d}}
}

func (s *pgOAuthStateStore) Create(ctx context.Context, state, provider, requestID string, ttl time.Duration) error {
	return s.Q().CreateOAuthState(ctx, generated.CreateOAuthStateParams{
		State:     state,
		Provider:  provider,
		RequestID: requestID,
		ExpiresAt: time.Now().Add(ttl),
	})
}

func (s *pgOAuthStateStore) Consume(ctx context.Context, state string) (*OAuthStateRow, error) {
	row, err := s.Q().ConsumeOAuthState(ctx, state)
	if err != nil {
		return nil, errors.New("oauth state not found or expired")
	}
	return &OAuthStateRow{State: row.State, Provider: row.Provider, RequestID: row.RequestID}, nil
}
