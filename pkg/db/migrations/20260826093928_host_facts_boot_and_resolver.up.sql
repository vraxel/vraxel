-- Six more things a machine can answer about itself, split by the rule
-- the columns beside them already follow: single-valued and comparable
-- across a fleet becomes a column, per-machine shapes become jsonb.
--
-- None of these needed a new report or a new endpoint. They ride the
-- host.facts frame that already exists, and every one of them sits still
-- between reboots -- which is what lets the agent compare a sample
-- against the last one it sent and stay silent. Usage figures were
-- deliberately left out of the swap entry for exactly that reason.
ALTER TABLE hosts
    -- What the bootloader passed the kernel. Changed only by editing the
    -- bootloader and rebooting, and where a fleet's exceptions are
    -- actually written down: mitigations=off, hugepages, an IO scheduler
    -- override. A machine that behaves unlike its neighbours usually
    -- differs here first.
    --
    -- 1024 because a Kubernetes node's line runs several hundred
    -- characters once the container runtime and cgroup flags are on it.
    ADD COLUMN kernel_cmdline varchar(1024) NOT NULL DEFAULT '',
    -- 'synced', 'unsynced', or '' for an agent that could not ask.
    -- A column and not part of some health blob: a wrong clock is
    -- invisible in every other reading and corrupts all of them, this
    -- platform's own staleness judgement included, and "show me every
    -- host whose clock has drifted" is a fleet query.
    ADD COLUMN clock_sync varchar(16) NOT NULL DEFAULT '';

-- No index on either. Nothing filters on them yet, and an index with no
-- query behind it is a write cost with nothing on the other side; it
-- goes in with the filter, the way virtualization's did.

ALTER TABLE host_facts
    -- [{device,kind,sizeBytes,priority}] -- configuration only. How much
    -- swap is IN USE belongs to the metrics stream: it changes every
    -- sample, and this report is sent only when its content changes.
    ADD COLUMN swaps jsonb NOT NULL DEFAULT '[]',
    -- {servers:[],search:[]} -- one object rather than two columns
    -- because neither half means anything without the other: an address
    -- resolves through the servers, and a short name reaches them only
    -- after the search list has finished with it.
    ADD COLUMN dns jsonb NOT NULL DEFAULT '{}',
    -- [{name,status}] from /sys/devices/system/cpu/vulnerabilities,
    -- verbatim. The statuses are not an enum: "Mitigation: PTE
    -- Inversion", "Not affected", "Vulnerable" and "Unknown: Dependent
    -- on hypervisor status" are four different situations, and folding
    -- them into a boolean would turn "we cannot tell from inside this
    -- guest" into a claim.
    ADD COLUMN cpu_mitigations jsonb NOT NULL DEFAULT '[]',
    -- [{type,fingerprint}] -- what an ssh client pins. Public halves
    -- only, reduced to a SHA256 fingerprint on the host before it was
    -- sent, the same treatment authorized_keys already gets. A machine
    -- rebuilt or replaced has new ones, which is precisely the event
    -- that makes every operator's client refuse to connect.
    ADD COLUMN ssh_host_keys jsonb NOT NULL DEFAULT '[]';
