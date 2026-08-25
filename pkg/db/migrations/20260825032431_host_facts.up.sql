-- The machine's inventory, reported by the agent on the host.facts frame.
--
-- Stored in two places on purpose, split by whether a value can be
-- QUERIED across a fleet rather than by how important it is.
--
-- Scalars go on hosts as columns: "every VMware guest", "every host still
-- on a 5.x kernel", "group by CPU model" are the questions a CMDB exists
-- to answer, and they are answered by an indexable column and by
-- list.Parse's filter tags, which read columns. In jsonb each of them
-- would need its own expression index to be anything but a table scan.
--
-- Lists go in host_facts as jsonb: nobody filters a fleet by "has a NIC
-- with MTU 9000", the shapes are per-machine, and columns would mean
-- nic1_mac / nic2_mac. One row per host, overwritten whole -- these are
-- not events and there is no history to keep here.
ALTER TABLE hosts
    ADD COLUMN virtualization       varchar(32)  NOT NULL DEFAULT '',
    ADD COLUMN cpu_model            varchar(128) NOT NULL DEFAULT '',
    ADD COLUMN cpu_sockets          integer      NOT NULL DEFAULT 0,
    ADD COLUMN cpu_cores_per_socket integer      NOT NULL DEFAULT 0,
    ADD COLUMN cpu_threads_per_core integer      NOT NULL DEFAULT 0,
    ADD COLUMN kernel_version       varchar(128) NOT NULL DEFAULT '',
    ADD COLUMN system_vendor        varchar(64)  NOT NULL DEFAULT '',
    ADD COLUMN product_name         varchar(128) NOT NULL DEFAULT '',
    ADD COLUMN bios_version         varchar(64)  NOT NULL DEFAULT '',
    ADD COLUMN serial_number        varchar(128) NOT NULL DEFAULT '',
    ADD COLUMN timezone             varchar(64)  NOT NULL DEFAULT '';

-- The one column here anyone groups a fleet by. Partial because '' is
-- every host that has not reported yet, which is a value nobody filters
-- FOR and which would otherwise be most of the index on a fresh install.
CREATE INDEX idx_hosts_virtualization ON hosts (virtualization) WHERE virtualization <> '';

-- Separate table, not a jsonb column on hosts: ListHosts is SELECT h.*,
-- so a column here would drag every host's whole hardware inventory
-- through every page of the host list to be discarded. The detail page
-- is the only reader, and it fetches one row.
CREATE TABLE host_facts (
    host_id       bigint      PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    -- [{name,mac,ipv4[],ipv6[],speedMbps,mtu,state}]
    nics          jsonb       NOT NULL DEFAULT '[]',
    -- [{mount,device,fstype,sizeBytes,usedBytes}]
    filesystems   jsonb       NOT NULL DEFAULT '[]',
    -- [{name,sizeBytes,rotational,model}]
    block_devices jsonb       NOT NULL DEFAULT '[]',
    -- When the agent last SENT these, not when they last changed: the
    -- agent stays silent while the content sits still, so this dates the
    -- report and the UI can say how old the inventory is.
    reported_at   timestamptz NOT NULL DEFAULT now()
);

-- No created_by. This is not a resource anyone creates, lists or owns --
-- it is one machine's self-description, reachable only through the host
-- it belongs to, and it has no creator to name.
