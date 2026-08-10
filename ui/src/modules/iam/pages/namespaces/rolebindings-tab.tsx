import { useMemo } from "react"
import { useParams, Navigate } from "react-router"
import {
  listNamespaceRoleBindings,
  createNamespaceRoleBinding,
  deleteNamespaceRoleBinding,
  deleteNamespaceRoleBindings,
  listNamespaceRoles,
} from "@/modules/iam/api/rbac"
import { listNamespaceUsers } from "@/modules/iam/api/users"
import { usePermission } from "@/core/permission/use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"
import {
  RoleBindingListView,
  type RoleBindingListConfig,
} from "@/modules/iam/components/rolebinding-list-view"

export default function NamespaceRoleBindingsTab() {
  const { workspaceId, namespaceId } = useParams() as { workspaceId: string; namespaceId: string }
  const { hasPermission } = usePermission()
  const permissionsLoaded = usePermissionStore((s) => s.permissions) !== null

  const config = useMemo<RoleBindingListConfig>(
    () => ({
      listBindings: (params) => listNamespaceRoleBindings(workspaceId, namespaceId, params),
      createBinding: (data) => createNamespaceRoleBinding(workspaceId, namespaceId, data),
      deleteBinding: (id) => deleteNamespaceRoleBinding(workspaceId, namespaceId, id),
      deleteBindings: (ids) => deleteNamespaceRoleBindings(workspaceId, namespaceId, ids),
      listRoles: (params) => listNamespaceRoles(workspaceId, namespaceId, params),
      listUsers: (params) =>
        listNamespaceUsers(workspaceId, namespaceId, { ...params, available: "true" }),
      permCreate: "iam:rolebindings:create",
      permDelete: "iam:rolebindings:delete",
      scope: "namespace",
      scopeParams: { workspaceId, namespaceId },
    }),
    [workspaceId, namespaceId],
  )

  if (permissionsLoaded && !hasPermission("iam:rolebindings:list", { workspaceId, namespaceId })) {
    return <Navigate to="/" replace />
  }

  return <RoleBindingListView config={config} />
}
