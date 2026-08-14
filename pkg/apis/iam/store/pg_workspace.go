package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
	"vraxel.io/vraxel/pkg/db/sqlnull"
)

type pgWorkspaceStore struct {
	db.Store
}

// NewPGWorkspaceStore creates a new PostgreSQL-backed WorkspaceStore.
func NewPGWorkspaceStore(d *db.DB) WorkspaceStore {
	return &pgWorkspaceStore{Store: db.Store{DB: d}}
}

func workspaceFromDetail(r generated.GetWorkspaceByIDRow) WorkspaceWithOwnerRow {
	return WorkspaceWithOwnerRow{
		WorkspaceRow: WorkspaceRow{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			OwnerID:     r.OwnerID,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		OwnerUsername:    r.OwnerUsername,
		CreatorName:      r.CreatorName,
		NamespaceCount:   r.NamespaceCount,
		MemberCount:      r.MemberCount,
		RoleBindingCount: r.RoleBindingCount,
	}
}

func workspaceFromUpdate(r generated.UpdateWorkspaceRow) WorkspaceWithOwnerRow {
	return WorkspaceWithOwnerRow{
		WorkspaceRow: WorkspaceRow{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			OwnerID:     r.OwnerID,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		OwnerUsername:    r.OwnerUsername,
		NamespaceCount:   r.NamespaceCount,
		MemberCount:      r.MemberCount,
		RoleBindingCount: r.RoleBindingCount,
	}
}

func workspaceFromPatch(r generated.PatchWorkspaceRow) WorkspaceWithOwnerRow {
	return WorkspaceWithOwnerRow{
		WorkspaceRow: WorkspaceRow{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			OwnerID:     r.OwnerID,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		OwnerUsername:    r.OwnerUsername,
		NamespaceCount:   r.NamespaceCount,
		MemberCount:      r.MemberCount,
		RoleBindingCount: r.RoleBindingCount,
	}
}

func (s *pgWorkspaceStore) Create(ctx context.Context, in WorkspaceCreateInput) (*WorkspaceWithOwnerRow, error) {
	var createdBy *int64
	if uid, ok := oidc.UserIDFromContext(ctx); ok {
		createdBy = &uid
	}
	return db.WithTxReturning(ctx, s.DB, func(ctx context.Context, qtx *generated.Queries) (*WorkspaceWithOwnerRow, error) {
		row, err := qtx.CreateWorkspace(ctx, generated.CreateWorkspaceParams{
			Name:        in.Name,
			DisplayName: in.DisplayName,
			Description: in.Description,
			OwnerID:     in.OwnerID,
			Status:      in.Status,
			CreatedBy:   createdBy,
		})
		if err != nil {
			return nil, fmt.Errorf("create workspace: %w", pgerrors.CheckPG(err))
		}

		wsAdminRoleID, err := workspaceCreateBuiltinWorkspaceRoles(ctx, qtx, row.ID)
		if err != nil {
			return nil, err
		}

		defaultNS, err := qtx.CreateNamespace(ctx, generated.CreateNamespaceParams{
			Name:        row.Name + "-default",
			DisplayName: "Default",
			Description: "Default namespace for workspace " + in.Name,
			WorkspaceID: row.ID,
			OwnerID:     in.OwnerID,
			Visibility:  "private",
			MaxMembers:  0,
			Status:      "active",
			CreatedBy:   createdBy,
		})
		if err != nil {
			return nil, fmt.Errorf("create default namespace: %w", err)
		}

		nsAdminRoleID, err := workspaceCreateBuiltinNamespaceRoles(ctx, qtx, defaultNS.ID)
		if err != nil {
			return nil, err
		}

		if _, err := qtx.CreateRoleBindingIfNotExists(ctx, generated.CreateRoleBindingIfNotExistsParams{
			UserID:      in.OwnerID,
			RoleID:      wsAdminRoleID,
			Scope:       ScopeWorkspace,
			WorkspaceID: &row.ID,
			IsOwner:     true,
		}); err != nil {
			return nil, fmt.Errorf("create workspace owner role binding: %w", err)
		}

		if _, err := qtx.CreateRoleBindingIfNotExists(ctx, generated.CreateRoleBindingIfNotExistsParams{
			UserID:      in.OwnerID,
			RoleID:      nsAdminRoleID,
			Scope:       ScopeNamespace,
			WorkspaceID: &row.ID,
			NamespaceID: &defaultNS.ID,
			IsOwner:     true,
		}); err != nil {
			return nil, fmt.Errorf("create default namespace owner role binding: %w", err)
		}

		detail, err := qtx.GetWorkspaceByID(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("get workspace after create: %w", err)
		}
		out := workspaceFromDetail(detail)
		return &out, nil
	})
}

// workspaceCreateBuiltinWorkspaceRoles creates the built-in workspace roles
// (with their permission rules) for a freshly-created workspace, returning the
// workspace-admin role ID for the owner role binding.
func workspaceCreateBuiltinWorkspaceRoles(ctx context.Context, qtx *generated.Queries, workspaceID int64) (int64, error) {
	var wsAdminRoleID int64
	for _, roleDef := range WorkspaceBuiltinRoles() {
		createdRole, err := qtx.CreateRole(ctx, generated.CreateRoleParams{
			Name:        roleDef.Name,
			DisplayName: roleDef.DisplayName,
			Description: roleDef.Description,
			Scope:       roleDef.Scope,
			Builtin:     true,
			WorkspaceID: &workspaceID,
		})
		if err != nil {
			return 0, fmt.Errorf("create workspace role %s: %w", roleDef.Name, err)
		}
		for _, pattern := range roleDef.Rules {
			if err := qtx.AddRolePermissionRule(ctx, generated.AddRolePermissionRuleParams{
				RoleID:  createdRole.ID,
				Pattern: pattern,
			}); err != nil {
				return 0, fmt.Errorf("add rule %s for role %s: %w", pattern, roleDef.Name, err)
			}
		}
		if roleDef.Name == RoleWorkspaceAdmin {
			wsAdminRoleID = createdRole.ID
		}
	}
	return wsAdminRoleID, nil
}

// workspaceCreateBuiltinNamespaceRoles creates the built-in namespace roles
// (with their permission rules) for the workspace's default namespace,
// returning the namespace-admin role ID for the owner role binding.
func workspaceCreateBuiltinNamespaceRoles(ctx context.Context, qtx *generated.Queries, namespaceID int64) (int64, error) {
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

func (s *pgWorkspaceStore) GetByID(ctx context.Context, id int64) (*WorkspaceWithOwnerRow, error) {
	row, err := s.Q().GetWorkspaceByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("workspace %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get workspace by id: %w", err)
	}
	out := workspaceFromDetail(row)
	return &out, nil
}

func (s *pgWorkspaceStore) Update(ctx context.Context, in WorkspaceUpdateInput) (*WorkspaceWithOwnerRow, error) {
	row, err := s.Q().UpdateWorkspace(ctx, generated.UpdateWorkspaceParams{
		ID:          in.ID,
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		OwnerID:     in.OwnerID,
		Status:      in.Status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("workspace %d: %w", in.ID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("update workspace: %w", pgerrors.CheckPG(err))
	}
	out := workspaceFromUpdate(row)
	return &out, nil
}

func (s *pgWorkspaceStore) Patch(ctx context.Context, in WorkspaceUpdateInput) (*WorkspaceWithOwnerRow, error) {
	row, err := s.Q().PatchWorkspace(ctx, generated.PatchWorkspaceParams{
		ID:          in.ID,
		Name:        sqlnull.ToNullString(in.Name),
		DisplayName: sqlnull.ToNullString(in.DisplayName),
		Description: sqlnull.ToNullString(in.Description),
		OwnerID:     sqlnull.ToNullInt64(in.OwnerID),
		Status:      sqlnull.ToNullString(in.Status),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("workspace %d: %w", in.ID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("patch workspace: %w", err)
	}
	out := workspaceFromPatch(row)
	return &out, nil
}

func (s *pgWorkspaceStore) Delete(ctx context.Context, id int64) error {
	// Cascade-delete child namespaces first (namespaces FK has no ON DELETE CASCADE).
	if err := s.Q().DeleteNamespacesByWorkspaceID(ctx, id); err != nil {
		return fmt.Errorf("cascade delete namespaces: %w", err)
	}

	rowsAffected, err := s.Q().DeleteWorkspace(ctx, id)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", pgerrors.CheckPG(err))
	}
	if rowsAffected == 0 {
		return fmt.Errorf("workspace %d: %w", id, pgerrors.ErrNotFound)
	}
	return nil
}

func (s *pgWorkspaceStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if err := s.Q().DeleteNamespacesByWorkspaceIDs(ctx, ids); err != nil {
		return 0, fmt.Errorf("cascade delete namespaces: %w", err)
	}
	deletedIDs, err := s.Q().DeleteWorkspacesByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("delete workspaces by ids: %w", pgerrors.CheckPG(err))
	}
	return int64(len(deletedIDs)), nil
}

type workspaceFilters struct {
	AccessibleIds []int64 `filter:"accessible_ids"`
	Status        *string `filter:"status"`
	Name          *string `filter:"name"`
	OwnerID       *int64  `filter:"owner_id"`
	Search        *string `filter:"search"`
}

func (s *pgWorkspaceStore) List(ctx context.Context, q list.Query) (*list.Result[WorkspaceWithOwnerRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	f := list.Parse[workspaceFilters](q.Filters)

	count, err := s.Q().CountWorkspaces(ctx, generated.CountWorkspacesParams{
		AccessibleIds: f.AccessibleIds,
		Status:        f.Status,
		Name:          f.Name,
		OwnerID:       f.OwnerID,
		Search:        f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count workspaces: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListWorkspaces(ctx, generated.ListWorkspacesParams{
		AccessibleIds: f.AccessibleIds,
		Status:        f.Status,
		Name:          f.Name,
		OwnerID:       f.OwnerID,
		Search:        f.Search,
		SortField:     q.SortBy,
		SortOrder:     sortOrder,
		PageOffset:    offset,
		PageSize:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	items := make([]WorkspaceWithOwnerRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, WorkspaceWithOwnerRow{
			WorkspaceRow: WorkspaceRow{
				ID:          r.ID,
				Name:        r.Name,
				DisplayName: r.DisplayName,
				Description: r.Description,
				OwnerID:     r.OwnerID,
				Status:      r.Status,
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.UpdatedAt,
			},
			OwnerUsername:  r.OwnerUsername,
			CreatorName:    r.CreatorName,
			NamespaceCount: r.NamespaceCount,
			MemberCount:    r.MemberCount,
		})
	}

	return &list.Result[WorkspaceWithOwnerRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgWorkspaceStore) CountNamespaces(ctx context.Context, workspaceID int64) (int64, error) {
	return s.Q().CountNamespacesByWorkspaceID(ctx, workspaceID)
}

func (s *pgWorkspaceStore) ListBlockingResources(ctx context.Context, workspaceID int64) ([]BlockingResourceRow, error) {
	rows, err := s.Q().CountWorkspaceBlockingResources(ctx, &workspaceID)
	if err != nil {
		return nil, fmt.Errorf("count workspace blocking resources: %w", err)
	}
	out := make([]BlockingResourceRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, BlockingResourceRow{Kind: r.Kind, Count: r.Cnt})
	}
	return out, nil
}
