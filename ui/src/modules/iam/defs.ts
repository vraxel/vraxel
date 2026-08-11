import { defineResource } from "@/core/registry/resource"

// Registry declarations for the iam module. iam's routes.tsx is
// non-standard: workspaces/:workspaceId is a resource DETAIL layout with
// nested children (users/roles/rolebindings/namespaces tabs), not the
// generic workspace scope prefix. Scopes below therefore mirror where
// each resource's LIST API is actually exposed by the backend
// (pkg/apis/iam install): users/roles/rolebindings exist at all three
// scopes, namespaces at platform + under a workspace, workspaces and
// permissions only at platform.
export const usersDef = defineResource({
  module: "iam",
  name: "users",
  scopes: ["platform", "workspace", "namespace"],
  detailParam: "userId",
})

export const rolesDef = defineResource({
  module: "iam",
  name: "roles",
  scopes: ["platform", "workspace", "namespace"],
  detailParam: "roleId",
})

export const workspacesDef = defineResource({
  module: "iam",
  name: "workspaces",
  scopes: ["platform"],
  detailParam: "workspaceId",
})

export const namespacesDef = defineResource({
  module: "iam",
  name: "namespaces",
  scopes: ["platform", "workspace"],
  detailParam: "namespaceId",
})

// Read-only permission registry; used for query keys only.
export const permissionsDef = defineResource({
  module: "iam",
  name: "permissions",
  scopes: ["platform"],
})
