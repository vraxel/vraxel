-- Duplicate-agent detection: catch two live processes sharing one agent id.
--
-- The agent id is derived from /etc/machine-id, and everything the agent
-- persists (machine id, host id, agent token) is copied by a disk clone or a
-- golden image built from an onboarded host. Those clones then supersede each
-- other's control channel forever while jobs land on whichever one currently
-- holds it -- silently, because from the server's side each connection looks
-- like an ordinary reconnect.
--
-- The discriminator is the boot nonce: random per agent PROCESS, memory-only,
-- so it is the one identity a clone cannot inherit. Keeping the last two lets
-- the server distinguish the cases that matter:
--
--   one agent reconnecting   N, N, N        nonce never changes
--   one agent restarting     N, M, M, M     changes once, then stable
--   an agent crash-looping   N1, N2, N3     always a brand-new value
--   two clones fighting      A, B, A        a RETIRED value comes back
--
-- Only the last can make prev_boot_nonce reappear, so "incoming == prev" is a
-- proof of two live processes rather than a heuristic -- no threshold, and no
-- false positive on a flapping network or a crash loop.
ALTER TABLE host_agents
    ADD COLUMN boot_nonce      varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN prev_boot_nonce varchar(64) NOT NULL DEFAULT '',
    -- When the conflict was last observed. The gateway refuses channels for a
    -- cooling-off window after this, then lets them back in: once the clones
    -- are shut down the host recovers on its own, with no operator action and
    -- no admin UI to depend on.
    ADD COLUMN conflict_at     timestamptz;

CREATE INDEX idx_host_agents_conflict ON host_agents (conflict_at) WHERE conflict_at IS NOT NULL;
