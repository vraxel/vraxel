-- name: CreateUser :one
INSERT INTO users (username, email, display_name, phone, avatar_url, status)
VALUES (@username, @email, @display_name, @phone, @avatar_url, @status)
RETURNING id, username, email, display_name, phone, avatar_url, status, builtin,
          last_login_at, created_at, updated_at;

-- name: GetUserByID :one
SELECT id, username, email, display_name, phone, avatar_url, status, builtin,
       last_login_at, created_at, updated_at
FROM users
WHERE id = @id;

-- name: GetUserByUsername :one
SELECT id, username, email, display_name, phone, avatar_url, status, builtin,
       last_login_at, created_at, updated_at
FROM users
WHERE username = @username;

-- name: UpdateUser :one
UPDATE users
SET username = @username,
    email = @email,
    display_name = @display_name,
    phone = @phone,
    avatar_url = @avatar_url,
    status = @status,
    updated_at = now()
WHERE id = @id
RETURNING id, username, email, display_name, phone, avatar_url, status, builtin,
          last_login_at, created_at, updated_at;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login_at = now(), updated_at = now()
WHERE id = @id;

-- builtin users (admin, future system accounts) cannot be removed via API.
-- A 0 rowsAffected return for an existing builtin user is translated to a
-- "builtin not deletable" error in the store layer; "real" not-found stays
-- ErrNotFound.

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = @id AND NOT builtin;

-- name: CountUsers :one
SELECT count(id)
FROM users
WHERE
    (sqlc.narg('status')::VARCHAR IS NULL OR status = sqlc.narg('status'))
    AND (sqlc.narg('search')::VARCHAR IS NULL OR (
        username ILIKE '%' || sqlc.narg('search') || '%'
        OR email ILIKE '%' || sqlc.narg('search') || '%'
        OR phone ILIKE '%' || sqlc.narg('search') || '%'
        OR display_name ILIKE '%' || sqlc.narg('search') || '%'
    ));

-- name: ListUsers :many
SELECT
    u.id, u.username, u.email, u.display_name, u.phone, u.avatar_url,
    u.status, u.builtin, u.last_login_at, u.created_at, u.updated_at,
    COALESCE(
        array_agg(DISTINCT n.name) FILTER (WHERE n.name IS NOT NULL),
        '{}'
    )::TEXT[] AS namespace_names
FROM users u
LEFT JOIN role_bindings rb ON u.id = rb.user_id AND rb.scope = 'namespace'
LEFT JOIN namespaces n ON rb.namespace_id = n.id
WHERE
    (sqlc.narg('status')::VARCHAR IS NULL OR u.status = sqlc.narg('status'))
    AND (sqlc.narg('search')::VARCHAR IS NULL OR (
        u.username ILIKE '%' || sqlc.narg('search') || '%'
        OR u.email ILIKE '%' || sqlc.narg('search') || '%'
        OR u.phone ILIKE '%' || sqlc.narg('search') || '%'
        OR u.display_name ILIKE '%' || sqlc.narg('search') || '%'
    ))
GROUP BY u.id, u.username, u.email, u.display_name, u.phone, u.avatar_url,
         u.status, u.builtin, u.last_login_at, u.created_at, u.updated_at
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'username' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN u.username END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'username' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN u.username END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'email' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN u.email END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'email' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN u.email END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN u.display_name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'display_name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN u.display_name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'phone' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN u.phone END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'phone' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN u.phone END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN u.created_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN u.created_at END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN u.status END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN u.status END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'updated_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN u.updated_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'updated_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN u.updated_at END DESC,
    u.created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;

-- name: PatchUser :one
UPDATE users
SET username = COALESCE(sqlc.narg('username'), username),
    email = COALESCE(sqlc.narg('email'), email),
    display_name = COALESCE(sqlc.narg('display_name'), display_name),
    phone = COALESCE(sqlc.narg('phone'), phone),
    avatar_url = COALESCE(sqlc.narg('avatar_url'), avatar_url),
    status = COALESCE(sqlc.narg('status'), status),
    updated_at = now()
WHERE id = @id
RETURNING id, username, email, display_name, phone, avatar_url, status, builtin,
          last_login_at, created_at, updated_at;

-- name: DeleteUsersByIDs :many
DELETE FROM users WHERE id = ANY(@ids::BIGINT[]) AND NOT builtin
RETURNING id;

-- name: GetUsersByIDs :many
SELECT id, username, email, display_name, phone, avatar_url, status, builtin,
       last_login_at, created_at, updated_at
FROM users WHERE id = ANY(@ids::BIGINT[]);

-- name: GetUserForAuth :one
SELECT id, username, email, display_name, phone, status, password_hash
FROM users WHERE username = @identifier OR email = @identifier;

-- name: SetPasswordHash :exec
UPDATE users SET password_hash = @password_hash, updated_at = now()
WHERE id = @id;

-- name: SetUserBuiltin :exec
UPDATE users SET builtin = @builtin, updated_at = now()
WHERE id = @id;
