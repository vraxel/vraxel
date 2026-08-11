-- Self-service registration + social login (GitHub / Google).
--
-- Registration and social users have no phone number, but the initial schema
-- declared phone as NOT NULL UNIQUE. Keep the column NOT NULL DEFAULT '' (so
-- Go code never deals with NULLs) and replace the plain unique constraint with
-- a partial unique index that only applies to real (non-empty) phones -- this
-- lets any number of phone-less users coexist while keeping real phones unique.
ALTER TABLE users DROP CONSTRAINT users_phone_key;
CREATE UNIQUE INDEX uk_users_phone ON users (phone) WHERE phone <> '';

-- --------------------------------------------------------- user_identities
-- Links an external OAuth provider identity (GitHub / Google) to a local user.
-- A local user may have several linked identities; each (provider, subject)
-- pair maps to exactly one user.
CREATE TABLE user_identities (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         VARCHAR(32)  NOT NULL,
    provider_subject VARCHAR(255) NOT NULL,
    email            VARCHAR(255) NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_subject)
);

CREATE INDEX idx_user_identities_user_id ON user_identities (user_id);

-- ------------------------------------------------------------ oauth_states
-- Short-lived CSRF/state store for the outbound social-login OAuth2 flow.
-- Mirrors oidc_pending_requests: the state value is the opaque key handed to
-- the external provider; request_id ties the callback back to the pending
-- vraxel authorization so the existing authorization-code flow can resume.
CREATE TABLE oauth_states (
    state      VARCHAR(64)  PRIMARY KEY,
    provider   VARCHAR(32)  NOT NULL,
    request_id VARCHAR(64)  NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ  NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_oauth_states_expires_at ON oauth_states (expires_at);
