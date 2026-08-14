-- name: CreateAuditLog :exec
INSERT INTO audit_logs (
    user_id, username, event_type, action, resource_type, resource_id,
    module, scope, workspace_id, namespace_id,
    http_method, http_path, status_code, client_ip, user_agent,
    duration_ms, success, detail, response_detail, created_at
) VALUES (
    @user_id, @username, @event_type, @action, @resource_type, @resource_id,
    @module, @scope, @workspace_id, @namespace_id,
    @http_method, @http_path, @status_code, @client_ip, @user_agent,
    @duration_ms, @success, @detail, @response_detail, @created_at
);

-- name: GetAuditLog :one
SELECT id, user_id, username, event_type, action, resource_type, resource_id,
       module, scope, workspace_id, namespace_id,
       http_method, http_path, status_code, client_ip, user_agent,
       duration_ms, success, detail, response_detail, created_at
FROM audit_logs
WHERE id = @id;

-- name: CountAuditLogs :one
SELECT count(id)
FROM audit_logs
WHERE
    (sqlc.narg('user_id')::BIGINT IS NULL OR user_id = sqlc.narg('user_id'))
    AND (sqlc.narg('event_type')::VARCHAR IS NULL OR event_type = ANY(string_to_array(sqlc.narg('event_type')::VARCHAR, ',')))
    AND (sqlc.narg('action')::VARCHAR IS NULL OR action = ANY(string_to_array(sqlc.narg('action')::VARCHAR, ',')))
    AND (sqlc.narg('resource_type')::VARCHAR IS NULL OR resource_type = sqlc.narg('resource_type'))
    AND (sqlc.narg('resource_id')::VARCHAR IS NULL OR resource_id = sqlc.narg('resource_id'))
    AND (sqlc.narg('module')::VARCHAR IS NULL OR module = ANY(string_to_array(sqlc.narg('module')::VARCHAR, ',')))
    AND (sqlc.narg('client_ip')::VARCHAR IS NULL OR client_ip = sqlc.narg('client_ip'))
    AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR workspace_id = sqlc.narg('workspace_id'))
    AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR namespace_id = sqlc.narg('namespace_id'))
    AND (sqlc.narg('success')::BOOLEAN IS NULL OR success = sqlc.narg('success'))
    AND (sqlc.narg('status_code')::INT IS NULL OR status_code = sqlc.narg('status_code'))
    AND (sqlc.narg('start_time')::TIMESTAMPTZ IS NULL OR created_at >= sqlc.narg('start_time'))
    AND (sqlc.narg('end_time')::TIMESTAMPTZ IS NULL OR created_at <= sqlc.narg('end_time'))
    AND (sqlc.narg('search')::VARCHAR IS NULL OR (
        username ILIKE '%' || sqlc.narg('search') || '%'
        OR resource_type ILIKE '%' || sqlc.narg('search') || '%'
        OR (resource_type || '/' || resource_id) ILIKE '%' || sqlc.narg('search') || '%'
        OR module ILIKE '%' || sqlc.narg('search') || '%'
        OR client_ip ILIKE '%' || sqlc.narg('search') || '%'
        OR CAST(status_code AS VARCHAR) ILIKE '%' || sqlc.narg('search') || '%'
    ));

-- name: ListAuditLogs :many
SELECT id, user_id, username, event_type, action, resource_type, resource_id,
       module, scope, workspace_id, namespace_id,
       http_method, http_path, status_code, client_ip, user_agent,
       duration_ms, success, detail, response_detail, created_at
FROM audit_logs
WHERE
    (sqlc.narg('user_id')::BIGINT IS NULL OR user_id = sqlc.narg('user_id'))
    AND (sqlc.narg('event_type')::VARCHAR IS NULL OR event_type = ANY(string_to_array(sqlc.narg('event_type')::VARCHAR, ',')))
    AND (sqlc.narg('action')::VARCHAR IS NULL OR action = ANY(string_to_array(sqlc.narg('action')::VARCHAR, ',')))
    AND (sqlc.narg('resource_type')::VARCHAR IS NULL OR resource_type = sqlc.narg('resource_type'))
    AND (sqlc.narg('resource_id')::VARCHAR IS NULL OR resource_id = sqlc.narg('resource_id'))
    AND (sqlc.narg('module')::VARCHAR IS NULL OR module = ANY(string_to_array(sqlc.narg('module')::VARCHAR, ',')))
    AND (sqlc.narg('client_ip')::VARCHAR IS NULL OR client_ip = sqlc.narg('client_ip'))
    AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR workspace_id = sqlc.narg('workspace_id'))
    AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR namespace_id = sqlc.narg('namespace_id'))
    AND (sqlc.narg('success')::BOOLEAN IS NULL OR success = sqlc.narg('success'))
    AND (sqlc.narg('status_code')::INT IS NULL OR status_code = sqlc.narg('status_code'))
    AND (sqlc.narg('start_time')::TIMESTAMPTZ IS NULL OR created_at >= sqlc.narg('start_time'))
    AND (sqlc.narg('end_time')::TIMESTAMPTZ IS NULL OR created_at <= sqlc.narg('end_time'))
    AND (sqlc.narg('search')::VARCHAR IS NULL OR (
        username ILIKE '%' || sqlc.narg('search') || '%'
        OR resource_type ILIKE '%' || sqlc.narg('search') || '%'
        OR (resource_type || '/' || resource_id) ILIKE '%' || sqlc.narg('search') || '%'
        OR module ILIKE '%' || sqlc.narg('search') || '%'
        OR client_ip ILIKE '%' || sqlc.narg('search') || '%'
        OR CAST(status_code AS VARCHAR) ILIKE '%' || sqlc.narg('search') || '%'
    ))
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN created_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN created_at END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'event_type' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN event_type END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'event_type' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN event_type END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'action' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN action END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'action' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN action END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'username' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN username END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'username' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN username END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'resource_type' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN resource_type END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'resource_type' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN resource_type END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'module' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN module END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'module' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN module END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status_code' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN status_code END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'status_code' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN status_code END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'duration_ms' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN duration_ms END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'duration_ms' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN duration_ms END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'success' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN success END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'success' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN success END DESC,
    created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;
