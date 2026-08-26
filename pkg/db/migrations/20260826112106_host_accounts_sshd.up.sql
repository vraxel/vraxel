-- The ssh daemon's effective configuration, beside the accounts it
-- decides between.
--
-- On host_accounts because it is the other half of one question. The user
-- list says root has a password; this says whether the daemon accepts
-- passwords and permits root. Neither is a finding alone, and reading
-- them from two places at two ages would let them disagree about the
-- machine they describe.
--
-- {ports,permitRootLogin,passwordAuth,...}. An object and not columns:
-- nobody filters a fleet by maxAuthTries, and the shape follows what
-- sshd -T reports rather than a schema this platform chose.
ALTER TABLE host_accounts
    ADD COLUMN sshd jsonb NOT NULL DEFAULT '{}';
