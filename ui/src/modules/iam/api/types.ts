import type { TypeMeta, ObjectMeta } from "@/core/api/types"

// --- User ---

export interface UserSpec {
  username: string
  email: string
  displayName?: string
  phone?: string
  avatarUrl?: string
  status?: "active" | "inactive"
  // builtin marks system accounts (admin) that cannot be deleted.
  // Read-only; set by backend seed/migration.
  builtin?: boolean
  namespaces?: string[]
  roles?: string[]
  joinedAt?: string
}

export interface User extends TypeMeta {
  metadata: ObjectMeta
  spec: UserSpec
}

export interface UserList extends TypeMeta {
  items: User[]
  totalCount: number
}

// --- Workspace ---

export interface WorkspaceSpec {
  displayName?: string
  description?: string
  ownerId: string
  ownerName?: string
  createdByName?: string
  namespaceCount?: number
  memberCount?: number
  roleBindingCount?: number
  status?: "active" | "inactive"
  roles?: string[]
  roleDisplayNames?: string[]
  joinedAt?: string
}

export interface Workspace extends TypeMeta {
  metadata: ObjectMeta
  spec: WorkspaceSpec
}

export interface WorkspaceList extends TypeMeta {
  items: Workspace[]
  totalCount: number
}

// --- Namespace ---

export interface NamespaceSpec {
  displayName?: string
  description?: string
  workspaceId: string
  workspaceName?: string
  ownerId: string
  ownerName?: string
  createdByName?: string
  visibility?: "public" | "private"
  maxMembers?: number
  memberCount?: number
  roleBindingCount?: number
  status?: "active" | "inactive"
  roles?: string[]
  roleDisplayNames?: string[]
  joinedAt?: string
}

export interface Namespace extends TypeMeta {
  metadata: ObjectMeta
  spec: NamespaceSpec
}

export interface NamespaceList extends TypeMeta {
  items: Namespace[]
  totalCount: number
}

// --- Permission ---

export interface PermissionSpec {
  code: string
  method: string
  path: string
  scope: "platform" | "workspace" | "namespace"
  description?: string
}

export interface Permission extends TypeMeta {
  metadata: ObjectMeta
  spec: PermissionSpec
}

export interface PermissionList extends TypeMeta {
  items: Permission[]
  totalCount: number
}

// --- Role ---

export interface RoleSpec {
  name: string
  displayName?: string
  description?: string
  scope: "platform" | "workspace" | "namespace"
  builtin?: boolean
  ruleCount?: number
  rules?: string[]
}

export interface Role extends TypeMeta {
  metadata: ObjectMeta
  spec: RoleSpec
}

export interface RoleList extends TypeMeta {
  items: Role[]
  totalCount: number
}

// --- RoleBinding ---

export interface RoleBindingSpec {
  userId: string
  roleId: string
  scope: "platform" | "workspace" | "namespace"
  workspaceId?: string
  namespaceId?: string
  workspaceName?: string
  namespaceName?: string
  isOwner?: boolean
  roleName?: string
  roleDisplayName?: string
  username?: string
  userDisplayName?: string
}

export interface RoleBinding extends TypeMeta {
  metadata: ObjectMeta
  spec: RoleBindingSpec
}

export interface RoleBindingList extends TypeMeta {
  items: RoleBinding[]
  totalCount: number
}

// --- UserPermissions ---
// Owned by core (the session shell reads them before any iam page
// mounts); re-exported here so iam pages keep a single import surface.
export type {
  WorkspaceScopePerms,
  NamespaceScopePerms,
  UserPermissionsSpec,
  UserPermissions,
  OIDCUserInfo,
} from "@/core/auth/identity"

// --- Misc IAM ---

export interface ChangePasswordRequest {
  oldPassword: string
  newPassword: string
}

export interface ResetPasswordRequest {
  newPassword: string
}

export interface TransferOwnershipRequest {
  newOwnerUserId: string
}
