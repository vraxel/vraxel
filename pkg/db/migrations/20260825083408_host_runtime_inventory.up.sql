-- What each host is RUNNING, and who can use it. The runtime half of the
-- inventory; host_facts is the other half and holds what a host IS.
--
-- Two tables rather than two more columns on host_facts, because the
-- three reports have three cadences (hardware hourly, accounts hourly,
-- workloads every five minutes) and each needs its own reported_at.
-- Sharing a row would mean one write clobbering another's timestamp, and
-- a single UPSERT that has to carry all three payloads or blank the ones
-- it was not sent.
--
-- One row per host, overwritten whole. Snapshots, not history: "who has
-- root on this box" is answered by the current state, and the question
-- these could also answer -- "when did that change" -- needs change
-- detection, a retention policy and a cleaner, which is a separate piece
-- of work and not one this schema forecloses.
--
-- jsonb rather than a row per process or per account. A row per account
-- across a fleet would make sense for "find every host where jenkins can
-- log in", and there is no such query yet; until there is, normalised
-- rows would be a table nothing reads, indexed for nobody. The detail
-- page fetches one host's blob and pages it in memory, which is honest
-- for the 29 accounts and 28 workloads a real machine reports.
CREATE TABLE host_processes (
    host_id     bigint      PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    -- [{name,user,count,unit,container,ports:[{proto,addr,port}]}]
    groups      jsonb       NOT NULL DEFAULT '[]',
    -- When the agent last SENT this, not when it last changed: the agent
    -- stays silent while the workload sits still.
    reported_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE host_accounts (
    host_id     bigint      PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    -- [{name,uid,gid,group,home,shell,canLogin,password,groups,privileges,sshKeys}]
    --
    -- password is one of set / locked / disabled / empty -- the SHAPE of
    -- the shadow field. The hash is read on the host to derive that word
    -- and is never sent, so it cannot be here. sshKeys carry a type, a
    -- SHA256 fingerprint and a comment, never a key body.
    users       jsonb       NOT NULL DEFAULT '[]',
    -- [{name,gid,members}]
    groups      jsonb       NOT NULL DEFAULT '[]',
    -- The verbatim grant lines of /etc/sudoers and its includes. Text,
    -- because sudo's grammar is real and a parser that half-understands
    -- it produces confident wrong answers about who can become root.
    sudo_rules  jsonb       NOT NULL DEFAULT '[]',
    reported_at timestamptz NOT NULL DEFAULT now()
);

-- No created_by on either. Neither is a resource anyone creates, lists or
-- owns -- both are one machine's self-description, reachable only through
-- the host they belong to, and neither has a creator to name.
