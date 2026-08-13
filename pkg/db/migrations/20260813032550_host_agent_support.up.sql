-- Agent-managed internal hosts: the data model backing the agent gateway.
--
-- vraxel manages internal hosts through an outbound agent (the control
-- plane cannot dial into them). This migration creates the host record and
-- the three agent tables register + the control channel need. The playbook
-- run / job / lock tables come with the jobs slice later.

-- ---------------------------------------------------------------------------
-- 1. hosts: the managed host record (cloud/PaaS columns intentionally absent)
-- ---------------------------------------------------------------------------
CREATE TABLE hosts (
    id                  bigserial    PRIMARY KEY,
    name                varchar(255) NOT NULL,
    display_name        varchar(255) NOT NULL DEFAULT '',
    description         text         NOT NULL DEFAULT '',
    hostname            varchar(255) NOT NULL DEFAULT '',
    os                  varchar(100) NOT NULL DEFAULT '',
    arch                varchar(50)  NOT NULL DEFAULT '',
    cpu_cores           integer      NOT NULL DEFAULT 0,
    memory_mb           bigint       NOT NULL DEFAULT 0,
    disk_gb             bigint       NOT NULL DEFAULT 0,
    scope               varchar(20)  NOT NULL,
    workspace_id        bigint,
    namespace_id        bigint,
    status              varchar(20)  NOT NULL DEFAULT 'active',
    status_message      text         NOT NULL DEFAULT '',
    ssh_port            integer      NOT NULL DEFAULT 22,
    agent_port          integer      NOT NULL DEFAULT 9100,
    monitor_status      varchar(20)  NOT NULL DEFAULT 'uninstalled',
    monitor_message     text         NOT NULL DEFAULT '',
    log_agent_status    varchar(20)  NOT NULL DEFAULT 'uninstalled',
    log_agent_message   text         NOT NULL DEFAULT '',
    origin              varchar(32)  NOT NULL DEFAULT 'manual',
    -- 'ssh': control plane dials directly; 'agent': host dials out via the agent
    connectivity_mode   varchar(16)  NOT NULL DEFAULT 'ssh',
    -- reported by the agent; display / log labels only, never dialled from here
    reported_ips        text[]       NOT NULL DEFAULT '{}',
    reported_primary_ip varchar(45)  NOT NULL DEFAULT '',
    primary_ip_override varchar(45)  NOT NULL DEFAULT '',
    -- CLAUDE.md: any listable resource shows its creator
    created_by          bigint       REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz  NOT NULL DEFAULT now(),
    updated_at          timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT chk_host_scope CHECK (
        (scope = 'platform'  AND workspace_id IS NULL     AND namespace_id IS NULL)
     OR (scope = 'workspace' AND workspace_id IS NOT NULL AND namespace_id IS NULL)
     OR (scope = 'namespace' AND namespace_id IS NOT NULL)
    ),
    CONSTRAINT chk_hosts_connectivity_mode CHECK (connectivity_mode IN ('ssh', 'agent'))
);

-- Host name is unique per scope; two customer networks can each have a
-- "node-1", so the agent registrar disambiguates on collision.
CREATE UNIQUE INDEX uk_hosts_platform  ON hosts (name)               WHERE scope = 'platform';
CREATE UNIQUE INDEX uk_hosts_workspace ON hosts (name, workspace_id) WHERE scope = 'workspace';
CREATE UNIQUE INDEX uk_hosts_namespace ON hosts (name, namespace_id) WHERE scope = 'namespace';
CREATE INDEX idx_hosts_namespace_id ON hosts (namespace_id) WHERE namespace_id IS NOT NULL;
CREATE INDEX idx_hosts_created_by   ON hosts (created_by);

-- ---------------------------------------------------------------------------
-- 2. server_instances: instance registry for cross-instance addressing
-- ---------------------------------------------------------------------------
-- An agent's control channel is a stateful long-lived connection pinned to
-- one server instance, while API requests land on any instance behind the
-- LB. Instance B resolves a host's owning instance via host_agents.instance_id
-- and this table's internal_addr to forward. Also the liveness oracle for
-- orphaned-run reclaim.
CREATE TABLE server_instances (
    instance_id   varchar(64)  PRIMARY KEY,
    internal_addr varchar(255) NOT NULL,
    started_at    timestamptz  NOT NULL DEFAULT now(),
    last_seen_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX idx_server_instances_last_seen ON server_instances (last_seen_at);

-- ---------------------------------------------------------------------------
-- 3. host_agent_join_tokens: one-time registration tokens
-- ---------------------------------------------------------------------------
-- The control plane cannot connect into a host, so onboarding is host-driven:
--   curl .../install-agent.sh | sh -s -- --server <url> --token <join-token>
-- The agent trades the join token for a long-lived agent token.
CREATE TABLE host_agent_join_tokens (
    id           bigserial    PRIMARY KEY,
    name         varchar(255) NOT NULL DEFAULT '',
    -- only the hash is stored; the plaintext is returned once at creation
    token_hash   bytea        NOT NULL UNIQUE,
    scope        varchar(20)  NOT NULL,
    workspace_id bigint       REFERENCES workspaces(id) ON DELETE CASCADE,
    namespace_id bigint       REFERENCES namespaces(id) ON DELETE CASCADE,
    max_uses     int          NOT NULL DEFAULT 1,
    used_count   int          NOT NULL DEFAULT 0,
    expires_at   timestamptz  NOT NULL,
    created_by   bigint       REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT chk_join_token_scope CHECK (
        (scope = 'platform'  AND workspace_id IS NULL     AND namespace_id IS NULL)
     OR (scope = 'workspace' AND workspace_id IS NOT NULL AND namespace_id IS NULL)
     OR (scope = 'namespace' AND namespace_id IS NOT NULL)
    ),
    CONSTRAINT chk_join_token_uses CHECK (max_uses > 0 AND used_count >= 0 AND used_count <= max_uses)
);

CREATE INDEX idx_host_agent_join_tokens_expires ON host_agent_join_tokens (expires_at);

-- ---------------------------------------------------------------------------
-- 4. host_agents: agent identity and control-channel session
-- ---------------------------------------------------------------------------
CREATE TABLE host_agents (
    host_id       bigint      PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    agent_id      uuid        NOT NULL UNIQUE,
    -- bumping revokes every agent token issued before
    token_version int         NOT NULL DEFAULT 1,
    version       varchar(32) NOT NULL DEFAULT '',
    -- the server instance currently holding the control channel
    instance_id   varchar(64) NOT NULL DEFAULT '',
    status        varchar(16) NOT NULL DEFAULT 'offline',
    connected_at  timestamptz,
    last_seen_at  timestamptz,
    -- host-vs-server clock offset carried on the heartbeat
    clock_skew_ms bigint      NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_host_agents_status CHECK (status IN ('online', 'offline'))
);

CREATE INDEX idx_host_agents_instance  ON host_agents (instance_id) WHERE status = 'online';
CREATE INDEX idx_host_agents_last_seen ON host_agents (last_seen_at);
