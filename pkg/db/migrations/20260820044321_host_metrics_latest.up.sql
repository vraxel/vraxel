-- The latest utilisation snapshot per host, written by the agent gateway
-- from every heartbeat that carries one (~15s cadence).
--
-- One row per host, overwritten in place -- this is a display cache, NOT
-- a time series, and it must never become one: history lives in the
-- agent's ring buffer (read on demand over the data channel) or, on the
-- full tier, in VictoriaMetrics. Nothing here needs retention sweeps,
-- and the row count is bounded by the host count.
--
-- Writers must NOT publish a host event for these updates: at one beat
-- per host per 15 seconds, notifying watchers would flood every open
-- page forever (the foreign-machine retry loop made exactly this
-- mistake). Staleness is derived by readers from sampled_at, so no
-- sweeper marks rows stale either.
--
-- sampled_at is the AGENT's clock at sampling time, on purpose: a host
-- with a broken clock then reads as stale rather than as fresh numbers
-- that are silently wrong.
--
-- fillfactor leaves room for HOT updates: every column of every row
-- churns every 15s, and none of them is indexed.
CREATE TABLE host_metrics_latest (
    host_id        bigint PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    sampled_at     timestamptz NOT NULL,
    cpu_used_pct   real NOT NULL,
    mem_used_pct   real NOT NULL,
    disk_used_pct  real NOT NULL,
    disk_used_path varchar(255) NOT NULL DEFAULT '',
    load1          real NOT NULL DEFAULT 0,
    load5          real NOT NULL DEFAULT 0,
    load15         real NOT NULL DEFAULT 0,
    net_rx_bps     real NOT NULL DEFAULT 0,
    net_tx_bps     real NOT NULL DEFAULT 0,
    -- The host list's per-row CPU sparkline: 48 half-hour buckets as a
    -- JSON array of numbers and nulls (a null is a bucket the agent
    -- holds nothing for), exactly as the agent sent it. jsonb rather
    -- than real[] because the value is born as JSON on the wire and
    -- dies as JSON in the API response; an array column would buy two
    -- conversions and a NaN-vs-NULL convention in exchange for nothing
    -- that reads it.
    cpu_trend      jsonb NOT NULL DEFAULT '[]'::jsonb
) WITH (fillfactor = 70);
