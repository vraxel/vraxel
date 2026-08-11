-- name: GetLoginThrottle :one
SELECT fail_count, window_start FROM login_throttle WHERE key = @key;

-- name: BumpLoginThrottle :one
-- Fixed-window counter: restart the window when the previous one has
-- fully elapsed, otherwise increment within it.
INSERT INTO login_throttle (key, window_start, fail_count)
VALUES (@key, now(), 1)
ON CONFLICT (key) DO UPDATE SET
    fail_count = CASE
        WHEN login_throttle.window_start < now() - make_interval(secs => @window_seconds::int)
        THEN 1
        ELSE login_throttle.fail_count + 1
    END,
    window_start = CASE
        WHEN login_throttle.window_start < now() - make_interval(secs => @window_seconds::int)
        THEN now()
        ELSE login_throttle.window_start
    END
RETURNING fail_count;

-- name: ResetLoginThrottle :exec
DELETE FROM login_throttle WHERE key = @key;

-- name: SweepLoginThrottle :exec
-- GC rows whose window can no longer influence a decision.
DELETE FROM login_throttle
WHERE window_start < now() - make_interval(secs => @window_seconds::int);
