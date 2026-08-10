-- Initial schema: IAM (users / workspaces / namespaces / roles / role
-- bindings / permissions), OIDC provider storage, and audit logs.

-- ---------------------------------------------------------------- users
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      VARCHAR(255) NOT NULL UNIQUE,
    email         VARCHAR(255) NOT NULL UNIQUE,
    display_name  VARCHAR(255) NOT NULL DEFAULT '',
    phone         VARCHAR(50)  NOT NULL UNIQUE,
    avatar_url    VARCHAR(512) NOT NULL DEFAULT '',
    status        VARCHAR(20)  NOT NULL DEFAULT 'active',
    password_hash VARCHAR(255) NOT NULL DEFAULT '',
    builtin       BOOLEAN      NOT NULL DEFAULT FALSE,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_status       ON users (status);
CREATE INDEX idx_users_created_at   ON users (created_at);
CREATE INDEX idx_users_display_name ON users (display_name);

-- ----------------------------------------------------------- workspaces
CREATE TABLE workspaces (
    id           BIGSERIAL PRIMARY KEY,
    name         VARCHAR(255) NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    description  TEXT         NOT NULL DEFAULT '',
    owner_id     BIGINT       NOT NULL REFERENCES users (id),
    status       VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_by   BIGINT       REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspaces_owner_id   ON workspaces (owner_id);
CREATE INDEX idx_workspaces_status     ON workspaces (status);
CREATE INDEX idx_workspaces_created_at ON workspaces (created_at);
CREATE INDEX idx_workspaces_created_by ON workspaces (created_by);

-- ----------------------------------------------------------- namespaces
CREATE TABLE namespaces (
    id           BIGSERIAL PRIMARY KEY,
    name         VARCHAR(255) NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    description  TEXT         NOT NULL DEFAULT '',
    workspace_id BIGINT       NOT NULL REFERENCES workspaces (id),
    owner_id     BIGINT       NOT NULL REFERENCES users (id),
    visibility   VARCHAR(20)  NOT NULL DEFAULT 'private',
    max_members  INTEGER      NOT NULL DEFAULT 0,
    status       VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_by   BIGINT       REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_namespaces_workspace_id ON namespaces (workspace_id);
CREATE INDEX idx_namespaces_owner_id     ON namespaces (owner_id);
CREATE INDEX idx_namespaces_visibility   ON namespaces (visibility);
CREATE INDEX idx_namespaces_status       ON namespaces (status);
CREATE INDEX idx_namespaces_created_at   ON namespaces (created_at);
CREATE INDEX idx_namespaces_created_by   ON namespaces (created_by);

-- ---------------------------------------------------------------- roles
-- A role lives at exactly one scope. chk_role_scope keeps the
-- (scope, workspace_id, namespace_id) triple self-consistent; the three
-- partial unique indexes give per-scope name uniqueness.
CREATE TABLE roles (
    id           BIGSERIAL PRIMARY KEY,
    name         VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    description  TEXT         NOT NULL DEFAULT '',
    scope        VARCHAR(20)  NOT NULL,
    workspace_id BIGINT       REFERENCES workspaces (id) ON DELETE CASCADE,
    namespace_id BIGINT       REFERENCES namespaces (id) ON DELETE CASCADE,
    builtin      BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT chk_role_scope CHECK (
        (scope = 'platform'  AND workspace_id IS NULL     AND namespace_id IS NULL)
     OR (scope = 'workspace' AND workspace_id IS NOT NULL AND namespace_id IS NULL)
     OR (scope = 'namespace' AND namespace_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX uk_roles_platform  ON roles (name)               WHERE scope = 'platform';
CREATE UNIQUE INDEX uk_roles_workspace ON roles (name, workspace_id) WHERE scope = 'workspace';
CREATE UNIQUE INDEX uk_roles_namespace ON roles (name, namespace_id) WHERE scope = 'namespace';

-- ------------------------------------------------ role_permission_rules
-- A role's permission set is a list of glob patterns (e.g. "iam:users:*").
CREATE TABLE role_permission_rules (
    role_id BIGINT       NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    pattern VARCHAR(255) NOT NULL,
    PRIMARY KEY (role_id, pattern)
);

-- -------------------------------------------------------- role_bindings
CREATE TABLE role_bindings (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users (id)      ON DELETE CASCADE,
    role_id      BIGINT      NOT NULL REFERENCES roles (id)      ON DELETE CASCADE,
    scope        VARCHAR(20) NOT NULL,
    workspace_id BIGINT      REFERENCES workspaces (id) ON DELETE CASCADE,
    namespace_id BIGINT      REFERENCES namespaces (id) ON DELETE CASCADE,
    is_owner     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_binding_scope CHECK (scope IN ('platform', 'workspace', 'namespace')),
    CONSTRAINT chk_binding_ids CHECK (
        (scope = 'platform'  AND workspace_id IS NULL     AND namespace_id IS NULL)
     OR (scope = 'workspace' AND workspace_id IS NOT NULL AND namespace_id IS NULL)
     OR (scope = 'namespace' AND namespace_id IS NOT NULL AND workspace_id IS NOT NULL)
    )
);

CREATE INDEX idx_role_bindings_user      ON role_bindings (user_id);
CREATE INDEX idx_role_bindings_workspace ON role_bindings (workspace_id) WHERE workspace_id IS NOT NULL;
CREATE INDEX idx_role_bindings_namespace ON role_bindings (namespace_id) WHERE namespace_id IS NOT NULL;

CREATE UNIQUE INDEX uk_role_bindings_platform  ON role_bindings (user_id, role_id)               WHERE scope = 'platform';
CREATE UNIQUE INDEX uk_role_bindings_workspace ON role_bindings (user_id, role_id, workspace_id) WHERE scope = 'workspace';
CREATE UNIQUE INDEX uk_role_bindings_namespace ON role_bindings (user_id, role_id, namespace_id) WHERE scope = 'namespace';

-- ---------------------------------------------------------- permissions
-- Rebuilt on every boot from the apiserver's registered routes
-- (see pkg/apis/iam/rbac_sync.go). Never edited by hand.
CREATE TABLE permissions (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(255) NOT NULL,
    method      VARCHAR(10)  NOT NULL,
    path        VARCHAR(512) NOT NULL,
    scope       VARCHAR(20)  NOT NULL DEFAULT 'platform',
    description VARCHAR(512) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (code, scope)
);

-- ------------------------------------------------------- refresh_tokens
CREATE TABLE refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    user_id    BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    client_id  VARCHAR(255) NOT NULL,
    scope      TEXT         NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ  NOT NULL,
    revoked    BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user_id    ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);

-- ------------------------------------------------------- OIDC provider
CREATE TABLE oidc_keys (
    id          BIGSERIAL PRIMARY KEY,
    key_id      VARCHAR(64) NOT NULL UNIQUE,
    private_key BYTEA       NOT NULL,
    public_key  BYTEA       NOT NULL,
    algorithm   VARCHAR(16) NOT NULL DEFAULT 'EdDSA',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE oidc_sessions (
    session_id VARCHAR(64) PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    auth_time  TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_oidc_sessions_expires_at ON oidc_sessions (expires_at);

CREATE TABLE oidc_auth_codes (
    code                  VARCHAR(64) PRIMARY KEY,
    client_id             VARCHAR(255) NOT NULL,
    user_id               BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    redirect_uri          TEXT         NOT NULL,
    scopes                TEXT         NOT NULL DEFAULT '',
    nonce                 VARCHAR(255) NOT NULL DEFAULT '',
    code_challenge        VARCHAR(255) NOT NULL DEFAULT '',
    code_challenge_method VARCHAR(10)  NOT NULL DEFAULT '',
    auth_time             TIMESTAMPTZ  NOT NULL,
    expires_at            TIMESTAMPTZ  NOT NULL,
    consumed              BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_oidc_auth_codes_expires_at ON oidc_auth_codes (expires_at);

CREATE TABLE oidc_pending_requests (
    id                    VARCHAR(64) PRIMARY KEY,
    response_type         VARCHAR(20)  NOT NULL,
    client_id             VARCHAR(255) NOT NULL,
    redirect_uri          TEXT         NOT NULL,
    scope                 TEXT         NOT NULL DEFAULT '',
    state                 TEXT         NOT NULL DEFAULT '',
    nonce                 VARCHAR(255) NOT NULL DEFAULT '',
    code_challenge        VARCHAR(255) NOT NULL DEFAULT '',
    code_challenge_method VARCHAR(10)  NOT NULL DEFAULT '',
    expires_at            TIMESTAMPTZ  NOT NULL,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_oidc_pending_requests_expires_at ON oidc_pending_requests (expires_at);

-- ----------------------------------------------------------- audit_logs
CREATE TABLE audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT,
    username        VARCHAR(255) NOT NULL DEFAULT '',
    event_type      VARCHAR(50)  NOT NULL,
    action          VARCHAR(50)  NOT NULL,
    resource_type   VARCHAR(100) NOT NULL DEFAULT '',
    resource_id     VARCHAR(100) NOT NULL DEFAULT '',
    module          VARCHAR(50)  NOT NULL DEFAULT '',
    scope           VARCHAR(20)  NOT NULL DEFAULT 'platform',
    workspace_id    BIGINT,
    namespace_id    BIGINT,
    http_method     VARCHAR(10)  NOT NULL DEFAULT '',
    http_path       VARCHAR(500) NOT NULL DEFAULT '',
    status_code     INTEGER      NOT NULL DEFAULT 0,
    client_ip       VARCHAR(45)  NOT NULL DEFAULT '',
    user_agent      VARCHAR(500) NOT NULL DEFAULT '',
    duration_ms     INTEGER      NOT NULL DEFAULT 0,
    success         BOOLEAN      NOT NULL DEFAULT TRUE,
    detail          JSONB,
    response_detail JSONB,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_user_id       ON audit_logs (user_id);
CREATE INDEX idx_audit_logs_event_type    ON audit_logs (event_type);
CREATE INDEX idx_audit_logs_resource_type ON audit_logs (resource_type);
CREATE INDEX idx_audit_logs_workspace_id  ON audit_logs (workspace_id);
CREATE INDEX idx_audit_logs_namespace_id  ON audit_logs (namespace_id);
CREATE INDEX idx_audit_logs_created_at    ON audit_logs (created_at);
