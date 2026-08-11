-- name: CreateUserIdentity :one
INSERT INTO user_identities (user_id, provider, provider_subject, email)
VALUES (@user_id, @provider, @provider_subject, @email)
RETURNING id, user_id, provider, provider_subject, email, created_at;

-- name: GetUserIdentity :one
SELECT id, user_id, provider, provider_subject, email, created_at
FROM user_identities
WHERE provider = @provider AND provider_subject = @provider_subject;
