-- name: AddRolePermissionRule :exec
INSERT INTO role_permission_rules (role_id, pattern)
VALUES (@role_id, @pattern)
ON CONFLICT DO NOTHING;

-- name: DeleteRolePermissionRules :exec
DELETE FROM role_permission_rules WHERE role_id = @role_id;

-- name: GetRulesByRoleID :many
SELECT pattern FROM role_permission_rules WHERE role_id = @role_id ORDER BY pattern;
