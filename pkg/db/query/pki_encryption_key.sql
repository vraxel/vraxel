-- name: GetPKIEncryptionKey :one
SELECT * FROM pki_encryption_keys ORDER BY id LIMIT 1;

-- name: CreatePKIEncryptionKey :one
INSERT INTO pki_encryption_keys (encryption_key)
VALUES (@encryption_key)
RETURNING *;
