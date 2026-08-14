-- hosts rows for machines that onboarded themselves through the agent.
-- Owned by the agent host store; the gateway reaches it via HostRegistrar.

-- name: CreateAgentHost :one
-- Distinct from a normal host create because the agent path sets
-- connectivity_mode + reported_primary_ip and never touches cloud-origin
-- columns. created_by is inherited from the join token's creator -- the
-- operator who authorised the onboarding is the traceable human, since
-- /register carries no user session.
--
-- origin='agent' records how the row came into existence and never
-- changes afterwards; connectivity_mode records how the control plane
-- reaches the host today and does change (an imported host that later
-- installs an agent flips to 'agent'). Conflating the two is what made
-- the old rollback guard below unsound.
INSERT INTO hosts (
    name, display_name, hostname, os, arch,
    cpu_cores, memory_mb, disk_gb, scope,
    workspace_id, namespace_id, status, ssh_port, agent_port,
    origin, connectivity_mode, reported_primary_ip, created_by
)
VALUES (
    @name, @display_name, @hostname, @os, @arch,
    @cpu_cores, @memory_mb, @disk_gb, @scope,
    @workspace_id, @namespace_id, 'running', 22, 9100,
    'agent', 'agent', @reported_primary_ip, sqlc.narg('created_by')
)
RETURNING id;

-- name: UpdateAgentHostFacts :execrows
-- Re-registration of a known agent: refresh the reported hardware facts
-- and the default-route IP, leave name / scope alone (an operator may have
-- renamed or re-scoped the host after onboarding).
UPDATE hosts
SET hostname            = @hostname,
    os                  = @os,
    arch                = @arch,
    cpu_cores           = @cpu_cores,
    memory_mb           = @memory_mb,
    disk_gb             = @disk_gb,
    reported_primary_ip = @reported_primary_ip,
    connectivity_mode   = 'agent',
    updated_at          = now()
WHERE id = @id;

-- name: GetAgentHostScope :one
-- Tenancy of an existing agent host, read before a re-registration is
-- allowed to rebind it. The agent id in a register request is derived
-- from the machine id the caller reports, which is not a secret, so the
-- scope of the join token presented has to be checked against the scope
-- of the host being claimed.
SELECT scope, workspace_id, namespace_id FROM hosts WHERE id = @id;

-- name: DeleteAgentHost :execrows
-- Roll back a host row created by a registration that then failed.
--
-- The guard is "no agent is bound to this host", not the old
-- connectivity_mode='agent'. connectivity_mode is mutable: an operator
-- imports a host, installs the agent on it later, UpdateAgentHostFacts
-- flips the column, and from then on the old guard would have let a
-- failed registration delete a hand-created row along with everything
-- hanging off it. Whether an agent is bound is the fact that actually
-- separates "this request just made this row" from "this row was here
-- first" -- the rollback only ever runs before host_agents is written,
-- or after that write failed.
--
-- The caller applies the same rule in-process (register.go passes the
-- create/attach outcome). This is the second lock on the same door,
-- because the thing behind it is an unrecoverable delete.
DELETE FROM hosts h
WHERE h.id = @id
  AND NOT EXISTS (SELECT 1 FROM host_agents ha WHERE ha.host_id = h.id);
