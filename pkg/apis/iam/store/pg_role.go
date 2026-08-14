package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/logger"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

type pgRoleStore struct {
	db.Store
}

// NewPGRoleStore creates a new PostgreSQL-backed RoleStore.
func NewPGRoleStore(d *db.DB) RoleStore {
	return &pgRoleStore{Store: db.Store{DB: d}}
}

func roleFromCreate(r generated.CreateRoleRow) RoleRow {
	return RoleRow{
		ID:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func roleFromGetByID(r generated.GetRoleByIDRow) RoleRow {
	return RoleRow{
		ID:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func roleFromGetByName(r generated.GetRoleByNameRow) RoleRow {
	return RoleRow{
		ID:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func roleFromGetByNameAndWorkspace(r generated.GetRoleByNameAndWorkspaceRow) RoleRow {
	return RoleRow{
		ID:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func roleFromGetByNameAndNamespace(r generated.GetRoleByNameAndNamespaceRow) RoleRow {
	return RoleRow{
		ID:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func roleFromUpdate(r generated.UpdateRoleRow) RoleRow {
	return RoleRow{
		ID:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func roleFromUpsert(r generated.UpsertRoleRow) RoleRow {
	return RoleRow{
		ID:          r.ID,
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Description: r.Description,
		Scope:       r.Scope,
		WorkspaceID: r.WorkspaceID,
		NamespaceID: r.NamespaceID,
		Builtin:     r.Builtin,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func roleFromList(r generated.ListRolesRow) RoleListItem {
	return RoleListItem{
		RoleRow: RoleRow{
			ID:          r.ID,
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			Scope:       r.Scope,
			WorkspaceID: r.WorkspaceID,
			NamespaceID: r.NamespaceID,
			Builtin:     r.Builtin,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		},
		RuleCount: r.RuleCount,
	}
}

func (s *pgRoleStore) Create(ctx context.Context, in RoleCreateInput) (*RoleRow, error) {
	row, err := s.Q().CreateRole(ctx, generated.CreateRoleParams{
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		Scope:       in.Scope,
		Builtin:     in.Builtin,
		WorkspaceID: in.WorkspaceID,
		NamespaceID: in.NamespaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("create role: %w", pgerrors.CheckPG(err))
	}
	out := roleFromCreate(row)
	return &out, nil
}

func (s *pgRoleStore) GetByID(ctx context.Context, id int64) (*RoleWithRulesRow, error) {
	row, err := s.Q().GetRoleByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("role %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get role by id: %w", err)
	}

	rules, err := s.Q().GetRulesByRoleID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get rules for role %d: %w", id, err)
	}

	return &RoleWithRulesRow{
		RoleRow: roleFromGetByID(row),
		Rules:   rules,
	}, nil
}

func (s *pgRoleStore) GetByName(ctx context.Context, name string) (*RoleRow, error) {
	row, err := s.Q().GetRoleByName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("role %s: %w", name, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get role by name: %w", err)
	}
	out := roleFromGetByName(row)
	return &out, nil
}

func (s *pgRoleStore) GetByNameAndWorkspace(ctx context.Context, name string, workspaceID int64) (*RoleRow, error) {
	row, err := s.Q().GetRoleByNameAndWorkspace(ctx, generated.GetRoleByNameAndWorkspaceParams{
		Name:        name,
		WorkspaceID: &workspaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("role %s: %w", name, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get role by name and workspace: %w", err)
	}
	out := roleFromGetByNameAndWorkspace(row)
	return &out, nil
}

func (s *pgRoleStore) GetByNameAndNamespace(ctx context.Context, name string, namespaceID int64) (*RoleRow, error) {
	row, err := s.Q().GetRoleByNameAndNamespace(ctx, generated.GetRoleByNameAndNamespaceParams{
		Name:        name,
		NamespaceID: &namespaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("role %s: %w", name, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get role by name and namespace: %w", err)
	}
	out := roleFromGetByNameAndNamespace(row)
	return &out, nil
}

func (s *pgRoleStore) Update(ctx context.Context, in RoleUpdateInput) (*RoleRow, error) {
	row, err := s.Q().UpdateRole(ctx, generated.UpdateRoleParams{
		ID:          in.ID,
		DisplayName: in.DisplayName,
		Description: in.Description,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("role %d: %w", in.ID, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("update role: %w", pgerrors.CheckPG(err))
	}
	out := roleFromUpdate(row)
	return &out, nil
}

func (s *pgRoleStore) Upsert(ctx context.Context, in RoleCreateInput) (*RoleRow, error) {
	row, err := s.Q().UpsertRole(ctx, generated.UpsertRoleParams{
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		Scope:       in.Scope,
		Builtin:     in.Builtin,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert role: %w", err)
	}
	out := roleFromUpsert(row)
	return &out, nil
}

func (s *pgRoleStore) Delete(ctx context.Context, id int64) error {
	rowsAffected, err := s.Q().DeleteRole(ctx, id)
	if err != nil {
		return fmt.Errorf("delete role: %w", pgerrors.CheckPG(err))
	}
	if rowsAffected == 0 {
		return fmt.Errorf("role %d: %w", id, pgerrors.ErrNotFound)
	}
	return nil
}

type roleFilters struct {
	Scope       *string  `filter:"scope"`
	Builtin     *bool    `filter:"builtin"`
	WorkspaceID *int64   `filter:"workspace_id"`
	NamespaceID *int64   `filter:"namespace_id"`
	Search      *string  `filter:"search"`
	ExtraNames  []string `filter:"extra_names"`
}

func (s *pgRoleStore) List(ctx context.Context, q list.Query) (*list.Result[RoleListItem], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	f := list.Parse[roleFilters](q.Filters)

	count, err := s.Q().CountRoles(ctx, generated.CountRolesParams{
		Scope:       f.Scope,
		Builtin:     f.Builtin,
		WorkspaceID: f.WorkspaceID,
		NamespaceID: f.NamespaceID,
		Search:      f.Search,
		ExtraNames:  f.ExtraNames,
	})
	if err != nil {
		return nil, fmt.Errorf("count roles: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListRoles(ctx, generated.ListRolesParams{
		Scope:       f.Scope,
		Builtin:     f.Builtin,
		WorkspaceID: f.WorkspaceID,
		NamespaceID: f.NamespaceID,
		Search:      f.Search,
		ExtraNames:  f.ExtraNames,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	items := make([]RoleListItem, len(rows))
	for i, r := range rows {
		items[i] = roleFromList(r)
	}

	return &list.Result[RoleListItem]{
		Items:      items,
		TotalCount: count,
	}, nil
}

// createBuiltinRolesInTx creates built-in roles with permission rules using the provided transaction-scoped queries.
func createBuiltinRolesInTx(ctx context.Context, qtx *generated.Queries, defs []BuiltinRoleDef, workspaceID *int64, namespaceID *int64) error {
	for _, def := range defs {
		var exists bool
		if workspaceID != nil {
			_, err := qtx.GetRoleByNameAndWorkspace(ctx, generated.GetRoleByNameAndWorkspaceParams{
				Name:        def.Name,
				WorkspaceID: workspaceID,
			})
			exists = err == nil
		} else if namespaceID != nil {
			_, err := qtx.GetRoleByNameAndNamespace(ctx, generated.GetRoleByNameAndNamespaceParams{
				Name:        def.Name,
				NamespaceID: namespaceID,
			})
			exists = err == nil
		}
		if exists {
			continue
		}

		row, err := qtx.CreateRole(ctx, generated.CreateRoleParams{
			Name:        def.Name,
			DisplayName: def.DisplayName,
			Description: def.Description,
			Scope:       def.Scope,
			Builtin:     true,
			WorkspaceID: workspaceID,
			NamespaceID: namespaceID,
		})
		if err != nil {
			return fmt.Errorf("create builtin role %s: %w", def.Name, err)
		}
		for _, pattern := range def.Rules {
			if err := qtx.AddRolePermissionRule(ctx, generated.AddRolePermissionRuleParams{
				RoleID:  row.ID,
				Pattern: pattern,
			}); err != nil {
				return fmt.Errorf("add rule %q for role %s: %w", pattern, def.Name, err)
			}
		}
	}
	return nil
}

func (s *pgRoleStore) SeedRBAC(ctx context.Context, roles []BuiltinRoleDef, adminUsername string) error {
	return s.DB.WithTx(ctx, func(ctx context.Context, qtx *generated.Queries) error {
		platformAdminRoleID, err := seedBuiltinRoles(ctx, qtx, roles)
		if err != nil {
			return err
		}

		if adminUsername != "" && platformAdminRoleID != 0 {
			adminUser, err := qtx.GetUserByUsername(ctx, adminUsername)
			if err == nil {
				_, _ = qtx.CreateRoleBindingIfNotExists(ctx, generated.CreateRoleBindingIfNotExistsParams{
					UserID: adminUser.ID,
					RoleID: platformAdminRoleID,
					Scope:  ScopePlatform,
				})
			}
		}

		if err := seedScopedRolesForWorkspaces(ctx, qtx); err != nil {
			return err
		}

		if err := seedScopedRolesForNamespaces(ctx, qtx); err != nil {
			return err
		}

		return migrateGlobalRolesToScoped(ctx, s.DB, qtx)
	})
}

// seedBuiltinRoles upserts all built-in role definitions and returns the platform-admin role ID.
func seedBuiltinRoles(ctx context.Context, qtx *generated.Queries, roles []BuiltinRoleDef) (int64, error) {
	var platformAdminRoleID int64
	for _, def := range roles {
		role, err := qtx.UpsertRole(ctx, generated.UpsertRoleParams{
			Name:        def.Name,
			DisplayName: def.DisplayName,
			Description: def.Description,
			Scope:       def.Scope,
			Builtin:     true,
		})
		if err != nil {
			return 0, fmt.Errorf("upsert builtin role %s: %w", def.Name, err)
		}

		if def.Name == RolePlatformAdmin {
			platformAdminRoleID = role.ID
		}

		if err := qtx.DeleteRolePermissionRules(ctx, role.ID); err != nil {
			return 0, fmt.Errorf("delete rules for role %s: %w", def.Name, err)
		}

		for _, pattern := range def.Rules {
			if err := qtx.AddRolePermissionRule(ctx, generated.AddRolePermissionRuleParams{
				RoleID:  role.ID,
				Pattern: pattern,
			}); err != nil {
				return 0, fmt.Errorf("add rule %q for role %s: %w", pattern, def.Name, err)
			}
		}
	}
	return platformAdminRoleID, nil
}

// seedScopedRolesForWorkspaces creates built-in workspace roles for all existing workspaces.
func seedScopedRolesForWorkspaces(ctx context.Context, qtx *generated.Queries) error {
	workspaceIDs, err := qtx.ListAllWorkspaceIDs(ctx)
	if err != nil {
		return fmt.Errorf("list workspace IDs: %w", err)
	}
	for _, wsID := range workspaceIDs {
		if err := createBuiltinRolesInTx(ctx, qtx, WorkspaceBuiltinRoles(), &wsID, nil); err != nil {
			return fmt.Errorf("create workspace roles for workspace %d: %w", wsID, err)
		}
	}
	return nil
}

// seedScopedRolesForNamespaces creates built-in namespace roles for all existing namespaces.
func seedScopedRolesForNamespaces(ctx context.Context, qtx *generated.Queries) error {
	nsRows, err := qtx.ListAllNamespaceIDsWithWorkspace(ctx)
	if err != nil {
		return fmt.Errorf("list namespace IDs: %w", err)
	}
	for _, nsRow := range nsRows {
		if err := createBuiltinRolesInTx(ctx, qtx, NamespaceBuiltinRoles(), nil, &nsRow.ID); err != nil {
			return fmt.Errorf("create namespace roles for namespace %d: %w", nsRow.ID, err)
		}
	}
	return nil
}

// migrateGlobalRolesToScoped re-points role_bindings from old global roles to new scoped roles,
// then deletes the old global roles.
func migrateGlobalRolesToScoped(ctx context.Context, d *db.DB, q *generated.Queries) error {
	type migrationPair struct {
		roleName string
		scope    string
	}
	migrations := []migrationPair{
		{RoleWorkspaceAdmin, ScopeWorkspace},
		{RoleWorkspaceViewer, ScopeWorkspace},
		{RoleNamespaceAdmin, ScopeNamespace},
		{RoleNamespaceViewer, ScopeNamespace},
	}
	for _, m := range migrations {
		if err := migrateOneGlobalRole(ctx, q, m.roleName, m.scope); err != nil {
			return err
		}
	}
	return nil
}

// migrateOneGlobalRole migrates a single global role to scoped roles.
func migrateOneGlobalRole(ctx context.Context, q *generated.Queries, roleName, scope string) error {
	oldRoleID, err := q.GetGlobalRoleIDByNameAndScope(ctx, generated.GetGlobalRoleIDByNameAndScopeParams{
		Name:  roleName,
		Scope: scope,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("find global role %s: %w", roleName, err)
	}

	if scope == ScopeWorkspace {
		if err := migrateWorkspaceRoleBindings(ctx, q, oldRoleID, roleName, scope); err != nil {
			return err
		}
	} else {
		if err := migrateNamespaceRoleBindings(ctx, q, oldRoleID, roleName, scope); err != nil {
			return err
		}
	}

	if _, err := q.DeleteRole(ctx, oldRoleID); err != nil {
		return fmt.Errorf("delete old global role %s: %w", roleName, err)
	}
	logger.Infof("migrated role %s from global to scoped", roleName)
	return nil
}

// migrateWorkspaceRoleBindings re-points all workspace-scoped role_bindings of
// the old global role to the per-workspace scoped role of the same name.
func migrateWorkspaceRoleBindings(ctx context.Context, q *generated.Queries, oldRoleID int64, roleName, scope string) error {
	bindings, err := q.ListBindingIDsAndWorkspaceByRole(ctx, oldRoleID)
	if err != nil {
		return fmt.Errorf("list bindings for old role %s: %w", roleName, err)
	}
	for _, b := range bindings {
		if b.WorkspaceID == nil {
			continue
		}
		newRoleID, err := q.GetWorkspaceRoleIDByName(ctx, generated.GetWorkspaceRoleIDByNameParams{
			Name:        roleName,
			WorkspaceID: b.WorkspaceID,
		})
		if err != nil {
			logger.Warnf("cannot find scoped role %s for %s %d, skipping binding %d", roleName, scope, *b.WorkspaceID, b.ID)
			continue
		}
		if err := q.RepointRoleBindingRole(ctx, generated.RepointRoleBindingRoleParams{
			RoleID: newRoleID,
			ID:     b.ID,
		}); err != nil {
			return fmt.Errorf("re-point binding %d: %w", b.ID, err)
		}
	}
	return nil
}

// migrateNamespaceRoleBindings re-points all namespace-scoped role_bindings of
// the old global role to the per-namespace scoped role of the same name.
func migrateNamespaceRoleBindings(ctx context.Context, q *generated.Queries, oldRoleID int64, roleName, scope string) error {
	bindings, err := q.ListBindingIDsAndNamespaceByRole(ctx, oldRoleID)
	if err != nil {
		return fmt.Errorf("list bindings for old role %s: %w", roleName, err)
	}
	for _, b := range bindings {
		if b.NamespaceID == nil {
			continue
		}
		newRoleID, err := q.GetNamespaceRoleIDByName(ctx, generated.GetNamespaceRoleIDByNameParams{
			Name:        roleName,
			NamespaceID: b.NamespaceID,
		})
		if err != nil {
			logger.Warnf("cannot find scoped role %s for %s %d, skipping binding %d", roleName, scope, *b.NamespaceID, b.ID)
			continue
		}
		if err := q.RepointRoleBindingRole(ctx, generated.RepointRoleBindingRoleParams{
			RoleID: newRoleID,
			ID:     b.ID,
		}); err != nil {
			return fmt.Errorf("re-point binding %d: %w", b.ID, err)
		}
	}
	return nil
}

func (s *pgRoleStore) SetPermissionRules(ctx context.Context, roleID int64, patterns []string) error {
	return s.DB.WithTx(ctx, func(ctx context.Context, qtx *generated.Queries) error {
		if err := qtx.DeleteRolePermissionRules(ctx, roleID); err != nil {
			return fmt.Errorf("delete existing rules: %w", err)
		}
		for _, pattern := range patterns {
			if err := qtx.AddRolePermissionRule(ctx, generated.AddRolePermissionRuleParams{
				RoleID:  roleID,
				Pattern: pattern,
			}); err != nil {
				return fmt.Errorf("add rule %q: %w", pattern, err)
			}
		}
		return nil
	})
}
