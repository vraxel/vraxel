package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgPermissionStore struct {
	db.Store
}

// NewPGPermissionStore creates a new PostgreSQL-backed PermissionStore.
func NewPGPermissionStore(d *db.DB) PermissionStore {
	return &pgPermissionStore{Store: db.Store{DB: d}}
}

func permissionFromBase(r generated.Permission) PermissionRow {
	return PermissionRow{
		ID:          r.ID,
		Code:        r.Code,
		Method:      r.Method,
		Path:        r.Path,
		Scope:       r.Scope,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
	}
}

func (s *pgPermissionStore) Upsert(ctx context.Context, in PermissionUpsertInput) (*PermissionRow, error) {
	row, err := s.Q().UpsertPermission(ctx, generated.UpsertPermissionParams{
		Code:        in.Code,
		Method:      in.Method,
		Path:        in.Path,
		Scope:       in.Scope,
		Description: in.Description,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert permission: %w", err)
	}
	out := permissionFromBase(row)
	return &out, nil
}

func (s *pgPermissionStore) DeleteByModuleNotInCodeScopes(ctx context.Context, modulePrefix string, keepCodeScopes []string) error {
	if err := s.Q().DeletePermissionsByModulePrefix(ctx, generated.DeletePermissionsByModulePrefixParams{
		ModulePrefix:   modulePrefix,
		KeepCodeScopes: keepCodeScopes,
	}); err != nil {
		return fmt.Errorf("delete permissions by module prefix: %w", err)
	}
	return nil
}

func (s *pgPermissionStore) GetByCode(ctx context.Context, code, scope string) (*PermissionRow, error) {
	row, err := s.Q().GetPermissionByCode(ctx, generated.GetPermissionByCodeParams{
		Code:  code,
		Scope: scope,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("permission %s: %w", code, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get permission by code: %w", err)
	}
	out := permissionFromBase(row)
	return &out, nil
}

func (s *pgPermissionStore) List(ctx context.Context, q list.Query) (*list.Result[PermissionRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)

	countParams := generated.CountPermissionsParams{
		ModulePrefix: list.FilterStr(q.Filters, "module_prefix"),
		Search:       list.FilterStr(q.Filters, "search"),
		Scope:        list.FilterStr(q.Filters, "scope"),
	}

	count, err := s.Q().CountPermissions(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("count permissions: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "asc"
	}

	rows, err := s.Q().ListPermissions(ctx, generated.ListPermissionsParams{
		ModulePrefix: countParams.ModulePrefix,
		Search:       countParams.Search,
		Scope:        countParams.Scope,
		SortField:    q.SortBy,
		SortOrder:    sortOrder,
		PageOffset:   offset,
		PageSize:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}

	items := make([]PermissionRow, len(rows))
	for i, r := range rows {
		items[i] = permissionFromBase(r)
	}

	return &list.Result[PermissionRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgPermissionStore) ListAllCodes(ctx context.Context) ([]string, error) {
	codes, err := s.Q().ListAllPermissionCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all permission codes: %w", err)
	}
	return codes, nil
}

func (s *pgPermissionStore) ListCodeScopes(ctx context.Context) ([]PermissionCodeScope, error) {
	rows, err := s.Q().ListAllPermissionCodesWithScope(ctx)
	if err != nil {
		return nil, fmt.Errorf("list permission code scopes: %w", err)
	}
	result := make([]PermissionCodeScope, len(rows))
	for i, r := range rows {
		result[i] = PermissionCodeScope{Code: r.Code, Scope: r.Scope}
	}
	return result, nil
}

func (s *pgPermissionStore) SyncModule(ctx context.Context, modulePrefix string, perms []PermissionUpsertInput) error {
	return s.DB.WithTx(ctx, func(ctx context.Context, qtx *generated.Queries) error {
		codeScopes := make([]string, 0, len(perms))
		for _, p := range perms {
			if _, err := qtx.UpsertPermission(ctx, generated.UpsertPermissionParams{
				Code:        p.Code,
				Method:      p.Method,
				Path:        p.Path,
				Scope:       p.Scope,
				Description: p.Description,
			}); err != nil {
				return fmt.Errorf("upsert permission %s (scope=%s): %w", p.Code, p.Scope, err)
			}
			codeScopes = append(codeScopes, p.Code+":"+p.Scope)
		}

		if err := qtx.DeletePermissionsByModulePrefix(ctx, generated.DeletePermissionsByModulePrefixParams{
			ModulePrefix:   modulePrefix,
			KeepCodeScopes: codeScopes,
		}); err != nil {
			return fmt.Errorf("cleanup stale permissions: %w", err)
		}
		return nil
	})
}
