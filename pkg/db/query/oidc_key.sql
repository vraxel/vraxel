-- name: GetOIDCKey :one
SELECT * FROM oidc_keys WHERE algorithm = @algorithm LIMIT 1;

-- name: CreateOIDCKey :one
INSERT INTO oidc_keys (key_id, private_key, public_key, algorithm)
VALUES (@key_id, @private_key, @public_key, @algorithm)
RETURNING *;
