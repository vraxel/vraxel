-- Make the master key a real singleton.
--
-- The table was created with a bigserial id and no uniqueness beyond it,
-- so two instances booting against an empty database each generated a key
-- and each INSERT succeeded. Both then signed agent tokens with a
-- different key; after the next restart every instance read whichever row
-- sorted first, and the agents holding tokens signed with the other key
-- were locked out for good (their 90-day credential is the only thing
-- they have, and the join token that produced it is single-use).
--
-- Pinning the row at id = 1 turns the second writer's INSERT into a
-- conflict, which the store now resolves by reading the winner's key.

-- Collapse any rows a pre-fix boot race already produced. The lowest id
-- is the one every instance would have converged on by ORDER BY id, so
-- keeping it invalidates the fewest live tokens.
DELETE FROM pki_encryption_keys
WHERE id <> (SELECT MIN(id) FROM pki_encryption_keys);

UPDATE pki_encryption_keys SET id = 1 WHERE id <> 1;

-- DROP DEFAULT retires the sequence: the id is now a constant the writer
-- supplies, not something the database hands out per insert.
ALTER TABLE pki_encryption_keys
    ALTER COLUMN id DROP DEFAULT,
    ADD CONSTRAINT chk_pki_encryption_key_singleton CHECK (id = 1);
