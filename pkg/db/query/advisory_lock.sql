-- name: AcquireAdvisoryLock :exec
-- Blocking advisory lock scoped to the caller's transaction. Released
-- on COMMIT / ROLLBACK. Coordinates cross-instance work in the vraxel-
-- server deployment; sync.Mutex is not an option because multiple
-- instances share the same database.
SELECT pg_advisory_xact_lock($1);

-- name: TryAdvisoryLock :one
-- Non-blocking advisory lock. Returns true if acquired, false if
-- another session already holds the key. Released on COMMIT / ROLLBACK.
SELECT pg_try_advisory_xact_lock($1);
