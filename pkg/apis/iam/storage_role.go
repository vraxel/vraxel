package iam

import (
	"strconv"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/api/types"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/runtime"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// --- Role Ops (platform CRUD) ---

type roleOps struct {
	roleStore modstore.RoleStore
	rbStore   modstore.RoleBindingStore
	permStore modstore.PermissionStore
}

// RolesDef declares the platform roles resource with full CRUD (no Patch).
func RolesDef(roleStore modstore.RoleStore, rbStore modstore.RoleBindingStore, permStore modstore.PermissionStore) apiserver.ResourceDef[Role] {
	o := roleOps{roleStore: roleStore, rbStore: rbStore, permStore: permStore}
	return apiserver.ResourceDef[Role]{
		Group: "iam", Name: "roles",
		Ops: apiserver.Ops[Role]{
			List:        o.list,
			Get:         o.get,
			Create:      o.create,
			Update:      o.update,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
	}
}

// +openapi:summary=获取平台角色详情
func (o roleOps) get(ctx apiserver.Ctx, id int64) (*Role, error) {
	role, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	if role.Scope != modstore.ScopePlatform {
		return nil, apierrors.NewNotFound("role", strconv.FormatInt(id, 10))
	}

	return roleWithRulesToAPI(role), nil
}

// +openapi:summary=获取平台角色列表
func (o roleOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[Role], error) {
	// Force platform scope — only return platform roles
	query.Filters["scope"] = modstore.ScopePlatform

	result, err := o.roleStore.List(ctx, query)
	if err != nil {
		return nil, domainErr(err)
	}

	items := make([]Role, len(result.Items))
	for i, item := range result.Items {
		items[i] = *roleListRowToAPI(&item)
	}

	return &list.Result[Role]{Items: items, TotalCount: result.TotalCount}, nil
}

// +openapi:summary=创建平台角色
func (o roleOps) create(ctx apiserver.Ctx, role *Role) (*Role, error) {
	// Force platform scope — scoped roles must be created via workspace/namespace endpoints
	role.Spec.Scope = modstore.ScopePlatform

	if errs := ValidateRoleCreate(&role.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	codeScopes, err := o.permStore.ListCodeScopes(ctx)
	if err != nil {
		return nil, domainErr(err)
	}
	if scopeErrs := ValidateRuleScopes(role.Spec.Scope, role.Spec.Rules, codeScopes); scopeErrs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", scopeErrs)
	}

	if ctx.DryRun {
		return role, nil
	}

	created, err := o.roleStore.Create(ctx, modstore.RoleCreateInput{
		Name:        role.Spec.Name,
		DisplayName: role.Spec.DisplayName,
		Description: role.Spec.Description,
		Scope:       modstore.ScopePlatform,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	if len(role.Spec.Rules) > 0 {
		if err := o.roleStore.SetPermissionRules(ctx, created.ID, role.Spec.Rules); err != nil {
			return nil, domainErr(err)
		}
	}

	// Re-fetch to include rules
	withRules, err := o.roleStore.GetByID(ctx, created.ID)
	if err != nil {
		return nil, domainErr(err)
	}

	return roleWithRulesToAPI(withRules), nil
}

// +openapi:summary=更新平台角色
func (o roleOps) update(ctx apiserver.Ctx, id int64, role *Role) (*Role, error) {
	existing, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}
	if existing.Scope != modstore.ScopePlatform {
		return nil, apierrors.NewNotFound("role", strconv.FormatInt(id, 10))
	}
	if existing.Builtin {
		return nil, apierrors.NewBadRequest("cannot modify built-in role", nil)
	}

	if errs := ValidateRoleUpdate(&role.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	codeScopes, err := o.permStore.ListCodeScopes(ctx)
	if err != nil {
		return nil, domainErr(err)
	}
	if scopeErrs := ValidateRuleScopes(existing.Scope, role.Spec.Rules, codeScopes); scopeErrs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", scopeErrs)
	}

	if ctx.DryRun {
		return role, nil
	}

	if _, err := o.roleStore.Update(ctx, modstore.RoleUpdateInput{
		ID:          id,
		Name:        existing.Name,
		DisplayName: role.Spec.DisplayName,
		Description: role.Spec.Description,
	}); err != nil {
		return nil, domainErr(err)
	}

	if len(role.Spec.Rules) > 0 {
		if err := o.roleStore.SetPermissionRules(ctx, id, role.Spec.Rules); err != nil {
			return nil, domainErr(err)
		}
	}

	withRules, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	return roleWithRulesToAPI(withRules), nil
}

// +openapi:summary=删除平台角色
func (o roleOps) delete(ctx apiserver.Ctx, id int64) error {
	existing, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return domainErr(err)
	}
	if existing.Scope != modstore.ScopePlatform {
		return apierrors.NewNotFound("role", strconv.FormatInt(id, 10))
	}
	if existing.Builtin {
		return apierrors.NewBadRequest("cannot delete built-in role", nil)
	}

	count, err := o.rbStore.CountByRoleAndScope(ctx, id, modstore.ScopePlatform)
	if err != nil {
		return domainErr(err)
	}
	if count > 0 {
		return apierrors.NewBadRequest("cannot delete role with active bindings", nil)
	}

	if ctx.DryRun {
		return nil
	}

	return o.roleStore.Delete(ctx, id)
}

// +openapi:summary=批量删除平台角色
func (o roleOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	success := 0
	for _, rid := range ids {
		existing, err := o.roleStore.GetByID(ctx, rid)
		if err != nil {
			continue
		}
		if existing.Scope != modstore.ScopePlatform || existing.Builtin {
			continue
		}
		count, err := o.rbStore.CountByRoleAndScope(ctx, rid, modstore.ScopePlatform)
		if err != nil || count > 0 {
			continue
		}
		if ctx.DryRun {
			success++
			continue
		}
		if err := o.roleStore.Delete(ctx, rid); err == nil {
			success++
		}
	}
	return &apiserver.BatchResult{
		SuccessCount: success,
		FailedCount:  len(ids) - success,
	}, nil
}

func roleToAPI(r *modstore.RoleRow) *Role {
	createdAt := r.CreatedAt
	updatedAt := r.UpdatedAt
	return &Role{
		TypeMeta: runtime.TypeMeta{Kind: "Role"},
		ObjectMeta: types.ObjectMeta{
			ID:        strconv.FormatInt(r.ID, 10),
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		},
		Spec: RoleSpec{
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Description: r.Description,
			Scope:       r.Scope,
			Builtin:     r.Builtin,
		},
	}
}

func roleListRowToAPI(r *modstore.RoleListItem) *Role {
	rc := r.RuleCount
	role := roleToAPI(&r.RoleRow)
	role.Spec.RuleCount = &rc
	return role
}

func roleWithRulesToAPI(r *modstore.RoleWithRulesRow) *Role {
	role := roleToAPI(&r.RoleRow)
	role.Spec.Rules = r.Rules
	return role
}

// --- Scoped Role Ops (full CRUD, for workspace/namespace scope) ---

// scopedRoleOps 作用域角色的完整 CRUD 实现，按 scope 过滤角色列表。
// 注册为 /workspaces/{workspaceId}/roles 和
// /workspaces/{workspaceId}/namespaces/{namespaceId}/roles。
// +openapi:resource=Role
// +openapi:path=/workspaces/{workspaceId}/roles
// +openapi:path=/workspaces/{workspaceId}/namespaces/{namespaceId}/roles
type scopedRoleOps struct {
	roleStore modstore.RoleStore
	rbStore   modstore.RoleBindingStore
	permStore modstore.PermissionStore
	scope     string // modstore.ScopeWorkspace or modstore.ScopeNamespace
}

// ScopedRolesDef declares the roles resource at one tenant scope
// (workspace or namespace) — distinct store scope filters per level,
// mirroring v1's per-scope storages.
func ScopedRolesDef(roleStore modstore.RoleStore, rbStore modstore.RoleBindingStore, permStore modstore.PermissionStore, scope string) apiserver.ResourceDef[Role] {
	o := scopedRoleOps{roleStore: roleStore, rbStore: rbStore, permStore: permStore, scope: scope}
	scopes := apiserver.ScopeWorkspace
	if scope == modstore.ScopeNamespace {
		scopes = apiserver.ScopeNamespace
	}
	return apiserver.ResourceDef[Role]{
		Group: "iam", Name: "roles",
		Scopes: scopes,
		Ops: apiserver.Ops[Role]{
			List:        o.list,
			Get:         o.get,
			Create:      o.create,
			Update:      o.update,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
	}
}

// scopeOwnerID returns the scope-specific resource ID from the request scope.
func (o scopedRoleOps) scopeOwnerID(ctx apiserver.Ctx) int64 {
	if o.scope == modstore.ScopeWorkspace {
		return ctx.Scope.WorkspaceID
	}
	return ctx.Scope.NamespaceID
}

// verifyScopeOwnership confirms the role belongs to this scope and to
// the scope owner identified by the request path. Returns NotFound when
// ownership does not match.
func (o scopedRoleOps) verifyScopeOwnership(existing *modstore.RoleWithRulesRow, id string, ownerID int64) error {
	if existing.Scope != o.scope {
		return apierrors.NewNotFound("role", id)
	}
	if o.scope == modstore.ScopeWorkspace {
		if existing.WorkspaceID == nil || *existing.WorkspaceID != ownerID {
			return apierrors.NewNotFound("role", id)
		}
	} else {
		if existing.NamespaceID == nil || *existing.NamespaceID != ownerID {
			return apierrors.NewNotFound("role", id)
		}
	}
	return nil
}

// +openapi:summary=获取租户角色列表
// +openapi:summary.workspaces.namespaces.roles=获取租户下项目的角色列表
func (o scopedRoleOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[Role], error) {
	query.Filters["scope"] = o.scope

	if o.scope == modstore.ScopeWorkspace {
		query.Filters["workspace_id"] = ctx.Scope.WorkspaceID
	} else {
		query.Filters["namespace_id"] = ctx.Scope.NamespaceID
	}

	result, err := o.roleStore.List(ctx, query)
	if err != nil {
		return nil, domainErr(err)
	}

	items := make([]Role, len(result.Items))
	for i, item := range result.Items {
		items[i] = *roleListRowToAPI(&item)
	}

	return &list.Result[Role]{Items: items, TotalCount: result.TotalCount}, nil
}

// +openapi:summary=获取租户角色详情
// +openapi:summary.workspaces.namespaces.roles=获取租户下项目的角色详情
func (o scopedRoleOps) get(ctx apiserver.Ctx, id int64) (*Role, error) {
	role, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	if err := o.verifyScopeOwnership(role, strconv.FormatInt(id, 10), o.scopeOwnerID(ctx)); err != nil {
		return nil, err
	}

	return roleWithRulesToAPI(role), nil
}

// +openapi:summary=创建租户自定义角色
// +openapi:summary.workspaces.namespaces.roles=创建租户下项目的自定义角色
func (o scopedRoleOps) create(ctx apiserver.Ctx, role *Role) (*Role, error) {
	role.Spec.Scope = o.scope

	if errs := ValidateRoleCreate(&role.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	codeScopes, err := o.permStore.ListCodeScopes(ctx)
	if err != nil {
		return nil, domainErr(err)
	}
	if scopeErrs := ValidateRuleScopes(role.Spec.Scope, role.Spec.Rules, codeScopes); scopeErrs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", scopeErrs)
	}

	if ctx.DryRun {
		return role, nil
	}

	dbRole := modstore.RoleCreateInput{
		Name:        role.Spec.Name,
		DisplayName: role.Spec.DisplayName,
		Description: role.Spec.Description,
		Scope:       o.scope,
	}

	ownerID := o.scopeOwnerID(ctx)
	if o.scope == modstore.ScopeWorkspace {
		dbRole.WorkspaceID = &ownerID
	} else {
		dbRole.NamespaceID = &ownerID
	}

	created, err := o.roleStore.Create(ctx, dbRole)
	if err != nil {
		return nil, domainErr(err)
	}

	if len(role.Spec.Rules) > 0 {
		if err := o.roleStore.SetPermissionRules(ctx, created.ID, role.Spec.Rules); err != nil {
			return nil, domainErr(err)
		}
	}

	withRules, err := o.roleStore.GetByID(ctx, created.ID)
	if err != nil {
		return nil, domainErr(err)
	}

	return roleWithRulesToAPI(withRules), nil
}

// +openapi:summary=更新租户角色
// +openapi:summary.workspaces.namespaces.roles=更新租户下项目的角色
func (o scopedRoleOps) update(ctx apiserver.Ctx, id int64, role *Role) (*Role, error) {
	existing, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}
	if existing.Builtin {
		return nil, apierrors.NewBadRequest("cannot modify built-in role", nil)
	}

	if err := o.verifyScopeOwnership(existing, strconv.FormatInt(id, 10), o.scopeOwnerID(ctx)); err != nil {
		return nil, err
	}

	if errs := ValidateRoleUpdate(&role.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	codeScopes, err := o.permStore.ListCodeScopes(ctx)
	if err != nil {
		return nil, domainErr(err)
	}
	if scopeErrs := ValidateRuleScopes(o.scope, role.Spec.Rules, codeScopes); scopeErrs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", scopeErrs)
	}

	if ctx.DryRun {
		return role, nil
	}

	if _, err := o.roleStore.Update(ctx, modstore.RoleUpdateInput{
		ID:          id,
		Name:        existing.Name,
		DisplayName: role.Spec.DisplayName,
		Description: role.Spec.Description,
	}); err != nil {
		return nil, domainErr(err)
	}

	if len(role.Spec.Rules) > 0 {
		if err := o.roleStore.SetPermissionRules(ctx, id, role.Spec.Rules); err != nil {
			return nil, domainErr(err)
		}
	}

	withRules, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	return roleWithRulesToAPI(withRules), nil
}

// +openapi:summary=删除租户角色
// +openapi:summary.workspaces.namespaces.roles=删除租户下项目的角色
func (o scopedRoleOps) delete(ctx apiserver.Ctx, id int64) error {
	existing, err := o.roleStore.GetByID(ctx, id)
	if err != nil {
		return domainErr(err)
	}
	if existing.Builtin {
		return apierrors.NewBadRequest("cannot delete built-in role", nil)
	}

	if err := o.verifyScopeOwnership(existing, strconv.FormatInt(id, 10), o.scopeOwnerID(ctx)); err != nil {
		return err
	}

	// Check no bindings exist
	count, err := o.rbStore.CountByRoleAndScope(ctx, id, o.scope)
	if err != nil {
		return domainErr(err)
	}
	if count > 0 {
		return apierrors.NewBadRequest("cannot delete role with active bindings", nil)
	}

	if ctx.DryRun {
		return nil
	}

	return o.roleStore.Delete(ctx, id)
}

// +openapi:summary=批量删除作用域角色
func (o scopedRoleOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	ownerID := o.scopeOwnerID(ctx)

	success := 0
	for _, rid := range ids {
		if !o.deleteCollectionEligible(ctx, rid, ownerID) {
			continue
		}
		if ctx.DryRun {
			success++
			continue
		}
		if err := o.roleStore.Delete(ctx, rid); err == nil {
			success++
		}
	}
	return &apiserver.BatchResult{
		SuccessCount: success,
		FailedCount:  len(ids) - success,
	}, nil
}

// deleteCollectionEligible reports whether the role rid may be deleted in a
// batch delete: it must exist, match this scope, not be built-in, be owned by
// ownerID for the scope, and have no active bindings.
func (o scopedRoleOps) deleteCollectionEligible(ctx apiserver.Ctx, rid, ownerID int64) bool {
	existing, err := o.roleStore.GetByID(ctx, rid)
	if err != nil {
		return false
	}
	if existing.Scope != o.scope || existing.Builtin {
		return false
	}
	if o.scope == modstore.ScopeWorkspace {
		if existing.WorkspaceID == nil || *existing.WorkspaceID != ownerID {
			return false
		}
	} else {
		if existing.NamespaceID == nil || *existing.NamespaceID != ownerID {
			return false
		}
	}
	count, err := o.rbStore.CountByRoleAndScope(ctx, rid, o.scope)
	if err != nil || count > 0 {
		return false
	}
	return true
}
