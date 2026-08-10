-- name: CreateOIDCPendingRequest :exec
INSERT INTO oidc_pending_requests (id, response_type, client_id, redirect_uri, scope,
    state, nonce, code_challenge, code_challenge_method, expires_at)
VALUES (@id, @response_type, @client_id, @redirect_uri, @scope,
    @state, @nonce, @code_challenge, @code_challenge_method, @expires_at);

-- name: ConsumeOIDCPendingRequest :one
DELETE FROM oidc_pending_requests
WHERE id = @id AND expires_at > now()
RETURNING id, response_type, client_id, redirect_uri, scope,
    state, nonce, code_challenge, code_challenge_method;

-- name: DeleteExpiredOIDCPendingRequests :exec
DELETE FROM oidc_pending_requests WHERE expires_at < now();
