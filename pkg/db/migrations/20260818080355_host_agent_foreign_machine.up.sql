-- Make a refused machine visible, instead of a host that just will not
-- come online.
--
-- The control channel already refuses an agent whose credential was issued
-- to a different machine (VerdictForeignMachine) -- that is the
-- deterministic half of clone handling and it is correct. But the refusal
-- was recorded ONLY in a server log line. From the operator's side the
-- host simply sits offline: no reason on the page, and no hint that the
-- recovery is to re-run the install command on that machine. Observed in
-- practice as eleven minutes of silent reconnect-refuse before anyone
-- thought to read the log.
--
-- A log line is the wrong place for it. The condition is a property of the
-- host row -- it persists, it has a fix, and the person who needs it is
-- looking at the host page, not at stdout on the API server.
ALTER TABLE host_agents
    -- When a machine last presented this host's credential and was refused
    -- for not being the machine it was issued to. NULL is the ordinary
    -- state; the UI says nothing.
    ADD COLUMN foreign_machine_at   timestamptz,
    -- The SMBIOS UUID of the machine that was refused. Kept because the
    -- operator's real question is "which machine is this", and the row's
    -- own product_uuid only answers the other half. Empty when the caller
    -- reported no usable UUID.
    ADD COLUMN foreign_machine_uuid varchar(64) NOT NULL DEFAULT '';
