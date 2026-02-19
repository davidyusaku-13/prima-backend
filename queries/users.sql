-- name: UpsertByClerkID :exec
INSERT INTO users (clerk_id, username, name, email, role, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'user', TRUE, NOW(), NOW())
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    email      = COALESCE(EXCLUDED.email, users.email),
    updated_at = NOW();

-- name: ListUsers :many
SELECT
    id,
    COALESCE(clerk_id, '')::text      AS clerk_id,
    name,
    COALESCE(email, '')::text         AS email,
    COALESCE(username, '')::text      AS username,
    role,
    is_active,
    created_at,
    updated_at,
    deleted_at,
    last_login_at
FROM users
ORDER BY id DESC;

-- name: DeleteUserByClerkID :exec
DELETE FROM users WHERE clerk_id = $1;

-- name: SoftDeleteUserByClerkID :exec
UPDATE users
SET deleted_at = NOW(), is_active = FALSE, updated_at = NOW()
WHERE clerk_id = $1;

-- name: UpdateLastLogin :exec
UPDATE users
SET last_login_at = NOW(), updated_at = NOW()
WHERE clerk_id = $1;
