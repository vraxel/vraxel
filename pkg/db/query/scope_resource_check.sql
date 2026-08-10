-- Workspace / Namespace deletion pre-check.
-- Each query returns one row per blocking resource kind whose count > 0
-- in the target scope. An empty result means the scope is safe to delete.
--
-- Blocking resources are user-deployed primary resources whose silent
-- cascade-delete or orphaning would surprise the operator. Pure metadata
-- (credentials, role bindings, roles) intentionally cascades and is not
-- listed here.
--
-- No business module is registered yet, so both queries carry a single
-- always-false branch that keeps them well-typed. To register a blocking
-- resource, add one branch:
--
--     UNION ALL
--     SELECT 'host', COUNT(*)::BIGINT
--         FROM hosts WHERE hosts.namespace_id = sqlc.narg('ns_id')
--
-- and the equivalent workspace_id / namespace-in-workspace branch below.

-- name: CountNamespaceBlockingResources :many
SELECT kind, cnt
FROM (
    SELECT ''::TEXT AS kind, 0::BIGINT AS cnt
    WHERE FALSE AND sqlc.narg('ns_id')::BIGINT IS NOT NULL
) t
WHERE cnt > 0
ORDER BY kind;

-- name: CountWorkspaceBlockingResources :many
-- Counts both workspace-scope rows (workspace_id = @ws_id) and
-- namespace-scope rows under any namespace whose workspace_id matches.
SELECT kind, cnt
FROM (
    SELECT ''::TEXT AS kind, 0::BIGINT AS cnt
    WHERE FALSE AND sqlc.narg('ws_id')::BIGINT IS NOT NULL
) t
WHERE cnt > 0
ORDER BY kind;
