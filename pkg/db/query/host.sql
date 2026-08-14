-- hosts: the operator-facing surface of the host record. The agent's own
-- writes live in host_agent_host.sql; this file is what the REST resource
-- reads and mutates.
--
-- Every read joins host_agents. An agent's online/offline is the host's
-- real status under agent management, and joining it here is what lets
-- the list render "never installed" (no row) as a state distinct from
-- "not answering" (row, status offline) without a second round trip.

-- name: CreateHost :one
-- A host recorded by hand. Deliberately different from CreateAgentHost:
-- origin says a human made this row, connectivity_mode starts at 'ssh'
-- because that is all there is until an agent binds, and the hardware
-- facts stay zero until something reports them.
--
-- primary_ip_override carries the operator's address. It is the column
-- anything here would dial (reported_primary_ip is agent-reported and
-- never dialable), and it is optional: a host that will only ever be
-- reached through an outbound agent has no address worth recording.
--
-- status is omitted so the column default fires, which is what the
-- one-vocabulary migration (20260813051100) set it for. Inventing an
-- 'unknown' here would be a third word for a column nothing in vraxel
-- reads: under agent management the host's real status is the agent's
-- online / offline, and the UI shows that instead.
INSERT INTO hosts (
    name, display_name, description, scope, workspace_id, namespace_id,
    ssh_port, origin, connectivity_mode, primary_ip_override, created_by
)
VALUES (
    @name, @display_name, @description, @scope, @workspace_id, @namespace_id,
    @ssh_port, 'manual', 'ssh', @primary_ip_override, sqlc.narg('created_by')
)
RETURNING id;

-- name: GetHostByID :one
SELECT h.*,
    COALESCE(NULLIF(u.display_name, ''), u.username, '') AS creator_name,
    a.agent_id       AS agent_id,
    a.status         AS agent_status,
    a.version        AS agent_version,
    a.connected_at   AS agent_connected_at,
    a.last_seen_at   AS agent_last_seen_at,
    a.conflict_at    AS agent_conflict_at
FROM hosts h
LEFT JOIN users u ON u.id = h.created_by
LEFT JOIN host_agents a ON a.host_id = h.id
WHERE h.id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR h.workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR h.namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: CountHosts :one
SELECT count(*)
FROM hosts h
LEFT JOIN host_agents a ON a.host_id = h.id
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR h.scope = sqlc.narg('scope'))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR h.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR h.namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('origin')::VARCHAR IS NULL OR h.origin = sqlc.narg('origin'))
  -- 'none' is not a value of host_agents.status; it selects the rows that
  -- have no agent at all, which is the state an imported host sits in.
  AND (sqlc.narg('agent_status')::VARCHAR IS NULL
       OR (sqlc.narg('agent_status')::VARCHAR = 'none' AND a.host_id IS NULL)
       OR a.status = sqlc.narg('agent_status'))
  AND (sqlc.narg('search')::VARCHAR IS NULL
       OR h.name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.display_name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.hostname ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.reported_primary_ip ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.primary_ip_override ILIKE '%' || sqlc.narg('search')::VARCHAR || '%');

-- name: ListHosts :many
SELECT h.*,
    COALESCE(NULLIF(u.display_name, ''), u.username, '') AS creator_name,
    a.agent_id       AS agent_id,
    a.status         AS agent_status,
    a.version        AS agent_version,
    a.connected_at   AS agent_connected_at,
    a.last_seen_at   AS agent_last_seen_at,
    a.conflict_at    AS agent_conflict_at
FROM hosts h
LEFT JOIN users u ON u.id = h.created_by
LEFT JOIN host_agents a ON a.host_id = h.id
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR h.scope = sqlc.narg('scope'))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR h.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR h.namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('origin')::VARCHAR IS NULL OR h.origin = sqlc.narg('origin'))
  AND (sqlc.narg('agent_status')::VARCHAR IS NULL
       OR (sqlc.narg('agent_status')::VARCHAR = 'none' AND a.host_id IS NULL)
       OR a.status = sqlc.narg('agent_status'))
  AND (sqlc.narg('search')::VARCHAR IS NULL
       OR h.name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.display_name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.hostname ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.reported_primary_ip ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.primary_ip_override ILIKE '%' || sqlc.narg('search')::VARCHAR || '%')
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN h.name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN h.name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN h.created_at END ASC,
    h.created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;

-- name: UpdateHost :execrows
-- Only the two fields a human owns. Everything else on this row is
-- either agent-reported (and would be overwritten on the next heartbeat)
-- or structural (scope, origin), so there is nothing else to expose.
UPDATE hosts
SET display_name = @display_name,
    description  = @description,
    updated_at   = now()
WHERE id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: DeleteHost :execrows
-- host_agents and any bound join token cascade with the row.
DELETE FROM hosts
WHERE id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: HostScopeByID :one
-- Tenancy of one host, read before a scope-bound join token is minted
-- for it: the token's scope must be the host's own, or redeeming it
-- would move the host between tenants.
SELECT scope, workspace_id, namespace_id FROM hosts WHERE id = @id;
