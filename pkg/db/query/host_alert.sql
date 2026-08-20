-- Threshold alert rules (operator CRUD, compute module) and per-host
-- breach states (written only by the agent gateway's evaluator, and only
-- on transitions -- never once per beat).

-- name: ListHostAlertRules :many
SELECT r.*,
    COALESCE(NULLIF(u.display_name, ''), u.username, '') AS creator_name,
    COALESCE(NULLIF(w.display_name, ''), w.name, '') AS workspace_name,
    COALESCE(NULLIF(ns.display_name, ''), ns.name, '') AS namespace_name,
    (SELECT count(*) FROM host_alert_states s
      WHERE s.rule_id = r.id AND s.firing) AS firing_count
FROM host_alert_rules r
LEFT JOIN users u ON u.id = r.created_by
LEFT JOIN workspaces w ON w.id = r.workspace_id
LEFT JOIN namespaces ns ON ns.id = r.namespace_id
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR r.scope = ANY(string_to_array(sqlc.narg('scope')::VARCHAR, ',')))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR r.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR r.namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('metric')::VARCHAR IS NULL OR r.metric = ANY(string_to_array(sqlc.narg('metric')::VARCHAR, ',')))
  AND (sqlc.narg('severity')::VARCHAR IS NULL OR r.severity = ANY(string_to_array(sqlc.narg('severity')::VARCHAR, ',')))
  AND (sqlc.narg('enabled')::BOOLEAN IS NULL OR r.enabled = sqlc.narg('enabled'))
  AND (sqlc.narg('search')::VARCHAR IS NULL
       OR r.name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR r.description ILIKE '%' || sqlc.narg('search')::VARCHAR || '%')
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN r.name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN r.name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'metric' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN r.metric END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'metric' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN r.metric END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'severity' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN r.severity END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'severity' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN r.severity END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_by' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN COALESCE(NULLIF(u.display_name, ''), u.username, '') END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_by' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN COALESCE(NULLIF(u.display_name, ''), u.username, '') END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN r.created_at END ASC,
    r.created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;

-- name: CountHostAlertRules :one
SELECT count(*)
FROM host_alert_rules r
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR r.scope = ANY(string_to_array(sqlc.narg('scope')::VARCHAR, ',')))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR r.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR r.namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('metric')::VARCHAR IS NULL OR r.metric = ANY(string_to_array(sqlc.narg('metric')::VARCHAR, ',')))
  AND (sqlc.narg('severity')::VARCHAR IS NULL OR r.severity = ANY(string_to_array(sqlc.narg('severity')::VARCHAR, ',')))
  AND (sqlc.narg('enabled')::BOOLEAN IS NULL OR r.enabled = sqlc.narg('enabled'))
  AND (sqlc.narg('search')::VARCHAR IS NULL
       OR r.name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%'
       OR r.description ILIKE '%' || sqlc.narg('search')::VARCHAR || '%');

-- name: GetHostAlertRule :one
SELECT r.*,
    COALESCE(NULLIF(u.display_name, ''), u.username, '') AS creator_name,
    COALESCE(NULLIF(w.display_name, ''), w.name, '') AS workspace_name,
    COALESCE(NULLIF(ns.display_name, ''), ns.name, '') AS namespace_name,
    (SELECT count(*) FROM host_alert_states s
      WHERE s.rule_id = r.id AND s.firing) AS firing_count
FROM host_alert_rules r
LEFT JOIN users u ON u.id = r.created_by
LEFT JOIN workspaces w ON w.id = r.workspace_id
LEFT JOIN namespaces ns ON ns.id = r.namespace_id
WHERE r.id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR r.workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR r.namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: CreateHostAlertRule :one
INSERT INTO host_alert_rules (
    name, description, scope, workspace_id, namespace_id,
    metric, op, threshold, for_seconds, severity, enabled, created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: UpdateHostAlertRule :execrows
UPDATE host_alert_rules
SET name = @name,
    description = @description,
    metric = @metric,
    op = @op,
    threshold = @threshold,
    for_seconds = @for_seconds,
    severity = @severity,
    enabled = @enabled,
    updated_at = now()
WHERE id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: DeleteHostAlertRule :execrows
DELETE FROM host_alert_rules
WHERE id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: ListEnabledHostAlertRules :many
-- The evaluator's read, cached in-process for ~30s per instance: rules
-- tolerate that staleness the same way scrape targets do.
SELECT id, scope, workspace_id, namespace_id, metric, op, threshold, for_seconds, severity, name
FROM host_alert_rules
WHERE enabled;

-- name: ListHostAlertStates :many
SELECT rule_id, breached_since, firing, firing_since, value
FROM host_alert_states
WHERE host_id = @host_id;

-- name: UpsertHostAlertBreach :exec
-- First observation of a breach: the pending row the debounce counts
-- from. ON CONFLICT keeps the original breached_since -- re-inserting on
-- every breaching beat would reset the clock and nothing would ever fire.
INSERT INTO host_alert_states (host_id, rule_id, breached_since, firing, value)
VALUES ($1, $2, now(), false, $3)
ON CONFLICT (host_id, rule_id) DO NOTHING;

-- name: MarkHostAlertFiring :execrows
-- The pending->firing flip, guarded on NOT firing so the caller knows a
-- row it flipped from a row somebody else (a reconnect race) already had.
UPDATE host_alert_states
SET firing = true, firing_since = now(), value = @value
WHERE host_id = @host_id AND rule_id = @rule_id AND NOT firing;

-- name: ClearHostAlertState :one
-- Recovery: the row disappears whether it was pending or firing; the
-- caller learns which it was, because only a firing one is worth an
-- event.
DELETE FROM host_alert_states
WHERE host_id = @host_id AND rule_id = @rule_id
RETURNING firing;

-- name: SweepDisabledAlertStates :many
-- The backstop for state rows whose rule was disabled: the evaluator's
-- own orphan sweep only runs on beats that evaluate at least one rule,
-- so a deployment whose LAST rule was just disabled would keep its
-- firing rows forever -- and an instance holding a stale rule cache can
-- re-create rows for up to one cache TTL after the disable. Runs on the
-- lease tick, one statement, safe to run concurrently everywhere.
DELETE FROM host_alert_states s
USING host_alert_rules r
WHERE r.id = s.rule_id AND NOT r.enabled
RETURNING s.host_id, s.firing;
