package iam

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/rest/filters"
	"vraxel.io/vraxel/lib/runtime"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// ===== namespaceOps 项目存储 =====

// namespaceOps 项目资源的典型 Ops 实现。支持按租户筛选项目列表。
// +openapi:path=/workspaces/{workspaceId}/namespaces
type namespaceOps struct {
	nsStore   modstore.NamespaceStore
	wsStore   modstore.WorkspaceStore
	userStore modstore.UserStore
	rbStore   modstore.RoleBindingStore
}

// NamespacesDef declares the namespaces resource at platform + workspace
// scope. The workspace-level registration reproduces v1's
// /workspaces/{id}/namespaces nesting (same URL, same iam:namespaces
// workspace-scope code after v1's scope strip); the platform list
// injects the binding-scoped AccessFilter for non-admins.
func NamespacesDef(nsStore modstore.NamespaceStore, wsStore modstore.WorkspaceStore, userStore modstore.UserStore, rbStore modstore.RoleBindingStore) apiserver.ResourceDef[Namespace] {
	o := namespaceOps{nsStore: nsStore, wsStore: wsStore, userStore: userStore, rbStore: rbStore}
	return apiserver.ResourceDef[Namespace]{
		Group: "iam", Name: "namespaces",
		Scopes: apiserver.ScopePlatform | apiserver.ScopeWorkspace,
		Ops: apiserver.Ops[Namespace]{
			List:        o.list,
			Get:         o.get,
			Create:      o.create,
			Update:      o.update,
			Patch:       o.patch,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
		ListAccessScope: "namespaces",
		PermScope:       apiserver.ScopeNamespace,
		// Flat /namespaces/{id} (get/update/delete) authorizes at the
		// namespace's own scope, not platform. Resolve the parent workspace
		// so a workspace-admin binding (workspace rules apply to namespace
		// scope) matches; a namespace-admin matches on the namespace id
		// alone, so a lookup miss (wsID stays 0) still authorizes them.
		ItemScopeResolver: func(ctx context.Context, id int64) apiserver.ScopeInfo {
			wsID, _ := nsStore.WorkspaceIDOf(ctx, id)
			return apiserver.ScopeInfo{Level: apiserver.ScopeNamespace, WorkspaceID: wsID, NamespaceID: id}
		},
	}
}

// +openapi:summary=获取项目详情
// +openapi:summary.workspaces.namespaces=获取租户下的项目详情
func (o namespaceOps) get(ctx apiserver.Ctx, id int64) (*Namespace, error) {
	ns, err := o.nsStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	return namespaceWithOwnerToAPI(ns), nil
}

// +openapi:summary=获取项目列表
// +openapi:summary.workspaces.namespaces=获取租户下的项目列表
func (o namespaceOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[Namespace], error) {
	// If called via /workspaces/{workspaceId}/namespaces, filter by workspace
	if ctx.Scope.Level == apiserver.ScopeWorkspace {
		query.Filters["workspace_id"] = strconv.FormatInt(ctx.Scope.WorkspaceID, 10)
	}

	// Inject access filter for non-admin users
	if af := filters.AccessFilterFromContext(ctx); af != nil && af.NamespaceIDs != nil {
		query.Filters["accessible_ids"] = af.NamespaceIDs
	}

	result, err := o.nsStore.List(ctx, query)
	if err != nil {
		return nil, domainErr(err)
	}

	items := make([]Namespace, len(result.Items))
	for i, item := range result.Items {
		items[i] = *namespaceWithOwnerToAPI(&item)
	}

	return &list.Result[Namespace]{Items: items, TotalCount: result.TotalCount}, nil
}

// +openapi:summary=创建项目
// +openapi:summary.workspaces.namespaces=在租户下创建项目
func (o namespaceOps) create(ctx apiserver.Ctx, ns *Namespace) (*Namespace, error) {
	// If workspace ID comes from the scope path, use it
	if ctx.Scope.Level == apiserver.ScopeWorkspace {
		ns.Spec.WorkspaceID = strconv.FormatInt(ctx.Scope.WorkspaceID, 10)
	}

	// Auto-inject ownerId from authenticated user
	if userID, ok := oidc.UserIDFromContext(ctx); ok && ns.Spec.OwnerID == "" {
		ns.Spec.OwnerID = strconv.FormatInt(userID, 10)
	}

	if errs := ValidateNamespaceCreate(ns.ObjectMeta.Name, &ns.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	if ctx.DryRun {
		return ns, nil
	}

	workspaceID, err := parseID(ns.Spec.WorkspaceID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid workspaceId: %s", ns.Spec.WorkspaceID), nil)
	}

	ownerID, err := parseID(ns.Spec.OwnerID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid ownerId: %s", ns.Spec.OwnerID), nil)
	}

	// Check workspace exists
	if _, err := o.wsStore.GetByID(ctx, workspaceID); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("workspace %d not found", workspaceID), nil)
	}

	// Check owner exists
	if _, err := o.userStore.GetByID(ctx, ownerID); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("owner user %d not found", ownerID), nil)
	}

	created, err := o.nsStore.Create(ctx, modstore.NamespaceCreateInput{
		Name:        ns.ObjectMeta.Name,
		DisplayName: ns.Spec.DisplayName,
		Description: ns.Spec.Description,
		WorkspaceID: workspaceID,
		OwnerID:     ownerID,
		Visibility:  ns.Spec.Visibility,
		MaxMembers:  int32(ns.Spec.MaxMembers),
		Status:      ns.Spec.Status,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return namespaceWithOwnerToAPI(created), nil
}

// +openapi:summary=更新项目信息（全量）
// +openapi:summary.workspaces.namespaces=更新租户下的项目信息（全量）
func (o namespaceOps) update(ctx apiserver.Ctx, id int64, ns *Namespace) (*Namespace, error) {
	if errs := ValidateNamespaceUpdate(&ns.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	if ctx.DryRun {
		return ns, nil
	}

	ownerID, err := parseID(ns.Spec.OwnerID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid ownerId: %s", ns.Spec.OwnerID), nil)
	}

	// workspace_id is immutable on update; the SQL UPDATE no longer sets it
	// and any value submitted in the body is ignored.
	updated, err := o.nsStore.Update(ctx, modstore.NamespaceUpdateInput{
		ID:          id,
		Name:        ns.ObjectMeta.Name,
		DisplayName: ns.Spec.DisplayName,
		Description: ns.Spec.Description,
		OwnerID:     ownerID,
		Visibility:  ns.Spec.Visibility,
		MaxMembers:  int32(ns.Spec.MaxMembers),
		Status:      ns.Spec.Status,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return namespaceWithOwnerToAPI(updated), nil
}

// +openapi:summary=更新项目信息（部分）
// +openapi:summary.workspaces.namespaces=更新租户下的项目信息（部分）
func (o namespaceOps) patch(ctx apiserver.Ctx, id int64, body json.RawMessage) (*Namespace, error) {
	ns, err := apiserver.DecodePatch[Namespace](body)
	if err != nil {
		return nil, err
	}

	if ctx.DryRun {
		existing, err := o.get(ctx, id)
		if err != nil {
			return nil, domainErr(err)
		}
		return existing, nil
	}

	var ownerID int64
	if ns.Spec.OwnerID != "" {
		ownerID, err = parseID(ns.Spec.OwnerID)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid ownerId: %s", ns.Spec.OwnerID), nil)
		}
	}

	// workspace_id is immutable on patch; the SQL UPDATE no longer sets it
	// and any value submitted in the body is ignored.
	patched, err := o.nsStore.Patch(ctx, modstore.NamespaceUpdateInput{
		ID:          id,
		Name:        ns.ObjectMeta.Name,
		DisplayName: ns.Spec.DisplayName,
		Description: ns.Spec.Description,
		OwnerID:     ownerID,
		Visibility:  ns.Spec.Visibility,
		MaxMembers:  int32(ns.Spec.MaxMembers),
		Status:      ns.Spec.Status,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return namespaceWithOwnerToAPI(patched), nil
}

// +openapi:summary=删除项目
// +openapi:summary.workspaces.namespaces=删除租户下的项目
func (o namespaceOps) delete(ctx apiserver.Ctx, id int64) error {
	if ctx.DryRun {
		return nil
	}

	rows, err := o.nsStore.ListBlockingResources(ctx, id)
	if err != nil {
		return domainErr(err)
	}
	if blockErr := blockingResourceError("namespace", rows); blockErr != nil {
		return blockErr
	}

	if err := o.nsStore.Delete(ctx, id); err != nil {
		return domainErr(err)
	}
	return nil
}

// +openapi:summary=批量删除项目
// +openapi:summary.workspaces.namespaces=批量删除租户下的项目
func (o namespaceOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	if ctx.DryRun {
		return &apiserver.BatchResult{
			SuccessCount: len(ids),
			FailedCount:  0,
		}, nil
	}

	for _, nid := range ids {
		rows, err := o.nsStore.ListBlockingResources(ctx, nid)
		if err != nil {
			return nil, domainErr(err)
		}
		if blockErr := blockingResourceError("namespace", rows); blockErr != nil {
			return nil, blockErr
		}
	}

	count, err := o.nsStore.DeleteByIDs(ctx, ids)
	if err != nil {
		return nil, domainErr(err)
	}

	return &apiserver.BatchResult{
		SuccessCount: int(count),
		FailedCount:  len(ids) - int(count),
	}, nil
}

// ===== namespaceUserOps 项目成员存储 =====

// namespaceUserOps 管理项目的成员关系，支持查询成员列表、批量添加和批量移除成员。
// 添加项目成员时会自动将其加入父租户。
// +openapi:path=/workspaces/{workspaceId}/namespaces/{namespaceId}/users
type namespaceUserOps struct {
	rbStore   modstore.RoleBindingStore
	nsStore   modstore.NamespaceStore
	userStore modstore.UserStore
}

// NamespaceUsersDef declares the namespace-scope users (member) surface.
// Create is asymmetric — the body is a BatchRequest ({ids, roleId}) and
// the response a Result envelope, not User→User — hence CreateAny.
func NamespaceUsersDef(rbStore modstore.RoleBindingStore, nsStore modstore.NamespaceStore, userStore modstore.UserStore) apiserver.ResourceDef[User] {
	o := namespaceUserOps{rbStore: rbStore, nsStore: nsStore, userStore: userStore}
	return apiserver.ResourceDef[User]{
		Group: "iam", Name: "users",
		Scopes: apiserver.ScopeNamespace,
		Ops: apiserver.Ops[User]{
			List:        o.list,
			Get:         o.get,
			CreateAny:   o.create,
			BatchDelete: o.batchDelete,
		},
		Verbs: []apiserver.VerbDef{
			apiserver.Verb("rolebindings", NewNamespaceUserRoleBindingsVerb(rbStore)),
		},
	}
}

// +openapi:summary=获取项目成员详情
// +openapi:summary.workspaces.namespaces.users=获取租户下项目的成员详情
func (o namespaceUserOps) get(ctx apiserver.Ctx, id int64) (*User, error) {
	user, err := o.userStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	return userToAPI(user), nil
}

// +openapi:summary=获取项目成员列表
// +openapi:summary.workspaces.namespaces.users=获取租户下项目的成员列表
//
// 同 workspaceUserOps.list:?available=true 时返回尚未加入此项目的平台用户。
func (o namespaceUserOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[User], error) {
	nsID := ctx.Scope.NamespaceID

	available := query.Filters["available"] == "true"
	delete(query.Filters, "available")

	if available {
		result, err := o.rbStore.ListNamespaceNonMembers(ctx, nsID, query)
		if err != nil {
			return nil, domainErr(err)
		}
		items := make([]User, len(result.Items))
		for i := range result.Items {
			items[i] = *userToAPI(&result.Items[i])
		}
		return &list.Result[User]{Items: items, TotalCount: result.TotalCount}, nil
	}

	result, err := o.rbStore.ListNamespaceMembers(ctx, nsID, query)
	if err != nil {
		return nil, domainErr(err)
	}

	items := make([]User, len(result.Items))
	for i, m := range result.Items {
		u := userToAPI(&m.UserRow)
		u.Spec.Roles = m.Roles
		u.Spec.JoinedAt = m.JoinedAt.Format(time.RFC3339)
		items[i] = *u
	}

	return &list.Result[User]{Items: items, TotalCount: result.TotalCount}, nil
}

// +openapi:summary=批量添加项目成员
// +openapi:summary.workspaces.namespaces.users=批量添加租户下项目的成员
func (o namespaceUserOps) create(ctx apiserver.Ctx, body json.RawMessage) (any, error) {
	nsID := ctx.Scope.NamespaceID

	req, err := apiserver.DecodePatch[BatchRequest](body)
	if err != nil {
		return nil, err
	}

	// Check max members limit
	ns, err := o.nsStore.GetByID(ctx, nsID)
	if err != nil {
		return nil, domainErr(err)
	}
	if ns.MaxMembers > 0 {
		currentCount, err := o.nsStore.CountUsers(ctx, nsID)
		if err != nil {
			return nil, domainErr(err)
		}
		if currentCount+int64(len(req.IDs)) > int64(ns.MaxMembers) {
			return nil, apierrors.NewBadRequest(
				fmt.Sprintf("namespace member limit exceeded: current %d, adding %d, max %d", currentCount, len(req.IDs), ns.MaxMembers),
				nil,
			)
		}
	}

	var roleID int64
	if req.RoleID != "" {
		roleID, err = parseID(req.RoleID)
		if err != nil {
			return nil, apierrors.NewBadRequest("invalid role ID", nil)
		}
	}

	added, err := batchAddUsers(ctx, req.IDs, o.userStore, func(ctx context.Context, uid int64) (bool, error) {
		if err := o.rbStore.AddNamespaceMember(ctx, uid, nsID, roleID); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return &apiserver.BatchResult{
		TypeMeta:     runtime.TypeMeta{Kind: "Result"},
		SuccessCount: added,
	}, nil
}

// +openapi:summary=批量移除项目成员
// +openapi:summary.workspaces.namespaces.users=批量移除租户下项目的成员
func (o namespaceUserOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	nsID := ctx.Scope.NamespaceID

	successCount, failedIDs, err := batchRemoveUsers(ctx, ids, func(ctx context.Context, uid int64) error {
		return o.rbStore.RemoveNamespaceMember(ctx, uid, nsID)
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return &apiserver.BatchResult{
		SuccessCount: successCount,
		FailedCount:  len(failedIDs),
		FailedIDs:    failedIDs,
	}, nil
}
