-- name: CreateOAuthState :exec
INSERT INTO oauth_states (state, provider, request_id, expires_at)
VALUES (@state, @provider, @request_id, @expires_at);

-- name: ConsumeOAuthState :one
DELETE FROM oauth_states
WHERE state = @state AND expires_at > now()
RETURNING state, provider, request_id;

-- name: DeleteExpiredOAuthStates :exec
DELETE FROM oauth_states WHERE expires_at < now();
