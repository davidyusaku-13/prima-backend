-- name: UpsertUserWithRole :exec
INSERT INTO users (clerk_id, username, name, email, first_name, last_name, role, is_active, created_at, updated_at)
VALUES (
  $1, $2, $3, $4, $5, $6,
  CASE WHEN NOT EXISTS (SELECT 1 FROM users WHERE deleted_at IS NULL)
       THEN 'superadmin'
       ELSE 'user'
  END,
  TRUE, NOW(), NOW()
)
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    first_name = EXCLUDED.first_name,
    last_name  = EXCLUDED.last_name,
    email      = COALESCE(EXCLUDED.email, users.email),
    is_active  = TRUE,
    deleted_at = NULL,
    updated_at = NOW();

-- name: ListUsers :many
SELECT
    COALESCE(clerk_id, '')::text      AS clerk_id,
    name,
    COALESCE(email, '')::text         AS email,
    COALESCE(username, '')::text      AS username,
    COALESCE(first_name, '')::text    AS first_name,
    COALESCE(last_name, '')::text     AS last_name,
    role,
    is_active,
    created_at,
    updated_at,
    deleted_at,
    last_login_at
FROM users
WHERE deleted_at IS NULL
ORDER BY created_at DESC, clerk_id DESC;

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

-- name: GetUserRole :one
SELECT role FROM users WHERE clerk_id = $1 AND is_active = TRUE AND deleted_at IS NULL;
