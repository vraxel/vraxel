-- name: CreateWorkspace :one
INSERT INTO workspaces (name, display_name, description, owner_id, status, created_by)
VALUES (@name, @display_name, @description, @owner_id, @status, sqlc.narg('created_by'))
RETURNING id, name, display_name, description, owner_id, status,
          created_at, updated_at, created_by;

-- name: GetWorkspaceByID :one
SELECT
    ws.id, ws.name, ws.display_name, ws.description, ws.owner_id, ws.status,
    ws.created_at, ws.updated_at,
    u.username AS owner_username,
    COALESCE(NULLIF(cu.display_name, ''), cu.username, '') AS creator_name,
    (SELECT count(*) FROM namespaces n WHERE n.workspace_id = ws.id) AS namespace_count,
    (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'workspace' AND rb.workspace_id = ws.id) AS member_count,
    (SELECT count(*) FROM role_bindings rb WHERE rb.scope = 'workspace' AND rb.workspace_id = ws.id) AS role_binding_count
FROM workspaces ws
JOIN users u ON ws.owner_id = u.id
LEFT JOIN users cu ON cu.id = ws.created_by
WHERE ws.id = @id;

-- name: UpdateWorkspace :one
UPDATE workspaces
SET name = @name,
    display_name = @display_name,
    description = @description,
    owner_id = @owner_id,
    status = @status,
    updated_at = now()
WHERE workspaces.id = @id
RETURNING workspaces.id, workspaces.name, workspaces.display_name, workspaces.description,
    workspaces.owner_id, workspaces.status, workspaces.created_at, workspaces.updated_at,
    (SELECT u.username FROM users u WHERE u.id = workspaces.owner_id) AS owner_username,
    (SELECT count(*) FROM namespaces n WHERE n.workspace_id = workspaces.id) AS namespace_count,
    (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'workspace' AND rb.workspace_id = workspaces.id) AS member_count,
    (SELECT count(*) FROM role_bindings rb WHERE rb.scope = 'workspace' AND rb.workspace_id = workspaces.id) AS role_binding_count;

-- name: DeleteWorkspace :execrows
DELETE FROM workspaces WHERE id = @id;

-- name: DeleteWorkspacesByIDs :many
DELETE FROM workspaces WHERE id = ANY(@ids::BIGINT[])
RETURNING id;

-- name: CountWorkspaces :one
SELECT count(ws.id)
FROM workspaces ws
WHERE
    (sqlc.narg('accessible_ids')::BIGINT[] IS NULL OR ws.id = ANY(sqlc.narg('accessible_ids')::BIGINT[]))
    AND (sqlc.narg('status')::VARCHAR IS NULL OR ws.status = sqlc.narg('status'))
    AND (sqlc.narg('name')::VARCHAR IS NULL OR ws.name ILIKE '%' || sqlc.narg('name') || '%')
    AND (sqlc.narg('owner_id')::BIGINT IS NULL OR ws.owner_id = sqlc.narg('owner_id'))
    AND (sqlc.narg('search')::VARCHAR IS NULL
         OR ws.name ILIKE '%' || sqlc.narg('search') || '%'
         OR ws.display_name ILIKE '%' || sqlc.narg('search') || '%'
         OR ws.description ILIKE '%' || sqlc.narg('search') || '%');

-- name: ListWorkspaces :many
WITH ws_data AS (
    SELECT
        ws.id, ws.name, ws.display_name, ws.description, ws.owner_id,
        ws.status, ws.created_at, ws.updated_at,
        u.username AS owner_username,
        COALESCE(NULLIF(cu.display_name, ''), cu.username, '') AS creator_name,
        (SELECT count(*) FROM namespaces n WHERE n.workspace_id = ws.id) AS namespace_count,
        (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'workspace' AND rb.workspace_id = ws.id) AS member_count
    FROM workspaces ws
    JOIN users u ON ws.owner_id = u.id
    LEFT JOIN users cu ON cu.id = ws.created_by
    WHERE
        (sqlc.narg('accessible_ids')::BIGINT[] IS NULL OR ws.id = ANY(sqlc.narg('accessible_ids')::BIGINT[]))
        AND (sqlc.narg('status')::VARCHAR IS NULL OR ws.status = sqlc.narg('status'))
        AND (sqlc.narg('name')::VARCHAR IS NULL OR ws.name ILIKE '%' || sqlc.narg('name') || '%')
        AND (sqlc.narg('owner_id')::BIGINT IS NULL OR ws.owner_id = sqlc.narg('owner_id'))
        AND (sqlc.narg('search')::VARCHAR IS NULL
             OR ws.name ILIKE '%' || sqlc.narg('search') || '%'
             OR ws.display_name ILIKE '%' || sqlc.narg('search') || '%'
             OR ws.description ILIKE '%' || sqlc.narg('search') || '%')
)
SELECT * FROM ws_data
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN created_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN created_at END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN status END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN status END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN display_name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN display_name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'updated_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN updated_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'updated_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN updated_at END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'namespace_count' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN namespace_count END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'namespace_count' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN namespace_count END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'member_count' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN member_count END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'member_count' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN member_count END DESC,
    created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;

-- name: CountNamespacesByWorkspaceID :one
SELECT count(id)
FROM namespaces
WHERE workspace_id = @workspace_id;

-- name: PatchWorkspace :one
UPDATE workspaces
SET name = COALESCE(sqlc.narg('name'), workspaces.name),
    display_name = COALESCE(sqlc.narg('display_name'), workspaces.display_name),
    description = COALESCE(sqlc.narg('description'), workspaces.description),
    owner_id = COALESCE(sqlc.narg('owner_id'), workspaces.owner_id),
    status = COALESCE(sqlc.narg('status'), workspaces.status),
    updated_at = now()
WHERE workspaces.id = @id
RETURNING workspaces.id, workspaces.name, workspaces.display_name, workspaces.description,
    workspaces.owner_id, workspaces.status, workspaces.created_at, workspaces.updated_at,
    (SELECT u.username FROM users u WHERE u.id = workspaces.owner_id) AS owner_username,
    (SELECT count(*) FROM namespaces n WHERE n.workspace_id = workspaces.id) AS namespace_count,
    (SELECT count(DISTINCT rb.user_id) FROM role_bindings rb WHERE rb.scope = 'workspace' AND rb.workspace_id = workspaces.id) AS member_count,
    (SELECT count(*) FROM role_bindings rb WHERE rb.scope = 'workspace' AND rb.workspace_id = workspaces.id) AS role_binding_count;

