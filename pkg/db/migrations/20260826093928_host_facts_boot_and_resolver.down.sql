ALTER TABLE host_facts
    DROP COLUMN IF EXISTS ssh_host_keys,
    DROP COLUMN IF EXISTS cpu_mitigations,
    DROP COLUMN IF EXISTS dns,
    DROP COLUMN IF EXISTS swaps;

ALTER TABLE hosts
    DROP COLUMN IF EXISTS clock_sync,
    DROP COLUMN IF EXISTS kernel_cmdline;
