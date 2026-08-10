-- name: CreateOIDCAuthCode :exec
INSERT INTO oidc_auth_codes (code, client_id, user_id, redirect_uri, scopes, nonce,
    code_challenge, code_challenge_method, auth_time, expires_at)
VALUES (@code, @client_id, @user_id, @redirect_uri, @scopes, @nonce,
    @code_challenge, @code_challenge_method, @auth_time, @expires_at);

-- name: ConsumeOIDCAuthCode :one
DELETE FROM oidc_auth_codes
WHERE code = @code AND consumed = false AND expires_at > now()
RETURNING code, client_id, user_id, redirect_uri, scopes, nonce,
    code_challenge, code_challenge_method, auth_time;

-- name: DeleteExpiredOIDCAuthCodes :exec
DELETE FROM oidc_auth_codes WHERE expires_at < now();
