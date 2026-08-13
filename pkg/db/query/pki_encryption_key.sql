-- name: GetPKIEncryptionKey :one
-- The master key lives in exactly one row, pinned at id = 1 by
-- chk_pki_encryption_key_singleton.
SELECT * FROM pki_encryption_keys WHERE id = 1;

-- name: CreatePKIEncryptionKey :exec
-- First-boot insert. DO NOTHING rather than an error on conflict: when
-- several instances boot against an empty database they each generate a
-- candidate key and race to insert it. Exactly one wins; the losers must
-- then READ the winner's key instead of using their own, or they would
-- sign agent tokens no other instance can verify. The store performs that
-- read unconditionally, which is why this statement returns nothing.
INSERT INTO pki_encryption_keys (id, encryption_key)
VALUES (1, @encryption_key)
ON CONFLICT (id) DO NOTHING;
