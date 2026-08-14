-- host_agents: the agent identity + control-channel session state.
-- Owned by pkg/apis/agentgw/store. See docs/agent/design.md §4.1 / §6.1.

-- name: FindHostAgentsByProductUUID :many
-- Candidate rows for a machine claiming an SMBIOS UUID. Takes an array
-- because one machine can spell its own UUID two ways (SMBIOS 2.6 byte
-- order -- see sameProductUUID); the caller passes both spellings.
--
-- :many, not :one, because the value is not guaranteed unique in the
-- wild: whitebox firmware ships batches with an identical UUID. Two rows
-- back means this value identifies a production run rather than a
-- machine, and the caller must refuse to claim on it. A UNIQUE index
-- would instead refuse to ONBOARD such a batch at all.
SELECT * FROM host_agents
WHERE product_uuid <> '' AND product_uuid = ANY(@product_uuids::text[])
ORDER BY host_id;

-- name: FindHostAgentsByMachineID :many
-- Everything built from one disk image. Not an identity lookup: this
-- answers "which hosts came from the same template", which is what turns
-- a clone from an ambiguity into a finding an operator can act on.
SELECT * FROM host_agents
WHERE machine_id <> '' AND machine_id = @machine_id
ORDER BY host_id;

-- name: RebindHostAgent :one
-- A machine that already has a row, registering again: keep the agent_id,
-- move it if the host changed, refresh the fingerprint, and bump
-- token_version to revoke every token issued before now (design §4.1
-- "撤销").
--
-- agent_id survives on purpose. It is allocated once and never derived,
-- so a credential keeps naming the same row even when the row's host_id
-- changes underneath it -- which is what lets an operator merge two hosts
-- without knocking the agent off.
UPDATE host_agents
SET host_id         = @host_id,
    version         = @version,
    token_version   = token_version + 1,
    product_uuid    = @product_uuid,
    machine_id      = @machine_id,
    macs            = @macs,
    identity_source = @identity_source,
    boot_at         = @boot_at,
    updated_at      = now()
WHERE agent_id = @agent_id
RETURNING *;

-- name: InsertHostAgent :one
-- A machine with no row yet. agent_id is supplied by the caller as fresh
-- randomness rather than derived from anything the machine reports.
INSERT INTO host_agents (host_id, agent_id, version, status,
                         product_uuid, machine_id, macs, identity_source, boot_at)
VALUES (@host_id, @agent_id, @version, 'offline',
        @product_uuid, @machine_id, @macs, @identity_source, @boot_at)
RETURNING *;

-- name: DeleteHostAgentByHostID :execrows
-- Detaches whatever agent currently holds a host. Used when a join token
-- bound to that host is redeemed by a DIFFERENT machine: host_agents is
-- keyed by host_id, one host has one agent, and the operator minting a
-- bound token for a host that already had one is saying "this machine is
-- that host now".
DELETE FROM host_agents WHERE host_id = @host_id;

-- name: UpdateHostAgentFingerprint :execrows
-- Refreshes the mutable half of the fingerprint on reconnect, for the one
-- benign way it changes: the machine kept its hardware identity but reset
-- /etc/machine-id, which is precisely what we tell the operator of a
-- cloned host to do. Clearing conflict_at is part of the same statement
-- because that reset is the fix for the conflict -- leaving the flag set
-- would keep refusing the machine that just complied.
UPDATE host_agents
SET machine_id  = @machine_id,
    macs        = @macs,
    boot_at     = @boot_at,
    conflict_at = NULL,
    updated_at  = now()
WHERE host_id = @host_id;

-- name: GetHostAgentByAgentID :one
SELECT * FROM host_agents WHERE agent_id = @agent_id;

-- name: GetHostAgentByHostID :one
SELECT * FROM host_agents WHERE host_id = @host_id;

-- name: CheckHostAgentIdentity :one
-- Rolls this connection's boot nonce into the row's two-slot history and
-- reports whether the agent id is currently contended.
--
-- Runs BEFORE the session is registered, because a contended id must not
-- reach MarkHostAgentOnline (which would mark the host online under a
-- channel we are about to close) or the run reconciler (which fails every
-- in-flight job the hello did not list).
--
-- Every expression on the right reads the OLD row, which is what makes the
-- three rules fit in one statement:
--   * prev only moves when the nonce actually changed, so an agent
--     reconnecting with the same value never pollutes its own history;
--   * a conflict is "the value we just retired is back", which only two
--     live processes can produce -- a crash loop emits nothing but fresh
--     values and can never match;
--   * an empty nonce (an agent older than this field) is inert: it neither
--     shifts the history nor raises a conflict.
-- RETURNING reads the NEW row, so the connection that trips the conflict is
-- itself refused rather than being the one admitted.
UPDATE host_agents
SET prev_boot_nonce = CASE WHEN @boot_nonce::text <> boot_nonce
                           THEN boot_nonce ELSE prev_boot_nonce END,
    boot_nonce      = @boot_nonce,
    conflict_at     = CASE WHEN @boot_nonce::text <> ''
                            AND @boot_nonce::text = prev_boot_nonce
                           THEN now() ELSE conflict_at END,
    updated_at      = now()
WHERE host_id = @host_id
RETURNING conflict_at IS NOT NULL
      AND conflict_at > now() - make_interval(secs => @cooldown_secs::float8) AS contended;

-- name: MarkHostAgentOnline :one
-- Called when a control channel is accepted. instance_id records which
-- the server instance holds the socket (cross-instance addressing, §6.2).
-- RETURNING connected_at gives the caller the exact timestamp this
-- connection was recorded with: it becomes the session token's connEpoch
-- (design §4.1), so a reconnect (which rewrites connected_at) invalidates
-- every session token minted for the previous connection. Signing with a
-- value read back separately would race a concurrent reconnect; returning
-- it from the same statement is the only value that provably matches.
--
-- conflict_at is cleared here, and only here, on the admit path. The
-- gateway refuses a contended identity for a cooldown window, so reaching
-- this statement means the window lapsed and the session that got through
-- was clean. Without the clear the column is write-only: the badge it
-- drives outranks online/offline, so a host that resolved its conflict
-- months ago would still be showing it.
UPDATE host_agents
SET status        = 'online',
    instance_id   = @instance_id,
    version       = @version,
    connected_at  = now(),
    last_seen_at  = now(),
    clock_skew_ms = @clock_skew_ms,
    conflict_at   = NULL,
    updated_at    = now()
WHERE host_id = @host_id
RETURNING connected_at;

-- name: TouchHostAgent :execrows
-- The pure heartbeat: it moves last_seen_at and nothing else.
--
-- Guarded on instance_id AND status, so zero rows means "this row is not
-- ours, or it is not marked online" -- and both are repaired the same
-- way, by re-claiming it with MarkHostAgentOnline. The second half of
-- that guard is what the stale sweep needs: the sweep can only ever be a
-- guess (it fires when nobody wrote 'offline' in time) and it guesses
-- wrong whenever DB writes are unavailable for a minute, so a live
-- channel has to be able to heal a row that was marked offline under it.
--
-- Restoring status here instead (SET status='online', no guard) would
-- also heal it, but then every beat -- one per agent per 15s -- would
-- look identical to a real online transition, and the watch stream has
-- no way to tell them apart. Leaving the transition to
-- MarkHostAgentOnline keeps the hot path silent and makes every status
-- write an actual change.
UPDATE host_agents
SET last_seen_at  = now(),
    clock_skew_ms = @clock_skew_ms,
    updated_at    = now()
WHERE host_id = @host_id AND instance_id = @instance_id AND status = 'online';

-- name: MarkHostAgentOffline :execrows
-- Guarded by instance_id: a stale disconnect handler must not flip a row
-- that another instance has since taken over via a fresh reconnect.
--
-- The status guard makes the affected-row count mean "the agent just went
-- offline" rather than "the statement ran", which is what the caller
-- announces on the watch channel.
UPDATE host_agents
SET status      = 'offline',
    updated_at  = now()
WHERE host_id = @host_id AND instance_id = @instance_id AND status <> 'offline';

-- name: MarkOrphanedHostAgentsOffline :many
-- Startup residue cleanup: rows still claiming 'online' under an instance
-- that no longer holds a lease. That covers this process's own previous
-- life, which is the common case after a hard kill.
--
-- Matching on the dead instance rather than on our own id is what makes
-- it work at all: instance_id carries the pid, so a restarted process
-- never shares an id with its predecessor and a self-targeted cleanup
-- could only ever match zero rows.
--
-- Live siblings are excluded by the lease join, so this is safe to run on
-- every boot in a horizontally scaled deployment.
--
-- RETURNING names the hosts that actually changed, which is the set the
-- watch channel announces. The status guard above is what keeps that set
-- to real transitions.
UPDATE host_agents
SET status = 'offline', updated_at = now()
WHERE status = 'online'
  AND instance_id NOT IN (
      SELECT instance_id FROM server_instances
      WHERE last_seen_at >= now() - make_interval(secs => @stale_after_secs::float8)
  )
RETURNING host_id;

-- name: MarkStaleHostAgentsOffline :many
-- Backstop for the case where neither the agent nor its owning instance
-- got to write 'offline' (host powered off, instance SIGKILLed).
-- The cutoff is computed from the DB clock, not the caller's: last_seen_at
-- is written with now() here, so comparing it against a Go-side timestamp
-- would fold server/DB clock drift straight into the staleness window.
--
-- RETURNING names the hosts that actually changed, for the watch channel.
UPDATE host_agents
SET status = 'offline', updated_at = now()
WHERE status = 'online'
  AND (last_seen_at IS NULL
       OR last_seen_at < now() - make_interval(secs => @stale_after_secs::float8))
RETURNING host_id;

-- name: MoveHostAgentBinding :one
-- Re-points a live agent at a different host, for a merge: two rows turn
-- out to be one machine, and the surviving row inherits the machine.
--
-- Only the host_id moves. agent_id, token_version and the credential
-- built from them stay put, which is what lets the agent keep running
-- across a merge -- it authenticates as an agent, and the host it belongs
-- to is looked up rather than carried in the token.
UPDATE host_agents
SET host_id    = @to_host_id,
    updated_at = now()
WHERE host_id = @from_host_id
RETURNING *;
