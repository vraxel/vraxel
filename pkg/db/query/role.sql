-- name: CreateRole :one
INSERT INTO roles (name, display_name, description, scope, builtin, workspace_id, namespace_id)
VALUES (@name, @display_name, @description, @scope, @builtin, @workspace_id, @namespace_id)
RETURNING id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at;

-- name: GetRoleByID :one
SELECT id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at
FROM roles
WHERE id = @id;

-- name: GetRoleByName :one
SELECT id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at
FROM roles
WHERE name = @name AND scope = 'platform';

-- name: GetRoleByNameAndWorkspace :one
SELECT id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at
FROM roles
WHERE name = @name AND scope = 'workspace' AND workspace_id = @workspace_id;

-- name: GetRoleByNameAndNamespace :one
SELECT id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at
FROM roles
WHERE name = @name AND scope = 'namespace' AND namespace_id = @namespace_id;

-- name: GetGlobalRoleIDByNameAndScope :one
SELECT id FROM roles
WHERE name = @name AND scope = @scope
  AND workspace_id IS NULL AND namespace_id IS NULL;

-- name: GetWorkspaceRoleIDByName :one
SELECT id FROM roles
WHERE name = @name AND workspace_id = @workspace_id;

-- name: GetNamespaceRoleIDByName :one
SELECT id FROM roles
WHERE name = @name AND namespace_id = @namespace_id;

-- name: UpdateRole :one
UPDATE roles
SET display_name = @display_name,
    description = @description,
    updated_at = now()
WHERE id = @id
RETURNING id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at;

-- name: UpsertRole :one
INSERT INTO roles (name, display_name, description, scope, builtin)
VALUES (@name, @display_name, @description, @scope, @builtin)
ON CONFLICT (name) WHERE scope = 'platform'
DO UPDATE SET display_name = EXCLUDED.display_name,
              description = EXCLUDED.description,
              updated_at = now()
RETURNING id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at;

-- name: DeleteRole :execrows
DELETE FROM roles WHERE id = @id;

-- name: CountRoles :one
SELECT count(id)
FROM roles
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR scope = sqlc.narg('scope'))
  AND (sqlc.narg('builtin')::BOOLEAN IS NULL OR builtin = sqlc.narg('builtin'))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('search')::VARCHAR IS NULL OR (
       name ILIKE '%' || sqlc.narg('search') || '%'
       OR display_name ILIKE '%' || sqlc.narg('search') || '%'
       OR description ILIKE '%' || sqlc.narg('search') || '%'
       OR (sqlc.narg('extra_names')::TEXT[] IS NOT NULL AND name = ANY(sqlc.narg('extra_names')::TEXT[]))
  ));

-- name: ListRoles :many
SELECT id, name, display_name, description, scope, builtin, workspace_id, namespace_id, created_at, updated_at,
       (SELECT COUNT(*) FROM role_permission_rules WHERE role_id = roles.id)::INT AS rule_count
FROM roles
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR scope = sqlc.narg('scope'))
  AND (sqlc.narg('builtin')::BOOLEAN IS NULL OR builtin = sqlc.narg('builtin'))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('search')::VARCHAR IS NULL OR (
       name ILIKE '%' || sqlc.narg('search') || '%'
       OR display_name ILIKE '%' || sqlc.narg('search') || '%'
       OR description ILIKE '%' || sqlc.narg('search') || '%'
       OR (sqlc.narg('extra_names')::TEXT[] IS NOT NULL AND name = ANY(sqlc.narg('extra_names')::TEXT[]))
  ))
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN display_name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN display_name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'scope' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN scope END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'scope' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN scope END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN created_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN created_at END DESC,
    created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;
