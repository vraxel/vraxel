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
    COALESCE(NULLIF(w.display_name, ''), w.name, '') AS workspace_name,
    COALESCE(NULLIF(ns.display_name, ''), ns.name, '') AS namespace_name,
    a.agent_id       AS agent_id,
    a.status         AS agent_status,
    a.version        AS agent_version,
    a.connected_at   AS agent_connected_at,
    a.last_seen_at   AS agent_last_seen_at,
    a.conflict_at    AS agent_conflict_at,
    a.foreign_machine_at   AS agent_foreign_machine_at,
    a.foreign_machine_uuid AS agent_foreign_machine_uuid,
    -- How many hosts were built from this host's disk image, this one
    -- included. 1 (or 0 for an agentless record) is the ordinary answer.
    --
    -- A scalar subquery rather than a join: the count is over ALL hosts
    -- sharing the machine id, which the WHERE clause below is busy
    -- narrowing away. It is also the whole point -- the sibling an
    -- operator needs to know about is often the one their current filter
    -- hides.
    (SELECT count(*) FROM host_agents s
      WHERE s.machine_id <> '' AND s.machine_id = a.machine_id) AS image_group_size,
    m.sampled_at     AS metrics_sampled_at,
    m.cpu_used_pct   AS metrics_cpu_used_pct,
    m.mem_used_pct   AS metrics_mem_used_pct,
    m.disk_used_pct  AS metrics_disk_used_pct,
    m.disk_used_path AS metrics_disk_used_path,
    m.load1          AS metrics_load1,
    m.load5          AS metrics_load5,
    m.load15         AS metrics_load15,
    m.net_rx_bps     AS metrics_net_rx_bps,
    m.net_tx_bps     AS metrics_net_tx_bps,
    m.cpu_trend      AS metrics_cpu_trend,
    -- Firing threshold alerts, for the list's red badge. A scalar
    -- subquery over a table that only holds live problems, so it is
    -- cheap exactly when everything is fine.
    (SELECT count(*) FROM host_alert_states s2
      WHERE s2.host_id = h.id AND s2.firing) AS alerts_firing,
    -- The firing alerts themselves, detail-only: which thresholds are
    -- breached, not just how many. json_agg because the shape is a
    -- read-only display list that goes straight to the API response.
    (SELECT COALESCE(jsonb_agg(jsonb_build_object(
        'ruleId', s3.rule_id::text,
        'ruleName', r3.name,
        'metric', r3.metric,
        'op', r3.op,
        'threshold', r3.threshold,
        'severity', r3.severity,
        'value', s3.value,
        'since', s3.firing_since
      ) ORDER BY s3.firing_since DESC), '[]'::jsonb)
      FROM host_alert_states s3
      JOIN host_alert_rules r3 ON r3.id = s3.rule_id
      WHERE s3.host_id = h.id AND s3.firing)::jsonb AS firing_alerts
FROM hosts h
LEFT JOIN users u ON u.id = h.created_by
LEFT JOIN workspaces w ON w.id = h.workspace_id
LEFT JOIN namespaces ns ON ns.id = h.namespace_id
LEFT JOIN host_agents a ON a.host_id = h.id
LEFT JOIN host_metrics_latest m ON m.host_id = h.id
WHERE h.id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR h.workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR h.namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: CountHosts :one
SELECT count(*)
FROM hosts h
LEFT JOIN workspaces w ON w.id = h.workspace_id
LEFT JOIN namespaces ns ON ns.id = h.namespace_id
LEFT JOIN host_agents a ON a.host_id = h.id
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR h.scope = ANY(string_to_array(sqlc.narg('scope')::VARCHAR, ',')))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR h.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR h.namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('origin')::VARCHAR IS NULL OR h.origin = ANY(string_to_array(sqlc.narg('origin')::VARCHAR, ',')))
  AND (sqlc.narg('agent_status')::VARCHAR IS NULL
       OR (a.host_id IS NULL AND 'none' = ANY(string_to_array(sqlc.narg('agent_status')::VARCHAR, ',')))
       OR a.status = ANY(string_to_array(sqlc.narg('agent_status')::VARCHAR, ',')))
  AND (sqlc.narg('search')::VARCHAR IS NULL
       OR h.name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.display_name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.hostname ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.reported_primary_ip ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.primary_ip_override ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.os ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.arch ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR CAST(h.cpu_cores AS TEXT) = sqlc.narg('search')::VARCHAR
       OR CAST(h.memory_mb AS TEXT) = sqlc.narg('search')::VARCHAR
       OR COALESCE(w.name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR COALESCE(w.display_name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR COALESCE(ns.name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR COALESCE(ns.display_name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%');

-- name: ListHosts :many
SELECT h.*,
    COALESCE(NULLIF(u.display_name, ''), u.username, '') AS creator_name,
    COALESCE(NULLIF(w.display_name, ''), w.name, '') AS workspace_name,
    COALESCE(NULLIF(ns.display_name, ''), ns.name, '') AS namespace_name,
    a.agent_id       AS agent_id,
    a.status         AS agent_status,
    a.version        AS agent_version,
    a.connected_at   AS agent_connected_at,
    a.last_seen_at   AS agent_last_seen_at,
    a.conflict_at    AS agent_conflict_at,
    a.foreign_machine_at   AS agent_foreign_machine_at,
    a.foreign_machine_uuid AS agent_foreign_machine_uuid,
    -- How many hosts were built from this host's disk image, this one
    -- included. 1 (or 0 for an agentless record) is the ordinary answer.
    --
    -- A scalar subquery rather than a join: the count is over ALL hosts
    -- sharing the machine id, which the WHERE clause below is busy
    -- narrowing away. It is also the whole point -- the sibling an
    -- operator needs to know about is often the one their current filter
    -- hides.
    (SELECT count(*) FROM host_agents s
      WHERE s.machine_id <> '' AND s.machine_id = a.machine_id) AS image_group_size,
    m.sampled_at     AS metrics_sampled_at,
    m.cpu_used_pct   AS metrics_cpu_used_pct,
    m.mem_used_pct   AS metrics_mem_used_pct,
    m.disk_used_pct  AS metrics_disk_used_pct,
    m.disk_used_path AS metrics_disk_used_path,
    m.load1          AS metrics_load1,
    m.load5          AS metrics_load5,
    m.load15         AS metrics_load15,
    m.net_rx_bps     AS metrics_net_rx_bps,
    m.net_tx_bps     AS metrics_net_tx_bps,
    m.cpu_trend      AS metrics_cpu_trend,
    -- Firing threshold alerts, for the list's red badge. A scalar
    -- subquery over a table that only holds live problems, so it is
    -- cheap exactly when everything is fine.
    (SELECT count(*) FROM host_alert_states s2
      WHERE s2.host_id = h.id AND s2.firing) AS alerts_firing
FROM hosts h
LEFT JOIN users u ON u.id = h.created_by
LEFT JOIN workspaces w ON w.id = h.workspace_id
LEFT JOIN namespaces ns ON ns.id = h.namespace_id
LEFT JOIN host_agents a ON a.host_id = h.id
LEFT JOIN host_metrics_latest m ON m.host_id = h.id
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR h.scope = ANY(string_to_array(sqlc.narg('scope')::VARCHAR, ',')))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR h.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR h.namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('origin')::VARCHAR IS NULL OR h.origin = ANY(string_to_array(sqlc.narg('origin')::VARCHAR, ',')))
  AND (sqlc.narg('agent_status')::VARCHAR IS NULL
       OR (a.host_id IS NULL AND 'none' = ANY(string_to_array(sqlc.narg('agent_status')::VARCHAR, ',')))
       OR a.status = ANY(string_to_array(sqlc.narg('agent_status')::VARCHAR, ',')))
  AND (sqlc.narg('search')::VARCHAR IS NULL
       OR h.name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.display_name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.hostname ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.reported_primary_ip ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.primary_ip_override ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.os ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR h.arch ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR CAST(h.cpu_cores AS TEXT) = sqlc.narg('search')::VARCHAR
       OR CAST(h.memory_mb AS TEXT) = sqlc.narg('search')::VARCHAR
       OR COALESCE(w.name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR COALESCE(w.display_name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR COALESCE(ns.name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR COALESCE(ns.display_name, '') ILIKE '%' || sqlc.narg('search')::VARCHAR || '%')
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN h.name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN h.name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'ip' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN h.reported_primary_ip END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'ip' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN h.reported_primary_ip END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'os' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN h.os END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'os' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN h.os END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'cpu_cores' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN h.cpu_cores END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'cpu_cores' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN h.cpu_cores END DESC,
    -- Metric sorts keep agentless hosts (NULL) at the bottom in BOTH
    -- directions: "most loaded first" must not open with a page of
    -- hosts that reported nothing. ASC gets that from PG's default;
    -- DESC needs it said.
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'cpu_used_pct' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN m.cpu_used_pct END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'cpu_used_pct' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN m.cpu_used_pct END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'mem_used_pct' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN m.mem_used_pct END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'mem_used_pct' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN m.mem_used_pct END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'disk_used_pct' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN m.disk_used_pct END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'disk_used_pct' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN m.disk_used_pct END DESC NULLS LAST,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'organization' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN COALESCE(NULLIF(w.display_name, ''), w.name, '') END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'organization' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN COALESCE(NULLIF(ns.display_name, ''), ns.name, '') END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'organization' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN COALESCE(NULLIF(w.display_name, ''), w.name, '') END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'organization' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN COALESCE(NULLIF(ns.display_name, ''), ns.name, '') END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'origin' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN h.origin END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'origin' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN h.origin END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_by' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN COALESCE(NULLIF(u.display_name, ''), u.username, '') END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_by' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN COALESCE(NULLIF(u.display_name, ''), u.username, '') END DESC,
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

-- name: DeleteHost :one
-- host_agents and any bound join token cascade with the row.
--
-- RETURNING the tenancy is what lets the deletion be announced: the watch
-- event needs a scope to route by, and after this statement there is no
-- row left to read one from. No rows means "not found" here, the same
-- thing zero affected rows used to mean.
DELETE FROM hosts
WHERE id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT)
RETURNING scope, workspace_id, namespace_id;

-- name: HostScopeByID :one
-- Tenancy of one host, read before a scope-bound join token is minted
-- for it: the token's scope must be the host's own, or redeeming it
-- would move the host between tenants.
SELECT scope, workspace_id, namespace_id FROM hosts WHERE id = @id;
