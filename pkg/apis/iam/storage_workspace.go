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

// ===== workspaceOps 租户存储 =====

// workspaceOps 租户资源的典型 Ops 实现。创建租户时自动创建默认项目并添加所有者为成员。
type workspaceOps struct {
	wsStore   modstore.WorkspaceStore
	userStore modstore.UserStore
	rbStore   modstore.RoleBindingStore
}

// WorkspacesDef declares the platform workspaces resource. The platform
// list injects the binding-scoped AccessFilter for non-admins
// (ListAccessScope); permission records fan out from workspace scope
// (PermScope) because the item URL literally IS the scope segment v1's
// URL-based derivation keyed on.
func WorkspacesDef(wsStore modstore.WorkspaceStore, userStore modstore.UserStore, rbStore modstore.RoleBindingStore) apiserver.ResourceDef[Workspace] {
	o := workspaceOps{wsStore: wsStore, userStore: userStore, rbStore: rbStore}
	return apiserver.ResourceDef[Workspace]{
		Group: "iam", Name: "workspaces",
		Ops: apiserver.Ops[Workspace]{
			List:        o.list,
			Get:         o.get,
			Create:      o.create,
			Update:      o.update,
			Patch:       o.patch,
			Delete:      o.delete,
			BatchDelete: o.batchDelete,
		},
		ListAccessScope: "workspaces",
		PermScope:       apiserver.ScopeWorkspace,
		// Flat /workspaces/{id} (get/update/delete) authorizes at the
		// workspace's own scope (id = the workspace), not platform, so a
		// workspace-admin's workspace-scoped binding matches instead of 403.
		ItemScopeResolver: func(_ context.Context, id int64) apiserver.ScopeInfo {
			return apiserver.ScopeInfo{Level: apiserver.ScopeWorkspace, WorkspaceID: id}
		},
	}
}

// +openapi:summary=获取租户详情
func (o workspaceOps) get(ctx apiserver.Ctx, id int64) (*Workspace, error) {
	ws, err := o.wsStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}
	return workspaceWithOwnerToAPI(ws), nil
}

// +openapi:summary=获取租户列表
func (o workspaceOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[Workspace], error) {
	// Inject access filter for non-admin users
	if af := filters.AccessFilterFromContext(ctx); af != nil && af.WorkspaceIDs != nil {
		query.Filters["accessible_ids"] = af.WorkspaceIDs
	}

	result, err := o.wsStore.List(ctx, query)
	if err != nil {
		return nil, domainErr(err)
	}

	items := make([]Workspace, len(result.Items))
	for i, item := range result.Items {
		items[i] = *workspaceWithOwnerToAPI(&item)
	}

	return &list.Result[Workspace]{Items: items, TotalCount: result.TotalCount}, nil
}

// +openapi:summary=创建租户
func (o workspaceOps) create(ctx apiserver.Ctx, ws *Workspace) (*Workspace, error) {
	// Auto-inject ownerId from authenticated user
	userID, ok := oidc.UserIDFromContext(ctx)
	if ok {
		ws.Spec.OwnerID = strconv.FormatInt(userID, 10)
	}

	if errs := ValidateWorkspaceCreate(ws.ObjectMeta.Name, &ws.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	if ctx.DryRun {
		return ws, nil
	}

	ownerID, err := parseID(ws.Spec.OwnerID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid ownerId: %s", ws.Spec.OwnerID), nil)
	}

	// Check owner exists
	if _, err := o.userStore.GetByID(ctx, ownerID); err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("owner user %d not found", ownerID), nil)
	}

	status := ws.Spec.Status
	if status == "" {
		status = "active"
	}

	created, err := o.wsStore.Create(ctx, modstore.WorkspaceCreateInput{
		Name:        ws.ObjectMeta.Name,
		DisplayName: ws.Spec.DisplayName,
		Description: ws.Spec.Description,
		OwnerID:     ownerID,
		Status:      status,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return workspaceWithOwnerToAPI(created), nil
}

// +openapi:summary=更新租户信息（全量）
func (o workspaceOps) update(ctx apiserver.Ctx, id int64, ws *Workspace) (*Workspace, error) {
	if errs := ValidateWorkspaceUpdate(&ws.Spec); errs.HasErrors() {
		return nil, apierrors.NewBadRequest("validation failed", errs)
	}

	if ctx.DryRun {
		return ws, nil
	}

	ownerID, err := parseID(ws.Spec.OwnerID)
	if err != nil {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid ownerId: %s", ws.Spec.OwnerID), nil)
	}

	updated, err := o.wsStore.Update(ctx, modstore.WorkspaceUpdateInput{
		ID:          id,
		Name:        ws.ObjectMeta.Name,
		DisplayName: ws.Spec.DisplayName,
		Description: ws.Spec.Description,
		OwnerID:     ownerID,
		Status:      ws.Spec.Status,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return workspaceWithOwnerToAPI(updated), nil
}

// +openapi:summary=更新租户信息（部分）
func (o workspaceOps) patch(ctx apiserver.Ctx, id int64, body json.RawMessage) (*Workspace, error) {
	ws, err := apiserver.DecodePatch[Workspace](body)
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
	if ws.Spec.OwnerID != "" {
		ownerID, err = parseID(ws.Spec.OwnerID)
		if err != nil {
			return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid ownerId: %s", ws.Spec.OwnerID), nil)
		}
	}

	patched, err := o.wsStore.Patch(ctx, modstore.WorkspaceUpdateInput{
		ID:          id,
		Name:        ws.ObjectMeta.Name,
		DisplayName: ws.Spec.DisplayName,
		Description: ws.Spec.Description,
		OwnerID:     ownerID,
		Status:      ws.Spec.Status,
	})
	if err != nil {
		return nil, domainErr(err)
	}

	return workspaceWithOwnerToAPI(patched), nil
}

// +openapi:summary=删除租户
func (o workspaceOps) delete(ctx apiserver.Ctx, id int64) error {
	if ctx.DryRun {
		return nil
	}

	rows, err := o.wsStore.ListBlockingResources(ctx, id)
	if err != nil {
		return domainErr(err)
	}
	if blockErr := blockingResourceError("workspace", rows); blockErr != nil {
		return blockErr
	}

	if err := o.wsStore.Delete(ctx, id); err != nil {
		return domainErr(err)
	}
	return nil
}

// +openapi:summary=批量删除租户
func (o workspaceOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	if ctx.DryRun {
		return &apiserver.BatchResult{
			SuccessCount: len(ids),
			FailedCount:  0,
		}, nil
	}

	for _, wid := range ids {
		rows, err := o.wsStore.ListBlockingResources(ctx, wid)
		if err != nil {
			return nil, domainErr(err)
		}
		if blockErr := blockingResourceError("workspace", rows); blockErr != nil {
			return nil, blockErr
		}
	}

	count, err := o.wsStore.DeleteByIDs(ctx, ids)
	if err != nil {
		return nil, domainErr(err)
	}

	return &apiserver.BatchResult{
		SuccessCount: int(count),
		FailedCount:  len(ids) - int(count),
	}, nil
}

// ===== workspaceUserOps 租户成员存储 =====

// workspaceUserOps 管理租户的成员关系，支持查询成员列表、批量添加和批量移除成员。
type workspaceUserOps struct {
	rbStore   modstore.RoleBindingStore
	userStore modstore.UserStore
}

// WorkspaceUsersDef declares the workspace-scope users (member) surface.
// Create is asymmetric — the body is a BatchRequest ({ids, roleId}) and
// the response a Result envelope, not User→User — hence CreateAny.
func WorkspaceUsersDef(rbStore modstore.RoleBindingStore, userStore modstore.UserStore) apiserver.ResourceDef[User] {
	o := workspaceUserOps{rbStore: rbStore, userStore: userStore}
	return apiserver.ResourceDef[User]{
		Group: "iam", Name: "users",
		Scopes: apiserver.ScopeWorkspace,
		Ops: apiserver.Ops[User]{
			List:        o.list,
			Get:         o.get,
			CreateAny:   o.create,
			BatchDelete: o.batchDelete,
		},
		Verbs: []apiserver.VerbDef{
			apiserver.Verb("rolebindings", NewWorkspaceUserRoleBindingsVerb(rbStore)),
		},
	}
}

// +openapi:summary=获取租户成员详情
func (o workspaceUserOps) get(ctx apiserver.Ctx, id int64) (*User, error) {
	user, err := o.userStore.GetByID(ctx, id)
	if err != nil {
		return nil, domainErr(err)
	}

	return userToAPI(user), nil
}

// +openapi:summary=获取租户成员列表
//
// 默认返回当前租户的成员;当查询参数 ?available=true 时,返回尚未加入此租户
// 的平台用户(供"添加成员"对话框选择)。两种模式共用同一权限码 iam:users:list,
// 调用方只需具备 workspace scope 权限即可,而不需要 platform 级 iam:users:list。
func (o workspaceUserOps) list(ctx apiserver.Ctx, query list.Query) (*list.Result[User], error) {
	wsID := ctx.Scope.WorkspaceID

	available := query.Filters["available"] == "true"
	delete(query.Filters, "available")

	if available {
		result, err := o.rbStore.ListWorkspaceNonMembers(ctx, wsID, query)
		if err != nil {
			return nil, domainErr(err)
		}
		items := make([]User, len(result.Items))
		for i := range result.Items {
			items[i] = *userToAPI(&result.Items[i])
		}
		return &list.Result[User]{Items: items, TotalCount: result.TotalCount}, nil
	}

	result, err := o.rbStore.ListWorkspaceMembers(ctx, wsID, query)
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

// +openapi:summary=批量添加租户成员
func (o workspaceUserOps) create(ctx apiserver.Ctx, body json.RawMessage) (any, error) {
	wsID := ctx.Scope.WorkspaceID

	req, err := apiserver.DecodePatch[BatchRequest](body)
	if err != nil {
		return nil, err
	}

	var roleID int64
	if req.RoleID != "" {
		roleID, err = parseID(req.RoleID)
		if err != nil {
			return nil, apierrors.NewBadRequest("invalid role ID", nil)
		}
	}

	added, err := batchAddUsers(ctx, req.IDs, o.userStore, func(ctx context.Context, uid int64) (bool, error) {
		if err := o.rbStore.AddWorkspaceMember(ctx, uid, wsID, roleID); err != nil {
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

// +openapi:summary=批量移除租户成员
func (o workspaceUserOps) batchDelete(ctx apiserver.Ctx, ids []int64) (*apiserver.BatchResult, error) {
	wsID := ctx.Scope.WorkspaceID

	successCount, failedIDs, err := batchRemoveUsers(ctx, ids, func(ctx context.Context, uid int64) error {
		return o.rbStore.RemoveWorkspaceMember(ctx, uid, wsID)
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
