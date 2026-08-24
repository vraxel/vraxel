-- host_metrics_latest: one overwritten row per host, written by the
-- agent gateway from heartbeats. NEVER publish a host event from here --
-- one beat per host per 15s would flood every watcher forever.

-- name: UpsertHostMetricsLatest :exec
INSERT INTO host_metrics_latest (
    host_id, sampled_at,
    cpu_used_pct, mem_used_pct, disk_used_pct, disk_used_path,
    disk_used_bytes, disk_total_bytes,
    load1, load5, load15, net_rx_bps, net_tx_bps, cpu_trend
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
ON CONFLICT (host_id) DO UPDATE SET
    sampled_at       = EXCLUDED.sampled_at,
    cpu_used_pct     = EXCLUDED.cpu_used_pct,
    mem_used_pct     = EXCLUDED.mem_used_pct,
    disk_used_pct    = EXCLUDED.disk_used_pct,
    disk_used_path   = EXCLUDED.disk_used_path,
    disk_used_bytes  = EXCLUDED.disk_used_bytes,
    disk_total_bytes = EXCLUDED.disk_total_bytes,
    load1            = EXCLUDED.load1,
    load5            = EXCLUDED.load5,
    load15           = EXCLUDED.load15,
    net_rx_bps       = EXCLUDED.net_rx_bps,
    net_tx_bps       = EXCLUDED.net_tx_bps,
    cpu_trend        = EXCLUDED.cpu_trend;
