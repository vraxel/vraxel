package iam

import (
	"strconv"
	"time"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/runtime"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// ===== users:workspaces 用户租户视图 =====

// NewUserWorkspacesVerb 创建用户租户视图，支持分页、筛选和排序。
// 注册为 GET /users/{userId}/workspaces
// +openapi:customverb=workspaces
// +openapi:resource=User
// +openapi:summary=获取用户关联的租户列表
func NewUserWorkspacesVerb(rbStore modstore.RoleBindingStore) func(apiserver.Ctx, int64, list.Query) (*list.Result[Workspace], error) {
	return func(ctx apiserver.Ctx, uid int64, query list.Query) (*list.Result[Workspace], error) {
		result, err := rbStore.ListUserWorkspaces(ctx, uid, query)
		if err != nil {
			return nil, err
		}

		items := make([]Workspace, len(result.Items))
		for i, item := range result.Items {
			ws := workspaceWithOwnerToAPI(&modstore.WorkspaceWithOwnerRow{
				WorkspaceRow:   item.WorkspaceRow,
				OwnerUsername:  item.OwnerUsername,
				NamespaceCount: item.NamespaceCount,
				MemberCount:    item.MemberCount,
			})
			ws.Spec.Role = item.Role
			ws.Spec.RoleDisplayName = item.RoleDisplayName
			ws.Spec.JoinedAt = item.JoinedAt.Format(time.RFC3339)
			items[i] = *ws
		}

		return &list.Result[Workspace]{Items: items, TotalCount: result.TotalCount}, nil
	}
}

// ===== users:namespaces 用户项目视图 =====

// NewUserNamespacesVerb 创建用户项目视图，支持分页、筛选和排序。
// 注册为 GET /users/{userId}/namespaces
// +openapi:customverb=namespaces
// +openapi:resource=User
// +openapi:summary=获取用户关联的项目列表
func NewUserNamespacesVerb(rbStore modstore.RoleBindingStore) func(apiserver.Ctx, int64, list.Query) (*list.Result[Namespace], error) {
	return func(ctx apiserver.Ctx, uid int64, query list.Query) (*list.Result[Namespace], error) {
		result, err := rbStore.ListUserNamespaces(ctx, uid, query)
		if err != nil {
			return nil, err
		}

		items := make([]Namespace, len(result.Items))
		for i, item := range result.Items {
			ns := namespaceWithOwnerToAPI(&modstore.NamespaceWithOwnerRow{
				NamespaceRow:  item.NamespaceRow,
				OwnerUsername: item.OwnerUsername,
				WorkspaceName: item.WorkspaceName,
				MemberCount:   item.MemberCount,
			})
			ns.Spec.Role = item.Role
			ns.Spec.RoleDisplayName = item.RoleDisplayName
			ns.Spec.JoinedAt = item.JoinedAt.Format(time.RFC3339)
			items[i] = *ns
		}

		return &list.Result[Namespace]{Items: items, TotalCount: result.TotalCount}, nil
	}
}

// ===== users:rolebindings 用户角色绑定视图 =====

// NewUserRoleBindingsVerb 创建用户角色绑定视图。
// 注册为 GET /users/{userId}/rolebindings
// +openapi:customverb=rolebindings
// +openapi:resource=User
// +openapi:response=RoleBindingList
// +openapi:summary=获取用户的角色绑定列表
func NewUserRoleBindingsVerb(rbStore modstore.RoleBindingStore) func(apiserver.Ctx, int64, list.Query) (*list.Result[RoleBinding], error) {
	return func(ctx apiserver.Ctx, uid int64, query list.Query) (*list.Result[RoleBinding], error) {
		result, err := rbStore.ListByUserID(ctx, uid, query)
		if err != nil {
			return nil, err
		}

		return roleBindingListResult(result), nil
	}
}

// ===== 作用域用户角色绑定视图 =====
//
// Scoped twins of the platform rolebindings verb so a workspace /
// namespace member with iam:users:get inside that scope can see role
// bindings of another user filtered to the same scope (bug #134:
// tangzz had project-admin + workspace test-role but the global
// rolebindings verb required platform iam:users:get, so the user
// detail page hit 403 inside their own workspace).
//
// workspace_id / namespace_id come from the scope path segments and
// override any query-supplied value so a workspace member cannot
// exfiltrate another scope's bindings by passing ?workspace_id=other
// in the URL.

func NewWorkspaceUserRoleBindingsVerb(rbStore modstore.RoleBindingStore) func(apiserver.Ctx, int64, list.Query) (*list.Result[RoleBinding], error) {
	return func(ctx apiserver.Ctx, uid int64, query list.Query) (*list.Result[RoleBinding], error) {
		query.Filters["workspace_id"] = ctx.Scope.WorkspaceID
		result, err := rbStore.ListByUserID(ctx, uid, query)
		if err != nil {
			return nil, err
		}
		return roleBindingListResult(result), nil
	}
}

func NewNamespaceUserRoleBindingsVerb(rbStore modstore.RoleBindingStore) func(apiserver.Ctx, int64, list.Query) (*list.Result[RoleBinding], error) {
	return func(ctx apiserver.Ctx, uid int64, query list.Query) (*list.Result[RoleBinding], error) {
		query.Filters["namespace_id"] = ctx.Scope.NamespaceID
		result, err := rbStore.ListByUserID(ctx, uid, query)
		if err != nil {
			return nil, err
		}
		return roleBindingListResult(result), nil
	}
}

// ===== users:permissions 用户权限视图 =====

// NewUserPermissionsVerb 创建用户权限聚合视图。响应是单个 UserPermissions
// 对象而非标准 list envelope，故走 VerbAny。
// 注册为 GET /users/{userId}/permissions
// +openapi:customverb=permissions
// +openapi:resource=User
// +openapi:response=UserPermissions
// +openapi:summary=获取用户的权限视图
func NewUserPermissionsVerb(rbStore modstore.RoleBindingStore, permStore modstore.PermissionStore) func(apiserver.Ctx, int64, list.Query) (any, error) {
	return func(ctx apiserver.Ctx, uid int64, query list.Query) (any, error) {
		// 1. Get all role bindings with rules for this user
		rows, err := rbStore.GetUserRoleBindingsWithRules(ctx, uid)
		if err != nil {
			return nil, err
		}

		// 2. Get all registered permission codes for pattern expansion
		allCodes, err := permStore.ListAllCodes(ctx)
		if err != nil {
			return nil, err
		}

		// 3. Group by scope and collect patterns + role names
		grouped := userPermsGroupByScope(rows)

		// 4. Expand patterns to concrete permission codes
		spec := userPermsBuildSpec(grouped, allCodes)

		return &UserPermissions{
			TypeMeta: runtime.TypeMeta{Kind: "UserPermissions"},
			Spec:     spec,
		}, nil
	}
}

// userPermsWsEntry 聚合单个 workspace 作用域下的权限模式与角色名集合（步骤 3 的中间态）。
type userPermsWsEntry struct {
	patterns  []string
	roleNames map[string]bool
}

// userPermsNsEntry 聚合单个 namespace 作用域下的权限模式、角色名集合与所属 workspace（步骤 3 的中间态）。
type userPermsNsEntry struct {
	patterns    []string
	roleNames   map[string]bool
	workspaceID string
}

// userPermsGrouped 是按作用域分组后的聚合结果（步骤 3 的输出）。
type userPermsGrouped struct {
	platformPatterns []string
	isPlatformAdmin  bool
	wsMap            map[string]*userPermsWsEntry
	nsMap            map[string]*userPermsNsEntry
}

// userPermsGroupByScope 实现步骤 3：遍历角色绑定行，按 platform / workspace / namespace
// 作用域分组收集权限模式与角色名。platform 下出现 "*:*" 即标记为平台管理员。每个分支逐字保留原逻辑。
func userPermsGroupByScope(rows []modstore.UserRoleBindingWithRules) userPermsGrouped {
	var platformPatterns []string
	platformRoleNames := make(map[string]bool)
	isPlatformAdmin := false

	wsMap := make(map[string]*userPermsWsEntry)
	nsMap := make(map[string]*userPermsNsEntry)

	for _, row := range rows {
		switch row.Scope {
		case modstore.ScopePlatform:
			platformPatterns = append(platformPatterns, row.Pattern)
			platformRoleNames[row.RoleName] = true
			if row.Pattern == "*:*" {
				isPlatformAdmin = true
			}
		case modstore.ScopeWorkspace:
			userPermsCollectWorkspace(wsMap, row)
		case modstore.ScopeNamespace:
			userPermsCollectNamespace(nsMap, row)
		}
	}

	return userPermsGrouped{
		platformPatterns: platformPatterns,
		isPlatformAdmin:  isPlatformAdmin,
		wsMap:            wsMap,
		nsMap:            nsMap,
	}
}

// userPermsCollectWorkspace 将一行 workspace 作用域绑定累加进 wsMap（WorkspaceID 为 nil 时跳过）。
func userPermsCollectWorkspace(wsMap map[string]*userPermsWsEntry, row modstore.UserRoleBindingWithRules) {
	if row.WorkspaceID != nil {
		wsIDStr := strconv.FormatInt(*row.WorkspaceID, 10)
		entry, ok := wsMap[wsIDStr]
		if !ok {
			entry = &userPermsWsEntry{roleNames: make(map[string]bool)}
			wsMap[wsIDStr] = entry
		}
		entry.patterns = append(entry.patterns, row.Pattern)
		entry.roleNames[row.RoleName] = true
	}
}

// userPermsCollectNamespace 将一行 namespace 作用域绑定累加进 nsMap（NamespaceID 为 nil 时跳过），
// 首次见到该 namespace 时记录其所属 workspace。
func userPermsCollectNamespace(nsMap map[string]*userPermsNsEntry, row modstore.UserRoleBindingWithRules) {
	if row.NamespaceID != nil {
		nsIDStr := strconv.FormatInt(*row.NamespaceID, 10)
		entry, ok := nsMap[nsIDStr]
		if !ok {
			var wsIDStr string
			if row.WorkspaceID != nil {
				wsIDStr = strconv.FormatInt(*row.WorkspaceID, 10)
			}
			entry = &userPermsNsEntry{roleNames: make(map[string]bool), workspaceID: wsIDStr}
			nsMap[nsIDStr] = entry
		}
		entry.patterns = append(entry.patterns, row.Pattern)
		entry.roleNames[row.RoleName] = true
	}
}

// userPermsBuildSpec 实现步骤 4：把分组得到的权限模式展开成具体权限码，构造 UserPermissionsSpec。
func userPermsBuildSpec(grouped userPermsGrouped, allCodes []string) UserPermissionsSpec {
	spec := UserPermissionsSpec{
		IsPlatformAdmin: grouped.isPlatformAdmin,
		Platform:        ExpandPatterns(grouped.platformPatterns, allCodes),
		Workspaces:      make(map[string]WorkspaceScopePerms),
		Namespaces:      make(map[string]NamespaceScopePerms),
	}

	for wsID, entry := range grouped.wsMap {
		spec.Workspaces[wsID] = WorkspaceScopePerms{
			RoleNames:   mapKeys(entry.roleNames),
			Permissions: ExpandPatterns(entry.patterns, allCodes),
		}
	}

	for nsID, entry := range grouped.nsMap {
		spec.Namespaces[nsID] = NamespaceScopePerms{
			RoleNames:   mapKeys(entry.roleNames),
			WorkspaceID: entry.workspaceID,
			Permissions: ExpandPatterns(entry.patterns, allCodes),
		}
	}

	return spec
}
