-- name: CreateNamespace :one
INSERT INTO namespaces (name, display_name, description, workspace_id, owner_id, visibility, max_members, status, created_by)
VALUES (@name, @display_name, @description, @workspace_id, @owner_id, @visibility, @max_members, @status, sqlc.narg('created_by'))
RETURNING id, name, display_name, description, workspace_id, owner_id, visibility, max_members, status,
          created_at, updated_at, created_by;

-- name: GetNamespaceByID :one
SELECT
    ns.id, ns.name, ns.display_name, ns.description, ns.workspace_id, ns.owner_id,
    ns.visibility, ns.max_members, ns.status, ns.created_at, ns.updated_at,
    u.username AS owner_username,
    w.name AS workspace_name,
    COALESCE(NULLIF(cu.display_name, ''), cu.username, '') AS creator_name,
    (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'namespace' AND rb.namespace_id = ns.id) AS member_count,
    (SELECT count(*) FROM role_bindings rb WHERE rb.scope = 'namespace' AND rb.namespace_id = ns.id) AS role_binding_count
FROM namespaces ns
JOIN users u ON ns.owner_id = u.id
JOIN workspaces w ON ns.workspace_id = w.id
LEFT JOIN users cu ON cu.id = ns.created_by
WHERE ns.id = @id;

-- name: UpdateNamespace :one
UPDATE namespaces
SET name = @name,
    display_name = @display_name,
    description = @description,
    owner_id = @owner_id,
    visibility = @visibility,
    max_members = @max_members,
    status = @status,
    updated_at = now()
WHERE namespaces.id = @id
RETURNING namespaces.id, namespaces.name, namespaces.display_name, namespaces.description,
    namespaces.workspace_id, namespaces.owner_id, namespaces.visibility, namespaces.max_members,
    namespaces.status, namespaces.created_at, namespaces.updated_at,
    (SELECT u.username FROM users u WHERE u.id = namespaces.owner_id) AS owner_username,
    (SELECT w.name FROM workspaces w WHERE w.id = namespaces.workspace_id) AS workspace_name,
    (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'namespace' AND rb.namespace_id = namespaces.id) AS member_count,
    (SELECT count(*) FROM role_bindings rb WHERE rb.scope = 'namespace' AND rb.namespace_id = namespaces.id) AS role_binding_count;

-- name: DeleteNamespace :execrows
DELETE FROM namespaces WHERE id = @id;

-- name: DeleteNamespacesByIDs :many
DELETE FROM namespaces WHERE id = ANY(@ids::BIGINT[])
RETURNING id;

-- name: DeleteNamespacesByWorkspaceID :exec
DELETE FROM namespaces WHERE workspace_id = @workspace_id;

-- name: DeleteNamespacesByWorkspaceIDs :exec
DELETE FROM namespaces WHERE workspace_id = ANY(@workspace_ids::BIGINT[]);

-- name: CountNamespaces :one
SELECT count(ns.id)
FROM namespaces ns
WHERE
    (sqlc.narg('accessible_ids')::BIGINT[] IS NULL OR ns.id = ANY(sqlc.narg('accessible_ids')::BIGINT[]))
    AND (sqlc.narg('status')::VARCHAR IS NULL OR ns.status = sqlc.narg('status'))
    AND (sqlc.narg('name')::VARCHAR IS NULL OR ns.name ILIKE '%' || sqlc.narg('name') || '%')
    AND (sqlc.narg('visibility')::VARCHAR IS NULL OR ns.visibility = sqlc.narg('visibility'))
    AND (sqlc.narg('owner_id')::BIGINT IS NULL OR ns.owner_id = sqlc.narg('owner_id'))
    AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR ns.workspace_id = sqlc.narg('workspace_id'))
    AND (sqlc.narg('search')::VARCHAR IS NULL OR (
        ns.name ILIKE '%' || sqlc.narg('search') || '%'
        OR ns.display_name ILIKE '%' || sqlc.narg('search') || '%'
        OR ns.description ILIKE '%' || sqlc.narg('search') || '%'
    ));

-- name: ListNamespaces :many
WITH ns_data AS (
    SELECT
        ns.id, ns.name, ns.display_name, ns.description, ns.workspace_id, ns.owner_id,
        ns.visibility, ns.max_members, ns.status, ns.created_at, ns.updated_at,
        u.username AS owner_username,
        w.name AS workspace_name,
        COALESCE(NULLIF(cu.display_name, ''), cu.username, '') AS creator_name,
        (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'namespace' AND rb.namespace_id = ns.id) AS member_count
    FROM namespaces ns
    JOIN users u ON ns.owner_id = u.id
    JOIN workspaces w ON ns.workspace_id = w.id
    LEFT JOIN users cu ON cu.id = ns.created_by
    WHERE
        (sqlc.narg('accessible_ids')::BIGINT[] IS NULL OR ns.id = ANY(sqlc.narg('accessible_ids')::BIGINT[]))
        AND (sqlc.narg('status')::VARCHAR IS NULL OR ns.status = sqlc.narg('status'))
        AND (sqlc.narg('name')::VARCHAR IS NULL OR ns.name ILIKE '%' || sqlc.narg('name') || '%')
        AND (sqlc.narg('visibility')::VARCHAR IS NULL OR ns.visibility = sqlc.narg('visibility'))
        AND (sqlc.narg('owner_id')::BIGINT IS NULL OR ns.owner_id = sqlc.narg('owner_id'))
        AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR ns.workspace_id = sqlc.narg('workspace_id'))
        AND (sqlc.narg('search')::VARCHAR IS NULL OR (
            ns.name ILIKE '%' || sqlc.narg('search') || '%'
            OR ns.display_name ILIKE '%' || sqlc.narg('search') || '%'
            OR ns.description ILIKE '%' || sqlc.narg('search') || '%'
        ))
)
SELECT * FROM ns_data
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN display_name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN display_name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'description' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN description END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'description' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN description END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN created_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN created_at END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'updated_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN updated_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'updated_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN updated_at END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'visibility' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN visibility END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'visibility' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN visibility END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN status END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN status END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'member_count' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN member_count END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'member_count' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN member_count END DESC,
    created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;

-- name: CountUsersByNamespaceID :one
SELECT count(DISTINCT user_id)
FROM role_bindings
WHERE scope = 'namespace' AND namespace_id = @namespace_id;

-- name: GetNamespaceWorkspaceID :one
SELECT workspace_id FROM namespaces WHERE id = @id;

-- name: PatchNamespace :one
UPDATE namespaces
SET name = COALESCE(sqlc.narg('name'), namespaces.name),
    display_name = COALESCE(sqlc.narg('display_name'), namespaces.display_name),
    description = COALESCE(sqlc.narg('description'), namespaces.description),
    owner_id = COALESCE(sqlc.narg('owner_id'), namespaces.owner_id),
    visibility = COALESCE(sqlc.narg('visibility'), namespaces.visibility),
    max_members = COALESCE(sqlc.narg('max_members'), namespaces.max_members),
    status = COALESCE(sqlc.narg('status'), namespaces.status),
    updated_at = now()
WHERE namespaces.id = @id
RETURNING namespaces.id, namespaces.name, namespaces.display_name, namespaces.description,
    namespaces.workspace_id, namespaces.owner_id, namespaces.visibility, namespaces.max_members,
    namespaces.status, namespaces.created_at, namespaces.updated_at,
    (SELECT u.username FROM users u WHERE u.id = namespaces.owner_id) AS owner_username,
    (SELECT w.name FROM workspaces w WHERE w.id = namespaces.workspace_id) AS workspace_name,
    (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'namespace' AND rb.namespace_id = namespaces.id) AS member_count,
    (SELECT count(*) FROM role_bindings rb WHERE rb.scope = 'namespace' AND rb.namespace_id = namespaces.id) AS role_binding_count;
