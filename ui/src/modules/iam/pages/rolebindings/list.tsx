import { useMemo } from "react"
import {
  listRoleBindings,
  createRoleBindings,
  deleteRoleBinding,
  deleteRoleBindings,
  listRoles,
} from "@/modules/iam/api/rbac"
import { listUsers } from "@/modules/iam/api/users"
import {
  RoleBindingListView,
  type RoleBindingListConfig,
} from "@/modules/iam/components/rolebinding-list-view"

export default function RoleBindingListPage() {
  const config = useMemo<RoleBindingListConfig>(
    () => ({
      listBindings: (params) => listRoleBindings(params),
      createBindings: (ids, roleId) => createRoleBindings(ids, roleId),
      deleteBinding: (id) => deleteRoleBinding(id),
      deleteBindings: (ids) => deleteRoleBindings(ids),
      listRoles: (params) => listRoles(params),
      listUsers: (params) => listUsers(params),
      permCreate: "iam:rolebindings:create",
      permDelete: "iam:rolebindings:delete",
      scope: "platform",
    }),
    [],
  )

  return <RoleBindingListView config={config} />
}
