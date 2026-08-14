-- host_agent_join_tokens: one-shot registration tokens exchanged for a
-- long-lived agent token (docs/agent/design.md §4.1). Plaintext is
-- returned once at create; only the SHA-256 hash is persisted.

-- name: CreateHostAgentJoinToken :one
INSERT INTO host_agent_join_tokens (
    name, token_hash, scope, workspace_id, namespace_id,
    max_uses, expires_at, created_by, target_host_id
)
VALUES (@name, @token_hash, @scope, @workspace_id, @namespace_id,
        @max_uses, @expires_at, @created_by, sqlc.narg('target_host_id'))
RETURNING *;

-- name: GetHostAgentJoinTokenByID :one
SELECT t.*,
    COALESCE(NULLIF(u.display_name, ''), u.username, '') AS creator_name
FROM host_agent_join_tokens t
LEFT JOIN users u ON u.id = t.created_by
WHERE t.id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR t.workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR t.namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: CountHostAgentJoinTokens :one
SELECT count(*) FROM host_agent_join_tokens
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR scope = sqlc.narg('scope'))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('search')::VARCHAR IS NULL OR name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%');

-- name: ListHostAgentJoinTokens :many
SELECT t.*,
    COALESCE(NULLIF(u.display_name, ''), u.username, '') AS creator_name
FROM host_agent_join_tokens t
LEFT JOIN users u ON u.id = t.created_by
WHERE (sqlc.narg('scope')::VARCHAR IS NULL OR t.scope = sqlc.narg('scope'))
  AND (sqlc.narg('workspace_id')::BIGINT IS NULL OR t.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('namespace_id')::BIGINT IS NULL OR t.namespace_id = sqlc.narg('namespace_id'))
  AND (sqlc.narg('search')::VARCHAR IS NULL OR t.name ILIKE '%' || sqlc.narg('search')::VARCHAR || '%')
ORDER BY
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN t.name END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'name' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN t.name END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'expires_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN t.expires_at END ASC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'expires_at' AND sqlc.arg('sort_order')::VARCHAR = 'desc' THEN t.expires_at END DESC,
    CASE WHEN sqlc.arg('sort_field')::VARCHAR = 'created_at' AND sqlc.arg('sort_order')::VARCHAR = 'asc' THEN t.created_at END ASC,
    t.created_at DESC
LIMIT sqlc.arg('page_size')::INT
OFFSET sqlc.arg('page_offset')::INT;

-- name: DeleteHostAgentJoinToken :execrows
DELETE FROM host_agent_join_tokens
WHERE id = @id
  AND (sqlc.narg('workspace_id_filter')::BIGINT IS NULL OR workspace_id IS NOT DISTINCT FROM sqlc.narg('workspace_id_filter')::BIGINT)
  AND (sqlc.narg('namespace_id_filter')::BIGINT IS NULL OR namespace_id IS NOT DISTINCT FROM sqlc.narg('namespace_id_filter')::BIGINT);

-- name: ConsumeHostAgentJoinToken :one
-- Atomic claim: the WHERE clause is the concurrency guard (the DB CHECK
-- used_count <= max_uses is the last line of defence behind it).
UPDATE host_agent_join_tokens
SET used_count = used_count + 1
WHERE token_hash = @token_hash
  AND expires_at > now()
  AND used_count < max_uses
RETURNING *;

-- name: PeekHostAgentJoinToken :one
-- Non-consuming liveness check, used by /register before it does any
-- work an unauthenticated caller should not be able to trigger. It burns
-- no use, so the atomic claim above stays the only thing that does.
SELECT * FROM host_agent_join_tokens
WHERE token_hash = @token_hash
  AND expires_at > now()
  AND used_count < max_uses;

-- name: BindHostAgentJoinTokenTarget :exec
-- Record which host a token actually onboarded, once it has.
--
-- Before redemption target_host_id is an operator's intent ("attach to
-- this host"); after redemption it is the outcome. One column, one
-- meaning throughout: the host this token concerns. The wizard reads it
-- to learn which machine answered, which it cannot know any other way --
-- /register's response goes to the agent, not to the browser.
--
-- Only fills a blank: a token minted against a specific host keeps
-- naming that host, and this is a no-op for it.
--
-- And only for a single-use token. Intent and outcome have opposite
-- cardinality: "attach to this one host" implies one use (which is what
-- chk_join_token_bound_single_use encodes), while a token good for N
-- machines has N outcomes and no single host to name. Without the
-- max_uses guard, the first registration against a batch token would
-- violate that CHECK -- silently, since the caller treats this as
-- best-effort. Batch onboarding needs its own answer to "which hosts did
-- this token bring in"; one column is not it.
UPDATE host_agent_join_tokens
SET target_host_id = @target_host_id
WHERE id = @id AND target_host_id IS NULL AND max_uses = 1;

-- name: RefundHostAgentJoinToken :exec
-- Give a use back when registration failed after the claim. The token is
-- typically single-use, so without this a machine whose registration
-- broke halfway can never retry: its token is spent and only an operator
-- minting a new one gets it onboarded.
UPDATE host_agent_join_tokens
SET used_count = used_count - 1
WHERE id = @id AND used_count > 0;
