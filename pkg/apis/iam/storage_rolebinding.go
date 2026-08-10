package iam

import (
	"fmt"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// ===== roleBindingOps 平台级角色绑定 =====

// +openapi:resource=RoleBinding
// +openapi:path=/rolebindings
type roleBindingOps struct {
	rbStore   modstore.RoleBindingStore
	roleStore modstore.RoleStore
}

// RoleBindingsDef declares the platform-level rolebindings resource
// (list / create / delete / batch delete; no Get).
func RoleBindingsDef(rbStore modstore.RoleBindingStore, roleStore modstore.RoleStore) apiserver.ResourceDef[RoleBinding] {
	o := roleBindingOps{rbStore: rbStore, roleStore: roleStore}
	return apiserver.ResourceDef[RoleBinding]{
		Group: "iam", Name: "rolebindings",
		Ops: apiserver.Ops[RoleBinding]{
			List:        o.list,
			Create:      o.create,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
	}
}

// +openapi:summary=获取平台级角色绑定列表
func (o roleBindingOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[RoleBinding], error) {
	result, err := o.rbStore.ListPlatform(ctx, query)
	if err != nil {
		return nil, domainErr(err)
	}
	return roleBindingListResult(result), nil
}

// +openapi:summary=创建平台级角色绑定
func (o roleBindingOps) create(ctx apiserver.Ctx, rb *RoleBinding) (*RoleBinding, error) {
	if errs := ValidateRoleBindingCreate(&rb.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	roleID, err := parseID(rb.Spec.RoleID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid role ID: %s", rb.Spec.RoleID), nil)
	}
	role, err := o.roleStore.GetByID(ctx, roleID)
	if err != nil {
		return nil, domainErr(err)
	}
	if role.Scope != modstore.ScopePlatform {
		return nil, apierrors.NewBadRequest("role scope must be 'platform' for platform-level bindings", nil)
	}

	userID, err := parseID(rb.Spec.UserID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid user ID: %s", rb.Spec.UserID), nil)
	}

	if ctx.DryRun {
		return rb, nil
	}

	created, err := o.rbStore.Create(ctx, modstore.RoleBindingCreateInput{
		UserID: userID,
		RoleID: roleID,
		Scope:  modstore.ScopePlatform,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return roleBindingToAPI(created, "", "", role.Name, role.DisplayName), nil
}

// +openapi:summary=删除平台级角色绑定
func (o roleBindingOps) delete(ctx apiserver.Ctx, id int64) error {
	return deleteRoleBinding(ctx, o.rbStore, id)
}

// +openapi:summary=批量删除平台级角色绑定
func (o roleBindingOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	return deleteRoleBindingsCollection(ctx, o.rbStore, ids)
}

// ===== workspaceRoleBindingOps 租户级角色绑定 =====

// +openapi:resource=RoleBinding
// +openapi:path=/workspaces/{workspaceId}/rolebindings
type workspaceRoleBindingOps struct {
	rbStore   modstore.RoleBindingStore
	roleStore modstore.RoleStore
}

// WorkspaceRoleBindingsDef declares the workspace-level rolebindings resource.
func WorkspaceRoleBindingsDef(rbStore modstore.RoleBindingStore, roleStore modstore.RoleStore) apiserver.ResourceDef[RoleBinding] {
	o := workspaceRoleBindingOps{rbStore: rbStore, roleStore: roleStore}
	return apiserver.ResourceDef[RoleBinding]{
		Group: "iam", Name: "rolebindings",
		Scopes: apiserver.ScopeWorkspace,
		Ops: apiserver.Ops[RoleBinding]{
			List:        o.list,
			Create:      o.create,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
	}
}

// +openapi:summary=获取租户级角色绑定列表
func (o workspaceRoleBindingOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[RoleBinding], error) {
	result, err := o.rbStore.ListByWorkspaceID(ctx, ctx.Scope.WorkspaceID, query)
	if err != nil {
		return nil, domainErr(err)
	}
	return roleBindingListResult(result), nil
}

// +openapi:summary=创建租户级角色绑定
func (o workspaceRoleBindingOps) create(ctx apiserver.Ctx, rb *RoleBinding) (*RoleBinding, error) {
	if errs := ValidateRoleBindingCreate(&rb.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	wsID := ctx.Scope.WorkspaceID

	roleID, err := parseID(rb.Spec.RoleID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid role ID: %s", rb.Spec.RoleID), nil)
	}
	role, err := o.roleStore.GetByID(ctx, roleID)
	if err != nil {
		return nil, domainErr(err)
	}
	if role.Scope != modstore.ScopeWorkspace {
		return nil, apierrors.NewBadRequest("role scope must be 'workspace' for workspace-level bindings", nil)
	}
	if role.WorkspaceID == nil || *role.WorkspaceID != wsID {
		return nil, apierrors.NewBadRequest("role does not belong to this workspace", nil)
	}

	userID, err := parseID(rb.Spec.UserID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid user ID: %s", rb.Spec.UserID), nil)
	}

	if ctx.DryRun {
		return rb, nil
	}

	created, err := o.rbStore.Create(ctx, modstore.RoleBindingCreateInput{
		UserID:      userID,
		RoleID:      roleID,
		Scope:       modstore.ScopeWorkspace,
		WorkspaceID: &wsID,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return roleBindingToAPI(created, "", "", role.Name, role.DisplayName), nil
}

// +openapi:summary=删除租户级角色绑定
func (o workspaceRoleBindingOps) delete(ctx apiserver.Ctx, id int64) error {
	return deleteRoleBinding(ctx, o.rbStore, id)
}

// +openapi:summary=批量删除租户级角色绑定
func (o workspaceRoleBindingOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	return deleteRoleBindingsCollection(ctx, o.rbStore, ids)
}

// ===== namespaceRoleBindingOps 项目级角色绑定 =====

// +openapi:resource=RoleBinding
// +openapi:path=/workspaces/{workspaceId}/namespaces/{namespaceId}/rolebindings
type namespaceRoleBindingOps struct {
	rbStore   modstore.RoleBindingStore
	roleStore modstore.RoleStore
	nsStore   modstore.NamespaceStore
}

// NamespaceRoleBindingsDef declares the namespace-level rolebindings resource.
func NamespaceRoleBindingsDef(rbStore modstore.RoleBindingStore, roleStore modstore.RoleStore, nsStore modstore.NamespaceStore) apiserver.ResourceDef[RoleBinding] {
	o := namespaceRoleBindingOps{rbStore: rbStore, roleStore: roleStore, nsStore: nsStore}
	return apiserver.ResourceDef[RoleBinding]{
		Group: "iam", Name: "rolebindings",
		Scopes: apiserver.ScopeNamespace,
		Ops: apiserver.Ops[RoleBinding]{
			List:        o.list,
			Create:      o.create,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
	}
}

// +openapi:summary=获取项目级角色绑定列表
// +openapi:summary.workspaces.namespaces.rolebindings=获取租户下项目的角色绑定列表
func (o namespaceRoleBindingOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[RoleBinding], error) {
	result, err := o.rbStore.ListByNamespaceID(ctx, ctx.Scope.NamespaceID, query)
	if err != nil {
		return nil, domainErr(err)
	}
	return roleBindingListResult(result), nil
}

// +openapi:summary=创建项目级角色绑定
// +openapi:summary.workspaces.namespaces.rolebindings=创建租户下项目的角色绑定
func (o namespaceRoleBindingOps) create(ctx apiserver.Ctx, rb *RoleBinding) (*RoleBinding, error) {
	if errs := ValidateRoleBindingCreate(&rb.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	nsID := ctx.Scope.NamespaceID

	// Look up namespace to get workspace ID
	ns, err := o.nsStore.GetByID(ctx, nsID)
	if err != nil {
		return nil, domainErr(err)
	}
	wsID := ns.WorkspaceID

	roleID, err := parseID(rb.Spec.RoleID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid role ID: %s", rb.Spec.RoleID), nil)
	}
	role, err := o.roleStore.GetByID(ctx, roleID)
	if err != nil {
		return nil, domainErr(err)
	}
	if role.Scope != modstore.ScopeNamespace {
		return nil, apierrors.NewBadRequest("role scope must be 'namespace' for namespace-level bindings", nil)
	}
	if role.NamespaceID == nil || *role.NamespaceID != nsID {
		return nil, apierrors.NewBadRequest("role does not belong to this namespace", nil)
	}

	userID, err := parseID(rb.Spec.UserID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid user ID: %s", rb.Spec.UserID), nil)
	}

	if ctx.DryRun {
		return rb, nil
	}

	created, err := o.rbStore.Create(ctx, modstore.RoleBindingCreateInput{
		UserID:      userID,
		RoleID:      roleID,
		Scope:       modstore.ScopeNamespace,
		WorkspaceID: &wsID,
		NamespaceID: &nsID,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return roleBindingToAPI(created, "", "", role.Name, role.DisplayName), nil
}

// +openapi:summary=删除项目级角色绑定
// +openapi:summary.workspaces.namespaces.rolebindings=删除租户下项目的角色绑定
func (o namespaceRoleBindingOps) delete(ctx apiserver.Ctx, id int64) error {
	return deleteRoleBinding(ctx, o.rbStore, id)
}

// +openapi:summary=批量删除项目级角色绑定
// +openapi:summary.workspaces.namespaces.rolebindings=批量删除租户下项目的角色绑定
func (o namespaceRoleBindingOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	return deleteRoleBindingsCollection(ctx, o.rbStore, ids)
}

// deleteRoleBinding is shared by the three RoleBinding ops variants:
// owner bindings are protected, dryRun validates without deleting.
func deleteRoleBinding(ctx apiserver.Ctx, rbStore modstore.RoleBindingStore, id int64) error {
	existing, err := rbStore.GetByID(ctx, id)
	if err != nil {
		return domainErr(err)
	}
	if existing.IsOwner {
		return apierrors.NewBadRequest("cannot delete owner role binding", nil)
	}

	if ctx.DryRun {
		return nil
	}

	return rbStore.Delete(ctx, id)
}

// deleteRoleBindingsCollection is shared by the three RoleBinding ops
// variants (platform / workspace / namespace). The store layer enforces the
// owner-binding guard (is_owner=false) so the same call is safe at every
// scope without re-fetching each row.
func deleteRoleBindingsCollection(ctx apiserver.Ctx, rbStore modstore.RoleBindingStore, ids []int64) (*apiserver.BatchResult, error) {
	if ctx.DryRun {
		return &apiserver.BatchResult{SuccessCount: len(ids), FailedCount: 0}, nil
	}

	deleted, err := rbStore.DeleteByIDs(ctx, ids)
	if err != nil {
		return nil, domainErr(err)
	}
	// Owner rows are silently skipped by the SQL filter; account for them
	// in FailedCount so the FE can surface "X of Y deleted" if it wants.
	return &apiserver.BatchResult{
		SuccessCount: int(deleted),
		FailedCount:  len(ids) - int(deleted),
	}, nil
}
