-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (token_hash, user_id, client_id, scope, expires_at)
VALUES (@token_hash, @user_id, @client_id, @scope, @expires_at)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = @token_hash;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked = true WHERE token_hash = @token_hash;

-- name: RevokeRefreshTokensByUserID :exec
UPDATE refresh_tokens SET revoked = true WHERE user_id = @user_id;

-- name: ConsumeRefreshToken :one
UPDATE refresh_tokens
SET revoked = true
WHERE token_hash = @token_hash AND revoked = false AND expires_at > now()
RETURNING *;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens WHERE expires_at < now() OR revoked = true;
