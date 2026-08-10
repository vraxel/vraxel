package oidc

import "context"

// PendingStore manages pending authorization requests.
type PendingStore interface {
	Store(ctx context.Context, req *AuthorizeRequest) (string, error)
	Consume(ctx context.Context, id string) (*AuthorizeRequest, error)
}
