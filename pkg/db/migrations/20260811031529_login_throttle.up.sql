-- Login brute-force throttle: fixed-window failure counters keyed by
-- "u:<lower(username)>" and "ip:<client-ip>". Lives in PG (not process
-- memory) because vraxel-server scales horizontally and all instances
-- must see the same counters.
CREATE TABLE login_throttle (
    key          TEXT        PRIMARY KEY,
    window_start TIMESTAMPTZ NOT NULL DEFAULT now(),
    fail_count   INTEGER     NOT NULL DEFAULT 0
);
