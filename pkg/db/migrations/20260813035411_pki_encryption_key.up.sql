-- Master encryption key for the platform. A single row holding a 32-byte
-- AES-256 key, generated once on first boot and persisted. The agent-token
-- signing key is derived from it; later it also backs credential/secret
-- encryption when the full pki module lands.
CREATE TABLE pki_encryption_keys (
    id             bigserial   PRIMARY KEY,
    encryption_key bytea       NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
