import type { Messages } from "../../types"

const iam = {
  // workspace
  "workspace.title": "Workspaces",
  "workspace.manage": "Manage your workspaces. {count} total.",
  "workspace.create": "Create Workspace",
  "workspace.edit": "Edit Workspace",
  "workspace.notFound": "Workspace not found.",
  "workspace.noData": "No workspaces found.",
  "workspace.details": "Details",
  "workspace.namespaces": "Namespaces",
  "workspace.members": "Users",
  "workspace.membersManage": "Manage workspace users. {count} total.",
  "workspace.owner": "Owner",
  "workspace.namespaceCount": "Namespaces",
  "workspace.memberCount": "Users",
  "workspace.searchPlaceholder": "Search name, display name, description...",
  "workspace.deleteConfirm":
    'Are you sure you want to delete workspace "{name}"? This action cannot be undone.',
  "workspace.batchDelete": "Batch Delete",
  "workspace.batchDeleteConfirm":
    "Are you sure you want to delete {count} selected workspaces? This action cannot be undone.",
  "workspace.addMember": "Add User",
  "workspace.addMemberDesc": "Select users to add to this workspace.",
  "workspace.removeMember": "Remove User",
  "workspace.removeMemberConfirm":
    'Are you sure you want to remove user "{name}" from this workspace?',
  "workspace.batchRemoveMemberConfirm":
    "Are you sure you want to remove {count} selected users from this workspace?",
  "workspace.memberAdded": "Users added successfully",
  "workspace.memberRemoved": "Users removed successfully",
  "workspace.memberPartialRemoved":
    "Removed {success} users, {failed} failed (owner cannot be removed)",
  "workspace.noMembers": "No users found.",
  "workspace.noAvailableUsers": "No users available to add.",
  "workspace.validation.name.format": "Name must be 3-50 letters, digits, hyphens, or underscores",
  "workspace.validation.name.taken": "This name is already taken",
  "workspace.validation.name.hint": "3-50 lowercase letters, digits, or hyphens, e.g. my-workspace",

  // namespace
  "namespace.title": "Namespaces",
  "namespace.manage": "Manage namespaces. {count} total.",
  "namespace.create": "Create Namespace",
  "namespace.edit": "Edit Namespace",
  "namespace.notFound": "Namespace not found.",
  "namespace.noData": "No namespaces found.",
  "namespace.workspaceName": "Workspace",
  "namespace.selectWorkspace": "Select a workspace",
  "namespace.visibility": "Visibility",
  "namespace.visibility.public": "Public",
  "namespace.visibility.private": "Private",
  "namespace.owner": "Owner",
  "namespace.details": "Details",
  "namespace.members": "Users",
  "namespace.membersManage": "Manage namespace users. {count} total.",
  "namespace.memberCount": "Users",
  "namespace.maxMembers": "Max Users",
  "namespace.maxMembersHint": "0 means unlimited",
  "namespace.validation.maxMembers": "Must be an integer between 0 and 1000000",
  "namespace.searchPlaceholder": "Search name, display name, description...",
  "namespace.deleteConfirm":
    'Are you sure you want to delete namespace "{name}"? This action cannot be undone.',
  "namespace.batchDelete": "Batch Delete",
  "namespace.batchDeleteConfirm":
    "Are you sure you want to delete {count} selected namespaces? This action cannot be undone.",
  "namespace.addMember": "Add User",
  "namespace.addMemberDesc": "Select users to add to this namespace.",
  "namespace.removeMember": "Remove User",
  "namespace.removeMemberConfirm":
    'Are you sure you want to remove user "{name}" from this namespace?',
  "namespace.batchRemoveMemberConfirm":
    "Are you sure you want to remove {count} selected users from this namespace?",
  "namespace.memberAdded": "Users added successfully",
  "namespace.memberRemoved": "Users removed successfully",
  "namespace.memberPartialRemoved":
    "Removed {success} users, {failed} failed (owner cannot be removed)",
  "namespace.noMembers": "No users found.",
  "namespace.noAvailableUsers": "No users available to add.",
  "namespace.validation.name.format":
    "Name must be 3-50 letters, digits, hyphens, or underscores, starting and ending with a letter or digit",
  "namespace.validation.name.taken": "This name is already taken",
  "namespace.validation.name.hint":
    "3-50 letters, digits, hyphens, or underscores, starting and ending with a letter or digit, e.g. my-project",
  "namespace.builtinDefault": "Default",
  "namespace.builtinDefaultDesc": "Default namespace for workspace {name}",

  // user
  "user.title": "Users",
  "user.manage": "Manage platform users. {count} total.",
  "user.create": "Create User",
  "user.edit": "Edit User",
  "user.noData": "No users found.",
  "user.notFound": "User not found.",
  "user.details": "User Details",
  "user.username": "Username",
  "user.email": "Email",
  "user.searchPlaceholder": "Search username, email, phone, display name...",
  "user.deleteConfirm":
    'Are you sure you want to delete user "{name}"? This action cannot be undone.',
  "user.resetPassword": "Reset Password",
  "user.resetPasswordTitle": 'Reset password for "{name}"',
  "user.resetPasswordHint":
    "Set a new password for this user. The user's existing sessions will be revoked and they must log in again with the new password.",
  "user.batchDelete": "Batch Delete",
  "user.batchDeleteConfirm":
    "Are you sure you want to delete {count} selected users? This action cannot be undone.",
  "user.workspaces": "Joined Workspaces",
  "user.namespaceRefs": "Joined Namespaces",
  "user.noWorkspaces": "Not joined any workspace yet.",
  "user.noNamespaceRefs": "Not joined any namespace yet.",
  "user.rolebindings": "Role Bindings",
  "user.noRolebindings": "No role bindings.",
  "user.role": "Role",
  "user.joinedAt": "Joined At",

  // role management
  "role.title": "Roles",
  "role.manage": "Manage roles. {count} total.",
  "role.create": "Create Role",
  "role.edit": "Edit Role",
  "role.noData": "No roles found.",
  "role.notFound": "Role not found.",
  "role.details": "Role Details",
  "role.users": "Users with this role",
  "role.assignUsers": "Assign Users",
  "role.assignUsersHint": "Grant this role to the selected users.",
  "role.noUsers": "No users have this role yet.",
  "role.name": "Name",
  "role.scope": "Scope",
  "role.builtin": "Built-in",
  "role.custom": "Custom",
  "role.rules": "Permission Rules",
  "role.rulesCount": "{count} rules",
  "role.searchPlaceholder": "Search name, display name, description...",
  "role.deleteConfirm":
    'Are you sure you want to delete role "{name}"? This action cannot be undone.',
  "role.batchDelete": "Batch Delete",
  "role.batchDeleteConfirm":
    "Are you sure you want to delete {count} selected roles? This action cannot be undone.",
  "role.builtinCannotEdit": "Built-in roles cannot be edited",
  "role.builtinCannotDelete": "Built-in roles cannot be deleted",
  "role.selectPermissions": "Select Permissions",
  "role.noPermissions": "No permissions selected",
  "role.matchedPermissions": "Matched {count} permissions",
  "role.matchCount": "{count} matched",
  "role.permissionCode": "Permission Code",
  "role.permissionDescription": "Description",
  "role.scope.platform": "Platform",
  "role.scope.workspace": "Workspace",
  "role.scope.namespace": "Namespace",
  "role.validation.name.format": "Name must be 3-50 letters, digits, hyphens, or underscores",
  "role.validation.name.taken": "This role name is already taken",
  "role.validation.name.hint": "3-50 lowercase letters, digits, or hyphens, e.g. my-role",
  "role.validation.rules.required": "At least one permission rule is required",

  // role binding management
  "rolebinding.title": "Role Bindings",
  "rolebinding.manage": "Manage role bindings. {count} total.",
  "rolebinding.create": "Create Role Binding",
  "rolebinding.revoke": "Revoke",
  "rolebinding.revokeConfirm": 'Revoke this role from "{name}"?',
  "rolebinding.ownerLocked": "The owner role cannot be revoked",
  "rolebinding.createN": "Create Bindings ({count} users)",
  "rolebinding.createPartial": "Bound {created} users; {skipped} already had this role.",
  "rolebinding.createDesc": "Select a user and a role to create a role binding.",
  "rolebinding.selectRole": "Select Role",
  "rolebinding.selectUser": "Select User",
  "rolebinding.noRoles": "No roles available.",
  "rolebinding.noUsers": "No users available.",
  "rolebinding.noData": "No role bindings found.",
  "rolebinding.deleteConfirm":
    'Are you sure you want to delete the role binding for user "{name}"?',
  "rolebinding.batchDelete": "Batch Delete",
  "rolebinding.batchDeleteConfirm":
    "Are you sure you want to delete {count} selected role bindings?",
  "rolebinding.scope": "Scope",
  "rolebinding.scope.platform": "Platform",
  "rolebinding.scope.workspace": "Workspace",
  "rolebinding.scope.namespace": "Namespace",
  "rolebinding.scopeTarget": "Scope Target",
  "rolebinding.role": "Role",
  "rolebinding.roleDisplayName": "Role Display Name",
  "rolebinding.searchPlaceholder": "Search username, role name...",

  // built-in role display names
  "role.platform-admin": "Platform Admin",
  "role.platform-viewer": "Platform Viewer",
  "role.workspace-admin": "Workspace Admin",
  "role.workspace-viewer": "Workspace Viewer",
  "role.workspace-member": "Workspace Member",
  "role.namespace-admin": "Namespace Admin",
  "role.namespace-viewer": "Namespace Viewer",

  // built-in role descriptions
  "role.desc.platform-admin": "Full access to all platform resources",
  "role.desc.platform-viewer": "Read-only access to all platform resources",
  "role.desc.workspace-admin": "Full access to all resources within the workspace",
  "role.desc.workspace-viewer": "Read-only access to all resources within the workspace",
  "role.desc.workspace-member": "Basic workspace membership for namespace-level users",
  "role.desc.namespace-admin": "Full access to all resources within the namespace",
  "role.desc.namespace-viewer": "Read-only access to all resources within the namespace",

  // permission selector groups - IAM
  "perm.group.iam": "IAM Management",
  "perm.group.iam.users": "Users",
  "perm.group.iam.workspaces": "Workspaces",
  "perm.group.iam.namespaces": "Namespaces",
  "perm.group.iam.roles": "Roles",
  "perm.group.iam.rolebindings": "Role Bindings",
  "perm.group.iam.permissions": "Permissions",

  // permission codes - users
  "perm.iam:users:list": "List users",
  "perm.iam:users:get": "Get user details",
  "perm.iam:users:create": "Create user",
  "perm.iam:users:update": "Update user",
  "perm.iam:users:patch": "Patch user",
  "perm.iam:users:delete": "Delete user",
  "perm.iam:users:deleteCollection": "Batch delete users",
  "perm.iam:users:change-password": "Change user password",

  // permission codes - workspaces
  "perm.iam:workspaces:list": "List workspaces",
  "perm.iam:workspaces:get": "Get workspace details",
  "perm.iam:workspaces:create": "Create workspace",
  "perm.iam:workspaces:update": "Update workspace",
  "perm.iam:workspaces:patch": "Patch workspace",
  "perm.iam:workspaces:delete": "Delete workspace",
  "perm.iam:workspaces:deleteCollection": "Batch delete workspaces",

  // permission codes - namespaces
  "perm.iam:namespaces:list": "List namespaces",
  "perm.iam:namespaces:get": "Get namespace details",
  "perm.iam:namespaces:create": "Create namespace",
  "perm.iam:namespaces:update": "Update namespace",
  "perm.iam:namespaces:patch": "Patch namespace",
  "perm.iam:namespaces:delete": "Delete namespace",
  "perm.iam:namespaces:deleteCollection": "Batch delete namespaces",

  // permission codes - roles
  "perm.iam:roles:list": "List roles",
  "perm.iam:roles:get": "Get role details",
  "perm.iam:roles:create": "Create role",
  "perm.iam:roles:update": "Update role",
  "perm.iam:roles:delete": "Delete role",

  // permission codes - role bindings
  "perm.iam:rolebindings:list": "List role bindings",
  "perm.iam:rolebindings:create": "Create role binding",
  "perm.iam:rolebindings:delete": "Delete role binding",
  "perm.iam:rolebindings:deleteCollection": "Batch delete role bindings",

  // permission codes - permissions
  "perm.iam:permissions:list": "List permissions",

  // additional missing keys
  "perm.iam:roles:deleteCollection": "Batch delete roles",
  "perm.iam:users:reset-password": "Reset user password",
} satisfies Messages

export default iam
