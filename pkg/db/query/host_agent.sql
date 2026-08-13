-- host_agents: the agent identity + control-channel session state.
-- Owned by pkg/apis/agentgw/store. See docs/agent/design.md §4.1 / §6.1.

-- name: UpsertHostAgent :one
-- Registration is idempotent on agent_id: the agent derives its id
-- deterministically from the machine identity, so re-running the install
-- script rebinds the SAME row instead of creating a second host. Each
-- registration bumps token_version, which revokes every previously issued
-- agent token for this machine (design §4.1 "撤销").
INSERT INTO host_agents (host_id, agent_id, version, status)
VALUES (@host_id, @agent_id, @version, 'offline')
ON CONFLICT (agent_id) DO UPDATE
SET host_id       = EXCLUDED.host_id,
    version       = EXCLUDED.version,
    token_version = host_agents.token_version + 1,
    updated_at    = now()
RETURNING *;
-- host_id = EXCLUDED.host_id (last writer wins) matters only in a race:
-- two register requests for the SAME machine, both passing the pre-check
-- (GetByAgentID finds nothing) before either commits. Without it the
-- second request's DO UPDATE would keep the FIRST request's host_id while
-- the token it hands back claims the SECOND's, so authAgent's
-- host-mismatch check would 401 the persisted (last-written) credential
-- forever. With it, the row and the last-issued token agree, so the agent
-- works; the loser's host row is left orphaned (connectivity_mode=agent,
-- no host_agents), which an operator deletes. That residual needs a
-- genuine simultaneous double-install -- the sequential retry case is
-- already clean, since the retry's GetByAgentID hits the committed row and
-- takes the UpdateFacts path -- so it does not justify serialising every
-- registration behind an advisory lock.

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
UPDATE host_agents
SET status        = 'online',
    instance_id   = @instance_id,
    version       = @version,
    connected_at  = now(),
    last_seen_at  = now(),
    clock_skew_ms = @clock_skew_ms,
    updated_at    = now()
WHERE host_id = @host_id
RETURNING connected_at;

-- name: TouchHostAgent :execrows
-- A heartbeat also RESTORES status='online'. The stale sweep can only
-- ever be a guess (it fires when nobody wrote 'offline' in time), and it
-- guesses wrong whenever DB writes are unavailable for a minute -- the
-- touches fail, the sweep then marks a perfectly live agent offline, and
-- without this the row stays offline forever while the channel is up,
-- which fails every session-token check for that host.
-- Guarded on instance_id: a touch from a stale socket must not steal a
-- row another instance has since claimed. Zero rows therefore means "the
-- row is not ours"; the caller re-claims it with MarkHostAgentOnline
-- rather than beating against a guard that can never match again.
UPDATE host_agents
SET status        = 'online',
    last_seen_at  = now(),
    clock_skew_ms = @clock_skew_ms,
    updated_at    = now()
WHERE host_id = @host_id AND instance_id = @instance_id;

-- name: MarkHostAgentOffline :exec
-- Guarded by instance_id: a stale disconnect handler must not flip a row
-- that another instance has since taken over via a fresh reconnect.
UPDATE host_agents
SET status      = 'offline',
    updated_at  = now()
WHERE host_id = @host_id AND instance_id = @instance_id;

-- name: MarkOrphanedHostAgentsOffline :exec
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
UPDATE host_agents
SET status = 'offline', updated_at = now()
WHERE status = 'online'
  AND instance_id NOT IN (
      SELECT instance_id FROM server_instances
      WHERE last_seen_at >= now() - make_interval(secs => @stale_after_secs::float8)
  );

-- name: MarkStaleHostAgentsOffline :exec
-- Backstop for the case where neither the agent nor its owning instance
-- got to write 'offline' (host powered off, instance SIGKILLed).
-- The cutoff is computed from the DB clock, not the caller's: last_seen_at
-- is written with now() here, so comparing it against a Go-side timestamp
-- would fold server/DB clock drift straight into the staleness window.
UPDATE host_agents
SET status = 'offline', updated_at = now()
WHERE status = 'online'
  AND (last_seen_at IS NULL
       OR last_seen_at < now() - make_interval(secs => @stale_after_secs::float8));
