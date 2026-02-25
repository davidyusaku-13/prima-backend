-- name: UpsertUserWithRole :exec
INSERT INTO users (clerk_id, username, name, email, first_name, last_name, role, is_active, created_at, updated_at)
VALUES (
  $1, $2, $3, $4, $5, $6,
  CASE WHEN NOT EXISTS (SELECT 1 FROM users WHERE deleted_at IS NULL)
       THEN 'superadmin'
       ELSE 'user'
  END,
  CASE WHEN NOT EXISTS (SELECT 1 FROM users WHERE deleted_at IS NULL)
       THEN TRUE
       ELSE FALSE
  END,
  NOW(), NOW()
)
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    first_name = EXCLUDED.first_name,
    last_name  = EXCLUDED.last_name,
    email      = COALESCE(EXCLUDED.email, users.email),
    is_active  = CASE
                   WHEN users.role = 'superadmin' THEN TRUE
                   WHEN EXISTS (
                     SELECT 1
                     FROM hospital_memberships hm
                     JOIN hospitals h ON h.id = hm.hospital_id
                     WHERE hm.user_clerk_id = users.clerk_id
                       AND hm.is_active = TRUE
                       AND hm.deleted_at IS NULL
                       AND h.is_active = TRUE
                       AND h.deleted_at IS NULL
                   ) THEN TRUE
                   ELSE FALSE
                 END,
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

-- name: GetUserAuthContext :one
SELECT
  u.clerk_id,
  u.role AS global_role,
  u.is_active,
  COALESCE(hm.hospital_id, 0)::bigint AS hospital_id,
  COALESCE(hm.membership_role, '')::text AS membership_role
FROM users u
LEFT JOIN LATERAL (
  SELECT hm.hospital_id, hm.membership_role
  FROM hospital_memberships hm
  JOIN hospitals h ON h.id = hm.hospital_id
  WHERE hm.user_clerk_id = u.clerk_id
    AND hm.is_active = TRUE
    AND hm.deleted_at IS NULL
    AND h.is_active = TRUE
    AND h.deleted_at IS NULL
  LIMIT 1
) hm ON TRUE
WHERE u.clerk_id = $1
  AND u.is_active = TRUE
  AND u.deleted_at IS NULL;

-- name: GetUserByClerkID :one
SELECT
  clerk_id,
  name,
  role,
  is_active,
  deleted_at
FROM users
WHERE clerk_id = $1;

-- name: SetUserActiveByClerkID :exec
UPDATE users
SET is_active = $2, updated_at = NOW()
WHERE clerk_id = $1;

-- name: SetUserRoleByClerkID :exec
UPDATE users
SET role = $2, updated_at = NOW()
WHERE clerk_id = $1;
