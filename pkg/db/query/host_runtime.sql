-- Runtime inventory: what a host runs (host_processes) and who can use
-- it (host_accounts). See host_facts.sql for the hardware half.

-- name: UpsertHostProcesses :exec
INSERT INTO host_processes (host_id, groups, reported_at)
VALUES (@host_id, @groups, now())
ON CONFLICT (host_id) DO UPDATE SET
    groups      = EXCLUDED.groups,
    reported_at = EXCLUDED.reported_at;

-- name: UpsertHostAccounts :exec
INSERT INTO host_accounts (host_id, users, groups, sudo_rules, reported_at)
VALUES (@host_id, @users, @groups, @sudo_rules, now())
ON CONFLICT (host_id) DO UPDATE SET
    users       = EXCLUDED.users,
    groups      = EXCLUDED.groups,
    sudo_rules  = EXCLUDED.sudo_rules,
    reported_at = EXCLUDED.reported_at;

-- name: GetHostProcesses :one
-- Joined through hosts rather than read straight from host_processes, so
-- the caller's tenancy filter applies to the same row the host list
-- applies it to. Reading the child table alone would serve one host's
-- workload to anybody who could guess its id.
SELECT p.groups, p.reported_at
FROM host_processes p
JOIN hosts h ON h.id = p.host_id
WHERE p.host_id = @host_id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR h.workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR h.namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: GetHostAccounts :one
SELECT a.users, a.groups, a.sudo_rules, a.reported_at
FROM host_accounts a
JOIN hosts h ON h.id = a.host_id
WHERE a.host_id = @host_id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR h.workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR h.namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);
