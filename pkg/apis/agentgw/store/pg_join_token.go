package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/shared/scope"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgJoinTokenStore struct {
	db.Store
}

// NewPGJoinTokenStore creates a PostgreSQL-backed JoinTokenStore.
func NewPGJoinTokenStore(d *db.DB) JoinTokenStore { return &pgJoinTokenStore{Store: db.Store{DB: d}} }

func (s *pgJoinTokenStore) Create(ctx context.Context, in JoinTokenCreateInput) (*JoinTokenRow, error) {
	row, err := s.Q().CreateHostAgentJoinToken(ctx, generated.CreateHostAgentJoinTokenParams{
		Name:        in.Name,
		TokenHash:   in.TokenHash,
		Scope:       in.Scope,
		WorkspaceID: in.WorkspaceID,
		NamespaceID: in.NamespaceID,
		MaxUses:     in.MaxUses,
		ExpiresAt:   in.ExpiresAt,
		CreatedBy:   in.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create join token: %w", pgerrors.CheckPG(err))
	}
	return joinTokenToDomain(&row, ""), nil
}

func (s *pgJoinTokenStore) GetByID(ctx context.Context, id int64, sf scope.Filter) (*JoinTokenRow, error) {
	row, err := s.Q().GetHostAgentJoinTokenByID(ctx, generated.GetHostAgentJoinTokenByIDParams{
		ID:                id,
		WorkspaceIDFilter: sf.WorkspaceID,
		NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("join token %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get join token: %w", err)
	}
	return &JoinTokenRow{
		ID:          row.ID,
		Name:        row.Name,
		TokenHash:   row.TokenHash,
		Scope:       row.Scope,
		WorkspaceID: row.WorkspaceID,
		NamespaceID: row.NamespaceID,
		MaxUses:     row.MaxUses,
		UsedCount:   row.UsedCount,
		ExpiresAt:   row.ExpiresAt,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		CreatorName: row.CreatorName,
	}, nil
}

func (s *pgJoinTokenStore) List(ctx context.Context, q list.Query) (*list.Result[JoinTokenRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	count, err := s.Q().CountHostAgentJoinTokens(ctx, generated.CountHostAgentJoinTokensParams{
		Scope:       list.FilterStr(q.Filters, "scope"),
		WorkspaceID: list.FilterInt64(q.Filters, "workspace_id"),
		NamespaceID: list.FilterInt64(q.Filters, "namespace_id"),
		Search:      list.FilterStr(q.Filters, "search"),
	})
	if err != nil {
		return nil, fmt.Errorf("count join tokens: %w", err)
	}

	rows, err := s.Q().ListHostAgentJoinTokens(ctx, generated.ListHostAgentJoinTokensParams{
		Scope:       list.FilterStr(q.Filters, "scope"),
		WorkspaceID: list.FilterInt64(q.Filters, "workspace_id"),
		NamespaceID: list.FilterInt64(q.Filters, "namespace_id"),
		Search:      list.FilterStr(q.Filters, "search"),
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list join tokens: %w", err)
	}

	items := make([]JoinTokenRow, len(rows))
	for i := range rows {
		items[i] = JoinTokenRow{
			ID:          rows[i].ID,
			Name:        rows[i].Name,
			TokenHash:   rows[i].TokenHash,
			Scope:       rows[i].Scope,
			WorkspaceID: rows[i].WorkspaceID,
			NamespaceID: rows[i].NamespaceID,
			MaxUses:     rows[i].MaxUses,
			UsedCount:   rows[i].UsedCount,
			ExpiresAt:   rows[i].ExpiresAt,
			CreatedBy:   rows[i].CreatedBy,
			CreatedAt:   rows[i].CreatedAt,
			CreatorName: rows[i].CreatorName,
		}
	}
	return &list.Result[JoinTokenRow]{Items: items, TotalCount: count}, nil
}

func (s *pgJoinTokenStore) Delete(ctx context.Context, id int64, sf scope.Filter) error {
	n, err := s.Q().DeleteHostAgentJoinToken(ctx, generated.DeleteHostAgentJoinTokenParams{
		ID:                id,
		WorkspaceIDFilter: sf.WorkspaceID,
		NamespaceIDFilter: sf.NamespaceID,
	})
	if err != nil {
		return fmt.Errorf("delete join token: %w", pgerrors.CheckPG(err))
	}
	if n == 0 {
		return fmt.Errorf("join token %d: %w", id, pgerrors.ErrNotFound)
	}
	return nil
}

func (s *pgJoinTokenStore) Consume(ctx context.Context, tokenHash []byte) (*JoinTokenRow, error) {
	row, err := s.Q().ConsumeHostAgentJoinToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Unknown hash, expired, or uses exhausted are deliberately
			// indistinguishable to the caller: /register must not become
			// a token-probing oracle.
			return nil, fmt.Errorf("join token: %w", pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("consume join token: %w", pgerrors.CheckPG(err))
	}
	return joinTokenToDomain(&row, ""), nil
}

func joinTokenToDomain(r *generated.HostAgentJoinToken, creatorName string) *JoinTokenRow {
	return &JoinTokenRow{
		ID:          r.ID,
		Name:        r.Name,
		TokenHash:   r.TokenHash,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		MaxUses:     r.MaxUses,
		UsedCount:   r.UsedCount,
		ExpiresAt:   r.ExpiresAt,
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt,
		CreatorName: creatorName,
	}
}
