-- A join token can name the host its agent will attach to.
--
-- Without this, every registration takes the create branch and a host
-- imported by hand gets a second, duplicate row the first time someone
-- installs an agent on it. Binding an id rather than reserving a name:
-- the row already exists in every case that needs this, so an id says
-- exactly what a name-shaped placeholder only approximates.
--
-- ON DELETE CASCADE, not SET NULL: a token bound to a deleted host must
-- not silently turn back into a token that creates a new one.
ALTER TABLE host_agent_join_tokens
    ADD COLUMN target_host_id bigint REFERENCES hosts(id) ON DELETE CASCADE;

-- One host, one machine. A bound token redeemable twice would let a
-- second machine take over the first one's row.
ALTER TABLE host_agent_join_tokens
    ADD CONSTRAINT chk_join_token_bound_single_use
    CHECK (target_host_id IS NULL OR max_uses = 1);

CREATE INDEX idx_host_agent_join_tokens_target
    ON host_agent_join_tokens (target_host_id)
    WHERE target_host_id IS NOT NULL;
