-- Machine identity: stop deriving it from the one signal a clone copies.
--
-- agent_id was UUIDv5(/etc/machine-id), which makes a cloned disk arrive as
-- the SAME agent -- the two machines then converge on one host row and
-- overwrite each other's facts. Measured on two VMware clones:
--
--     machine-id      af96255a...   af96255a...   IDENTICAL
--     root fs UUID    d524ccc5...   d524ccc5...   IDENTICAL
--     product_uuid    8c1e4d56...   dda44d56...   different
--     MAC             ...b8:93:e2   ...b7:11:77   different
--
-- The pattern is not "some signals disagree". It is that the signals split
-- cleanly by WHERE THEY LIVE:
--
--   A class -- assigned from outside the disk image (product_uuid, MACs).
--              A hypervisor re-issues these when it copies a VM, so they
--              tell two machines apart.
--   B class -- carried inside the disk image (machine-id, root fs UUID).
--              A copy inherits them, so they can only say "same image".
--
-- Scoring the two classes together would be worse than useless: B-class
-- signals are perfectly correlated with each other (they are all just "the
-- disk"), so counting them as independent votes lets a clone outvote the
-- hypervisor. Hence: A class claims a row, B class only groups.
--
-- These columns hold the fingerprint the machine last presented, so
-- matching lives in server code rather than in a hash the agent computed.
-- The rule can then change without re-onboarding a fleet -- which is the
-- property the old derivation lacked, and the reason it could not be fixed
-- in place.
ALTER TABLE host_agents
    -- A class. Empty when the machine has no usable SMBIOS UUID (junk
    -- vendor values are rejected before they land here); such a host can
    -- never be auto-claimed, only merged by an operator.
    ADD COLUMN product_uuid    varchar(64) NOT NULL DEFAULT '',
    -- A-class corroboration, and the evidence an operator reads when
    -- deciding a merge. Never a claim key on its own: bonds, USB NICs and
    -- randomised MACs all move without the machine changing.
    ADD COLUMN macs            text[]      NOT NULL DEFAULT '{}',
    -- B class. Groups clones of one image so the operator can be told to
    -- fix the template, and is what makes "same image, different machine"
    -- a positive finding instead of an ambiguity.
    ADD COLUMN machine_id      varchar(64) NOT NULL DEFAULT '',
    -- Which class claimed this row, so a later match knows whether it is
    -- comparing like with like. '' for rows that predate this.
    ADD COLUMN identity_source varchar(16) NOT NULL DEFAULT '',
    -- Server-computed: now() - uptime, never the machine's own clock.
    --
    -- This is the time evidence that keeps the common case out of the
    -- operator's lap. A machine whose motherboard was replaced cannot have
    -- been running before the old one's last heartbeat, so an overlap is
    -- proof of two machines -- and every VM cloned from a live host has
    -- one. Suspend/resume only pushes boot_at later, so the test misses
    -- clones rather than inventing them.
    ADD COLUMN boot_at         timestamptz;

-- Claim lookup. Partial: the empty string is "no usable A-class signal",
-- which must never collide with another such host.
CREATE INDEX idx_host_agents_product_uuid ON host_agents (product_uuid) WHERE product_uuid <> '';

-- Image grouping, for the clone/template findings on the host list.
CREATE INDEX idx_host_agents_machine_id ON host_agents (machine_id) WHERE machine_id <> '';
