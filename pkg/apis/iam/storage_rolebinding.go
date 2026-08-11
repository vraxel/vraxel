package iam

import (
	"encoding/json"
	"fmt"
	"vraxel.io/vraxel/lib/runtime"

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
			CreateAny:   o.create,
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

// batchBindRole is the shared body of the three scoped create handlers.
// A binding row is one (user, role) pair, so granting a role to N users
// writes N rows -- CreateMany puts them in one transaction so a failure
// halfway through cannot leave a partially authorized set.
//
// The role is resolved and scope-checked once, not per user: every row
// in the request targets the same role, and rejecting the request as a
// whole is clearer than reporting the same error N times.
func batchBindRole(
	ctx apiserver.Ctx,
	rbStore modstore.RoleBindingStore,
	roleStore modstore.RoleStore,
	body json.RawMessage,
	scope string,
	workspaceID, namespaceID *int64,
	checkRole func(role *modstore.RoleWithRulesRow) error,
) (any, error) {
	req, err := apiserver.DecodePatch[BatchRequest](body)
	if err != nil {
		return nil, err
	}
	if len(req.IDs) == 0 {
		return nil, apierrors.NewBadRequest("ids is required", nil)
	}
	if req.RoleID == "" {
		return nil, apierrors.NewBadRequest("roleId is required", nil)
	}

	roleID, err := parseID(req.RoleID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid role ID: %s", req.RoleID), nil)
	}
	role, err := roleStore.GetByID(ctx, roleID)
	if err != nil {
		return nil, domainErr(err)
	}
	if err := checkRole(role); err != nil {
		return nil, err
	}

	inputs := make([]modstore.RoleBindingCreateInput, 0, len(req.IDs))
	for _, raw := range req.IDs {
		userID, err := parseID(raw)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid user ID: %s", raw), nil)
		}
		inputs = append(inputs, modstore.RoleBindingCreateInput{
			UserID:      userID,
			RoleID:      roleID,
			Scope:       scope,
			WorkspaceID: workspaceID,
			NamespaceID: namespaceID,
		})
	}

	if ctx.DryRun {
		return &apiserver.BatchResult{
			TypeMeta:     runtime.TypeMeta{Kind: "Result"},
			SuccessCount: len(inputs),
		}, nil
	}

	created, err := rbStore.CreateMany(ctx, inputs)
	if err != nil {
		return nil, domainErr(err)
	}
	// Insertion is idempotent, so anything not newly created was already
	// bound. That is not a failure -- report it separately so the UI can
	// say "3 added, 2 already had this role" instead of claiming 5 grants.
	return &apiserver.BatchResult{
		TypeMeta:     runtime.TypeMeta{Kind: "Result"},
		SuccessCount: created,
		FailedCount:  len(inputs) - created,
	}, nil
}

// +openapi:summary=创建平台级角色绑定
func (o roleBindingOps) create(ctx apiserver.Ctx, body json.RawMessage) (any, error) {
	return batchBindRole(ctx, o.rbStore, o.roleStore, body, modstore.ScopePlatform, nil, nil,
		func(role *modstore.RoleWithRulesRow) error {
			if role.Scope != modstore.ScopePlatform {
				return apierrors.NewBadRequest("role scope must be 'platform' for platform-level bindings", nil)
			}
			return nil
		})
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
			CreateAny:   o.create,
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
func (o workspaceRoleBindingOps) create(ctx apiserver.Ctx, body json.RawMessage) (any, error) {
	wsID := ctx.Scope.WorkspaceID
	return batchBindRole(ctx, o.rbStore, o.roleStore, body, modstore.ScopeWorkspace, &wsID, nil,
		func(role *modstore.RoleWithRulesRow) error {
			if role.Scope != modstore.ScopeWorkspace {
				return apierrors.NewBadRequest("role scope must be 'workspace' for workspace-level bindings", nil)
			}
			if role.WorkspaceID == nil || *role.WorkspaceID != wsID {
				return apierrors.NewBadRequest("role does not belong to this workspace", nil)
			}
			return nil
		})
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
			CreateAny:   o.create,
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
func (o namespaceRoleBindingOps) create(ctx apiserver.Ctx, body json.RawMessage) (any, error) {
	nsID := ctx.Scope.NamespaceID
	ns, err := o.nsStore.GetByID(ctx, nsID)
	if err != nil {
		return nil, domainErr(err)
	}
	wsID := ns.WorkspaceID
	return batchBindRole(ctx, o.rbStore, o.roleStore, body, modstore.ScopeNamespace, &wsID, &nsID,
		func(role *modstore.RoleWithRulesRow) error {
			if role.Scope != modstore.ScopeNamespace {
				return apierrors.NewBadRequest("role scope must be 'namespace' for namespace-level bindings", nil)
			}
			if role.NamespaceID == nil || *role.NamespaceID != nsID {
				return apierrors.NewBadRequest("role does not belong to this namespace", nil)
			}
			return nil
		})
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
