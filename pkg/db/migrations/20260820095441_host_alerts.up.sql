-- Threshold alerting over the heartbeat snapshots (design §8): rules are
-- what an operator writes, states are what the evaluator maintains.
--
-- Evaluation happens server-side, in the agent gateway's heartbeat path,
-- NOT on the agent: the numbers are already here, rules become editable
-- without touching a fleet that has no self-upgrade, and multi-instance
-- needs no locking -- one host's beats all arrive at whichever instance
-- holds its control channel, so that instance is the sole writer of the
-- host's states.

CREATE TABLE host_alert_rules (
    id           bigserial PRIMARY KEY,
    name         varchar(255) NOT NULL,
    description  varchar(1000) NOT NULL DEFAULT '',
    -- Tenancy, host-style: a platform rule sees every host, a workspace
    -- rule its workspace's hosts, a namespace rule its project's.
    scope        varchar(32) NOT NULL,
    workspace_id bigint REFERENCES workspaces(id) ON DELETE CASCADE,
    namespace_id bigint REFERENCES namespaces(id) ON DELETE CASCADE,
    -- One of the heartbeat summary's numeric fields (cpu_used_pct,
    -- mem_used_pct, disk_used_pct, load1/5/15, net_rx_bps, net_tx_bps),
    -- validated in the handler; the evaluator reads by this key.
    metric       varchar(32) NOT NULL,
    -- gt / ge / lt / le.
    op           varchar(2) NOT NULL,
    threshold    real NOT NULL,
    -- The debounce: the breach must hold this long (wall clock, spanning
    -- however many beats that is) before the alert fires. The same
    -- threshold-with-hysteresis-in-time idea as the probe engine's
    -- failureThreshold, expressed in seconds because beats can be missed.
    for_seconds  int NOT NULL DEFAULT 60,
    severity     varchar(16) NOT NULL DEFAULT 'warning',
    enabled      boolean NOT NULL DEFAULT true,
    created_by   bigint REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- One row per (host, rule) that is currently breaching -- pending or
-- firing. Recovery DELETEs the row, so the table only ever holds live
-- problems and its size is bounded by them. Nothing writes here on a
-- healthy beat; the notify-per-beat trap does not exist on this path.
CREATE TABLE host_alert_states (
    host_id        bigint NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    rule_id        bigint NOT NULL REFERENCES host_alert_rules(id) ON DELETE CASCADE,
    -- When the breach was first observed; firing flips once now() -
    -- breached_since clears the rule's for_seconds.
    breached_since timestamptz NOT NULL,
    firing         boolean NOT NULL DEFAULT false,
    firing_since   timestamptz,
    -- The observation that flipped it, kept for the operator ("fired at
    -- 96%"); the CURRENT value lives in host_metrics_latest.
    value          real NOT NULL,
    PRIMARY KEY (host_id, rule_id)
);
