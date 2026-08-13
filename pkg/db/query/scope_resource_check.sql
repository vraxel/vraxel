-- Workspace / Namespace deletion pre-check.
-- Each query returns one row per blocking resource kind whose count > 0
-- in the target scope. An empty result means the scope is safe to delete.
--
-- Blocking resources are user-deployed primary resources whose silent
-- cascade-delete or orphaning would surprise the operator. Pure metadata
-- (credentials, role bindings, roles) intentionally cascades and is not
-- listed here.
--
-- To register a blocking resource, add one branch per query following the
-- hosts pair below.
--
-- hosts has no FK to workspaces / namespaces (unlike, say,
-- host_agent_join_tokens, which cascades). Without this check deleting a
-- namespace left its hosts behind pointing at an id that no longer
-- exists, with their agents still connected and still being managed on
-- behalf of a tenant that is gone.

-- name: CountNamespaceBlockingResources :many
SELECT kind, cnt
FROM (
    SELECT ''::TEXT AS kind, 0::BIGINT AS cnt
    WHERE FALSE AND sqlc.narg('ns_id')::BIGINT IS NOT NULL

    UNION ALL
    SELECT 'host'::TEXT, COUNT(*)::BIGINT
        FROM hosts WHERE hosts.namespace_id = sqlc.narg('ns_id')::BIGINT
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

    UNION ALL
    SELECT 'host'::TEXT, COUNT(*)::BIGINT
        FROM hosts
        WHERE hosts.workspace_id = sqlc.narg('ws_id')::BIGINT
           OR hosts.namespace_id IN (
               SELECT id FROM namespaces WHERE workspace_id = sqlc.narg('ws_id')::BIGINT
           )
) t
WHERE cnt > 0
ORDER BY kind;
