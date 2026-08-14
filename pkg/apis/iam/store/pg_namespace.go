package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
	"vraxel.io/vraxel/pkg/db/sqlnull"
)

type pgNamespaceStore struct {
	db.Store
	// wsIDMemo caches namespace id -> workspace id for WorkspaceIDOf. The
	// mapping is immutable, so entries never expire or need invalidation.
	wsIDMemo sync.Map
}

func NewPGNamespaceStore(d *db.DB) NamespaceStore {
	return &pgNamespaceStore{Store: db.Store{DB: d}}
}

// WorkspaceIDOf returns the parent workspace id for a namespace via the
// light single-column query (not the join-heavy GetByID), for the authz
// hot path that scopes a flat /namespaces/{id} route. The mapping is
// immutable (NamespaceUpdateInput has no workspace field -- a namespace
// never changes workspace), so results are memoized permanently: the auth
// path pays the DB round-trip once per namespace id, not once per request.
func (s *pgNamespaceStore) WorkspaceIDOf(ctx context.Context, id int64) (int64, error) {
	if v, ok := s.wsIDMemo.Load(id); ok {
		return v.(int64), nil
	}
	wsID, err := s.Q().GetNamespaceWorkspaceID(ctx, id)
	if err != nil {
		return 0, err
	}
	s.wsIDMemo.Store(id, wsID)
	return wsID, nil
}

func namespaceFromDetail(r generated.GetNamespaceByIDRow) NamespaceWithOwnerRow {
	return NamespaceWithOwnerRow{
		NamespaceRow: NamespaceRow{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			WorkspaceID: r.WorkspaceID,
			OwnerID:     r.OwnerID,
			Visibility:  r.Visibility,
			MaxMembers:  r.MaxMembers,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		OwnerUsername:    r.OwnerUsername,
		WorkspaceName:    r.WorkspaceName,
		CreatorName:      r.CreatorName,
		MemberCount:      r.MemberCount,
		RoleBindingCount: r.RoleBindingCount,
	}
}

func namespaceFromUpdate(r generated.UpdateNamespaceRow) NamespaceWithOwnerRow {
	return NamespaceWithOwnerRow{
		NamespaceRow: NamespaceRow{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			WorkspaceID: r.WorkspaceID,
			OwnerID:     r.OwnerID,
			Visibility:  r.Visibility,
			MaxMembers:  r.MaxMembers,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		OwnerUsername:    r.OwnerUsername,
		WorkspaceName:    r.WorkspaceName,
		MemberCount:      r.MemberCount,
		RoleBindingCount: r.RoleBindingCount,
	}
}

func namespaceFromPatch(r generated.PatchNamespaceRow) NamespaceWithOwnerRow {
	return NamespaceWithOwnerRow{
		NamespaceRow: NamespaceRow{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			WorkspaceID: r.WorkspaceID,
			OwnerID:     r.OwnerID,
			Visibility:  r.Visibility,
			MaxMembers:  r.MaxMembers,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		OwnerUsername:    r.OwnerUsername,
		WorkspaceName:    r.WorkspaceName,
		MemberCount:      r.MemberCount,
		RoleBindingCount: r.RoleBindingCount,
	}
}

func (s *pgNamespaceStore) Create(ctx context.Context, input NamespaceCreateInput) (*NamespaceWithOwnerRow, error) {
	var createdBy *int64
	if uid, ok := oidc.UserIDFromContext(ctx); ok {
		createdBy = &uid
	}
	return db.WithTxReturning(ctx, s.DB, func(ctx context.Context, qtx *generated.Queries) (*NamespaceWithOwnerRow, error) {
		row, err := qtx.CreateNamespace(ctx, generated.CreateNamespaceParams{
			Name:        input.Name,
			DisplayName: input.DisplayName,
			Description: input.Description,
			WorkspaceID: input.WorkspaceID,
			OwnerID:     input.OwnerID,
			Visibility:  input.Visibility,
			MaxMembers:  input.MaxMembers,
			Status:      input.Status,
			CreatedBy:   createdBy,
		})
		if err != nil {
			return nil, fmt.Errorf("create namespace: %w", pgerrors.CheckPG(err))
		}

		nsAdminRoleID, err := namespaceCreateBuiltinRoles(ctx, qtx, row.ID)
		if err != nil {
			return nil, err
		}

		if _, err := qtx.CreateRoleBindingIfNotExists(ctx, generated.CreateRoleBindingIfNotExistsParams{
			UserID:      input.OwnerID,
			RoleID:      nsAdminRoleID,
			Scope:       ScopeNamespace,
			WorkspaceID: &input.WorkspaceID,
			NamespaceID: &row.ID,
			IsOwner:     true,
		}); err != nil {
			return nil, fmt.Errorf("create namespace owner role binding: %w", err)
		}

		nsRow, err := qtx.GetNamespaceByID(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("get namespace after create: %w", err)
		}
		out := namespaceFromDetail(nsRow)
		return &out, nil
	})
}

// namespaceCreateBuiltinRoles creates the built-in namespace roles (with their
// permission rules) for a freshly-created namespace, returning the
// namespace-admin role ID for the owner role binding.
func namespaceCreateBuiltinRoles(ctx context.Context, qtx *generated.Queries, namespaceID int64) (int64, error) {
	var nsAdminRoleID int64
	for _, roleDef := range NamespaceBuiltinRoles() {
		createdRole, err := qtx.CreateRole(ctx, generated.CreateRoleParams{
			Name:        roleDef.Name,
			DisplayName: roleDef.DisplayName,
			Description: roleDef.Description,
			Scope:       roleDef.Scope,
			Builtin:     true,
			NamespaceID: &namespaceID,
		})
		if err != nil {
			return 0, fmt.Errorf("create namespace role %s: %w", roleDef.Name, err)
		}
		for _, pattern := range roleDef.Rules {
			if err := qtx.AddRolePermissionRule(ctx, generated.AddRolePermissionRuleParams{
				RoleID:  createdRole.ID,
				Pattern: pattern,
			}); err != nil {
				return 0, fmt.Errorf("add rule %s for role %s: %w", pattern, roleDef.Name, err)
			}
		}
		if roleDef.Name == RoleNamespaceAdmin {
			nsAdminRoleID = createdRole.ID
		}
	}
	return nsAdminRoleID, nil
}

func (s *pgNamespaceStore) GetByID(ctx context.Context, id int64) (*NamespaceWithOwnerRow, error) {
	row, err := s.Q().GetNamespaceByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("namespace %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get namespace by id: %w", err)
	}
	out := namespaceFromDetail(row)
	return &out, nil
}

func (s *pgNamespaceStore) Update(ctx context.Context, input NamespaceUpdateInput) (*NamespaceWithOwnerRow, error) {
	row, err := s.Q().UpdateNamespace(ctx, generated.UpdateNamespaceParams{
		ID:          input.ID,
		Name:        input.Name,
		DisplayName: input.DisplayName,
		Description: input.Description,
		OwnerID:     input.OwnerID,
		Visibility:  input.Visibility,
		MaxMembers:  input.MaxMembers,
		Status:      input.Status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("namespace %d: %w", input.ID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("update namespace: %w", pgerrors.CheckPG(err))
	}
	out := namespaceFromUpdate(row)
	return &out, nil
}

func (s *pgNamespaceStore) Patch(ctx context.Context, input NamespaceUpdateInput) (*NamespaceWithOwnerRow, error) {
	row, err := s.Q().PatchNamespace(ctx, generated.PatchNamespaceParams{
		ID:          input.ID,
		Name:        sqlnull.ToNullString(input.Name),
		DisplayName: sqlnull.ToNullString(input.DisplayName),
		Description: sqlnull.ToNullString(input.Description),
		OwnerID:     sqlnull.ToNullInt64(input.OwnerID),
		Visibility:  sqlnull.ToNullString(input.Visibility),
		MaxMembers:  sqlnull.ToNullInt32(input.MaxMembers),
		Status:      sqlnull.ToNullString(input.Status),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("namespace %d: %w", input.ID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("patch namespace: %w", err)
	}
	out := namespaceFromPatch(row)
	return &out, nil
}

func (s *pgNamespaceStore) Delete(ctx context.Context, id int64) error {
	rowsAffected, err := s.Q().DeleteNamespace(ctx, id)
	if err != nil {
		return fmt.Errorf("delete namespace: %w", pgerrors.CheckPG(err))
	}
	if rowsAffected == 0 {
		return fmt.Errorf("namespace %d: %w", id, pgerrors.ErrNotFound)
	}
	return nil
}

func (s *pgNamespaceStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	deletedIDs, err := s.Q().DeleteNamespacesByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("delete namespaces by ids: %w", pgerrors.CheckPG(err))
	}
	return int64(len(deletedIDs)), nil
}

type namespaceFilters struct {
	AccessibleIds []int64 `filter:"accessible_ids"`
	Status        *string `filter:"status"`
	Name          *string `filter:"name"`
	Visibility    *string `filter:"visibility"`
	OwnerID       *int64  `filter:"owner_id"`
	WorkspaceID   *int64  `filter:"workspace_id"`
	Search        *string `filter:"search"`
}

func (s *pgNamespaceStore) List(ctx context.Context, q list.Query) (*list.Result[NamespaceWithOwnerRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	f := list.Parse[namespaceFilters](q.Filters)

	count, err := s.Q().CountNamespaces(ctx, generated.CountNamespacesParams{
		AccessibleIds: f.AccessibleIds,
		Status:        f.Status,
		Name:          f.Name,
		Visibility:    f.Visibility,
		OwnerID:       f.OwnerID,
		WorkspaceID:   f.WorkspaceID,
		Search:        f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count namespaces: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListNamespaces(ctx, generated.ListNamespacesParams{
		AccessibleIds: f.AccessibleIds,
		Status:        f.Status,
		Name:          f.Name,
		Visibility:    f.Visibility,
		OwnerID:       f.OwnerID,
		WorkspaceID:   f.WorkspaceID,
		Search:        f.Search,
		SortField:     q.SortBy,
		SortOrder:     sortOrder,
		PageOffset:    offset,
		PageSize:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	items := make([]NamespaceWithOwnerRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, NamespaceWithOwnerRow{
			NamespaceRow: NamespaceRow{
				ID:          r.ID,
				Name:        r.Name,
				DisplayName: r.DisplayName,
				Description: r.Description,
				WorkspaceID: r.WorkspaceID,
				OwnerID:     r.OwnerID,
				Visibility:  r.Visibility,
				MaxMembers:  r.MaxMembers,
				Status:      r.Status,
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.UpdatedAt,
			},
			OwnerUsername: r.OwnerUsername,
			WorkspaceName: r.WorkspaceName,
			CreatorName:   r.CreatorName,
			MemberCount:   r.MemberCount,
		})
	}

	return &list.Result[NamespaceWithOwnerRow]{Items: items, TotalCount: count}, nil
}

func (s *pgNamespaceStore) CountUsers(ctx context.Context, namespaceID int64) (int64, error) {
	nsID := &namespaceID
	return s.Q().CountUsersByNamespaceID(ctx, nsID)
}

func (s *pgNamespaceStore) ListBlockingResources(ctx context.Context, namespaceID int64) ([]BlockingResourceRow, error) {
	rows, err := s.Q().CountNamespaceBlockingResources(ctx, &namespaceID)
	if err != nil {
		return nil, fmt.Errorf("count namespace blocking resources: %w", err)
	}
	out := make([]BlockingResourceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, BlockingResourceRow{Kind: r.Kind, Count: r.Cnt})
	}
	return out, nil
}
