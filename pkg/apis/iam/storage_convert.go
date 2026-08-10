package iam

import (
	"strconv"
	"vraxel.io/vraxel/lib/api/types"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/runtime"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// ===== helpers =====

func userToAPI(u *modstore.UserRow) *User {
	createdAt := u.CreatedAt
	updatedAt := u.UpdatedAt
	return &User{
		TypeMeta: runtime.TypeMeta{Kind: "User"},
		ObjectMeta: types.ObjectMeta{
			ID:        strconv.FormatInt(u.ID, 10),
			Name:      u.Username,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		},
		Spec: UserSpec{
			Username:    u.Username,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Phone:       u.Phone,
			AvatarURL:   u.AvatarURL,
			Status:      u.Status,
			Builtin:     u.Builtin,
		},
	}
}

func userWithNamespacesToAPI(u *modstore.UserWithNamespacesRow) *User {
	user := userToAPI(&u.UserRow)
	if len(u.NamespaceNames) > 0 {
		user.Spec.Namespaces = u.NamespaceNames
	}
	return user
}

func workspaceToAPI(w *modstore.WorkspaceRow) *Workspace {
	createdAt := w.CreatedAt
	updatedAt := w.UpdatedAt
	return &Workspace{
		TypeMeta: runtime.TypeMeta{Kind: "Workspace"},
		ObjectMeta: types.ObjectMeta{
			ID:        strconv.FormatInt(w.ID, 10),
			Name:      w.Name,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		},
		Spec: WorkspaceSpec{
			DisplayName: w.DisplayName,
			Description: w.Description,
			OwnerID:     strconv.FormatInt(w.OwnerID, 10),
			Status:      w.Status,
		},
	}
}

func workspaceWithOwnerToAPI(w *modstore.WorkspaceWithOwnerRow) *Workspace {
	ws := workspaceToAPI(&w.WorkspaceRow)
	ws.Spec.OwnerName = w.OwnerUsername
	ws.Spec.CreatedByName = w.CreatorName
	ws.Spec.NamespaceCount = int(w.NamespaceCount)
	ws.Spec.MemberCount = int(w.MemberCount)
	ws.Spec.RoleBindingCount = int(w.RoleBindingCount)
	return ws
}

func namespaceWithOwnerToAPI(n *modstore.NamespaceWithOwnerRow) *Namespace {
	ns := namespaceToAPI(&n.NamespaceRow)
	ns.Spec.OwnerName = n.OwnerUsername
	ns.Spec.WorkspaceName = n.WorkspaceName
	ns.Spec.CreatedByName = n.CreatorName
	ns.Spec.MemberCount = int(n.MemberCount)
	ns.Spec.RoleBindingCount = int(n.RoleBindingCount)
	return ns
}

func namespaceToAPI(n *modstore.NamespaceRow) *Namespace {
	createdAt := n.CreatedAt
	updatedAt := n.UpdatedAt
	return &Namespace{
		TypeMeta: runtime.TypeMeta{Kind: "Namespace"},
		ObjectMeta: types.ObjectMeta{
			ID:        strconv.FormatInt(n.ID, 10),
			Name:      n.Name,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		},
		Spec: NamespaceSpec{
			DisplayName: n.DisplayName,
			Description: n.Description,
			WorkspaceID: strconv.FormatInt(n.WorkspaceID, 10),
			OwnerID:     strconv.FormatInt(n.OwnerID, 10),
			Visibility:  n.Visibility,
			MaxMembers:  int(n.MaxMembers),
			Status:      n.Status,
		},
	}
}
func roleBindingToAPI(rb *modstore.RoleBindingRow, username, userDisplayName, roleName, roleDisplayName string) *RoleBinding {
	createdAt := rb.CreatedAt
	var wsID, nsID *string
	if rb.WorkspaceID != nil {
		s := strconv.FormatInt(*rb.WorkspaceID, 10)
		wsID = &s
	}
	if rb.NamespaceID != nil {
		s := strconv.FormatInt(*rb.NamespaceID, 10)
		nsID = &s
	}
	return &RoleBinding{
		TypeMeta: runtime.TypeMeta{Kind: "RoleBinding"},
		ObjectMeta: types.ObjectMeta{
			ID:        strconv.FormatInt(rb.ID, 10),
			CreatedAt: &createdAt,
		},
		Spec: RoleBindingSpec{
			UserID:          strconv.FormatInt(rb.UserID, 10),
			RoleID:          strconv.FormatInt(rb.RoleID, 10),
			Scope:           rb.Scope,
			WorkspaceID:     wsID,
			NamespaceID:     nsID,
			IsOwner:         rb.IsOwner,
			RoleName:        roleName,
			RoleDisplayName: roleDisplayName,
			Username:        username,
			UserDisplayName: userDisplayName,
		},
	}
}

func roleBindingWithDetailsToAPI(rb *modstore.RoleBindingWithDetailsRow) *RoleBinding {
	result := roleBindingToAPI(&rb.RoleBindingRow, rb.Username, rb.UserDisplayName, rb.RoleName, rb.RoleDisplayName)
	result.Spec.WorkspaceName = rb.WorkspaceName
	result.Spec.NamespaceName = rb.NamespaceName
	return result
}

// roleBindingListResult converts a store rolebinding page into the typed
// list result; the framework folds it into the RoleBindingList envelope.
func roleBindingListResult(result *list.Result[modstore.RoleBindingWithDetailsRow]) *list.Result[RoleBinding] {
	items := make([]RoleBinding, len(result.Items))
	for i, item := range result.Items {
		items[i] = *roleBindingWithDetailsToAPI(&item)
	}
	return &list.Result[RoleBinding]{
		Items:      items,
		TotalCount: result.TotalCount,
	}
}

func permissionToAPI(p *modstore.PermissionRow) *Permission {
	createdAt := p.CreatedAt
	return &Permission{
		TypeMeta: runtime.TypeMeta{Kind: "Permission"},
		ObjectMeta: types.ObjectMeta{
			ID:        strconv.FormatInt(p.ID, 10),
			CreatedAt: &createdAt,
		},
		Spec: PermissionSpec{
			Code:        p.Code,
			Method:      p.Method,
			Path:        p.Path,
			Scope:       p.Scope,
			Description: p.Description,
		},
	}
}
