-- name: CreateOIDCSession :exec
INSERT INTO oidc_sessions (session_id, user_id, auth_time, expires_at)
VALUES (@session_id, @user_id, @auth_time, @expires_at);

-- name: GetOIDCSession :one
SELECT session_id, user_id, auth_time, expires_at
FROM oidc_sessions
WHERE session_id = @session_id AND expires_at > now();

-- name: DeleteOIDCSession :execrows
DELETE FROM oidc_sessions WHERE session_id = @session_id;

-- name: DeleteExpiredOIDCSessions :exec
DELETE FROM oidc_sessions WHERE expires_at < now();
