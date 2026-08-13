-- hosts rows for machines that onboarded themselves through the agent.
-- Owned by the agent host store; the gateway reaches it via HostRegistrar.

-- name: CreateAgentHost :one
-- Distinct from a normal host create because the agent path sets
-- connectivity_mode + reported_primary_ip and never touches cloud-origin
-- columns. created_by is inherited from the join token's creator -- the
-- operator who authorised the onboarding is the traceable human, since
-- /register carries no user session.
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
    'manual', 'agent', @reported_primary_ip, sqlc.narg('created_by')
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
-- Roll back a host row created by a registration that then failed. Bound
-- to connectivity_mode='agent' so this can never remove a host that was
-- onboarded any other way.
DELETE FROM hosts WHERE id = @id AND connectivity_mode = 'agent';
