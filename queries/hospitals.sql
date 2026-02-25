-- name: CreateHospital :one
INSERT INTO hospitals (name, slug, created_by_clerk_id, is_active, created_at, updated_at)
VALUES ($1, $2, $3, TRUE, NOW(), NOW())
RETURNING id, name, slug, is_active, created_by_clerk_id, created_at, updated_at, deleted_at;

-- name: ListHospitals :many
SELECT id, name, slug, is_active, created_by_clerk_id, created_at, updated_at, deleted_at
FROM hospitals
WHERE deleted_at IS NULL
ORDER BY created_at DESC, id DESC;

-- name: GetHospitalBySlug :one
SELECT id, name, slug, is_active, created_by_clerk_id, created_at, updated_at, deleted_at
FROM hospitals
WHERE slug = $1
  AND deleted_at IS NULL;

-- name: GetHospitalByID :one
SELECT id, name, slug, is_active, created_by_clerk_id, created_at, updated_at, deleted_at
FROM hospitals
WHERE id = $1
  AND deleted_at IS NULL;

-- name: AssignHospitalAdmin :exec
INSERT INTO hospital_memberships (
    hospital_id,
    user_clerk_id,
    membership_role,
    is_active,
    invited_by_clerk_id,
    created_at,
    updated_at
)
VALUES ($1, $2, 'admin', TRUE, $3, NOW(), NOW())
ON CONFLICT (user_clerk_id) DO UPDATE
SET hospital_id = EXCLUDED.hospital_id,
    membership_role = EXCLUDED.membership_role,
    is_active = TRUE,
    invited_by_clerk_id = EXCLUDED.invited_by_clerk_id,
    deleted_at = NULL,
    updated_at = NOW();

-- name: ListHospitalUsersBySlug :many
SELECT
  u.clerk_id,
  u.name,
  COALESCE(u.email, '')::text AS email,
  COALESCE(u.username, '')::text AS username,
  COALESCE(u.first_name, '')::text AS first_name,
  COALESCE(u.last_name, '')::text AS last_name,
  u.role,
  hm.membership_role,
  u.is_active,
  u.created_at,
  u.updated_at,
  u.last_login_at
FROM hospital_memberships hm
JOIN hospitals h ON h.id = hm.hospital_id
JOIN users u ON u.clerk_id = hm.user_clerk_id
WHERE h.slug = $1
  AND h.deleted_at IS NULL
  AND hm.deleted_at IS NULL
  AND u.deleted_at IS NULL
ORDER BY hm.created_at DESC, u.clerk_id DESC;

-- name: ListHospitalAdminsBySlug :many
SELECT
  u.clerk_id,
  u.name,
  COALESCE(u.email, '')::text AS email,
  COALESCE(u.username, '')::text AS username,
  hm.created_at AS assigned_at
FROM hospital_memberships hm
JOIN hospitals h ON h.id = hm.hospital_id
JOIN users u ON u.clerk_id = hm.user_clerk_id
WHERE h.slug = $1
  AND h.deleted_at IS NULL
  AND hm.membership_role = 'admin'
  AND hm.is_active = TRUE
  AND hm.deleted_at IS NULL
  AND u.deleted_at IS NULL
ORDER BY hm.created_at DESC, u.clerk_id DESC;

-- name: ListAdminAssignmentCandidates :many
SELECT
  u.clerk_id,
  u.name,
  COALESCE(u.email, '')::text AS email,
  COALESCE(u.username, '')::text AS username,
  u.role,
  u.is_active,
  COALESCE(hm.hospital_id, 0)::bigint AS membership_hospital_id,
  COALESCE(hm.membership_role, '')::text AS membership_role,
  COALESCE(hm.is_active, FALSE) AS membership_is_active,
  COALESCE(hm_h.slug, '')::text AS membership_hospital_slug
FROM users u
LEFT JOIN hospital_memberships hm
  ON hm.user_clerk_id = u.clerk_id
 AND hm.is_active = TRUE
 AND hm.deleted_at IS NULL
LEFT JOIN hospitals hm_h ON hm_h.id = hm.hospital_id
WHERE u.deleted_at IS NULL
  AND u.role <> 'superadmin'
ORDER BY u.created_at DESC, u.clerk_id DESC;

-- name: DeactivateHospitalAdminMembership :execrows
UPDATE hospital_memberships
SET
  is_active = FALSE,
  deleted_at = NOW(),
  updated_at = NOW()
WHERE hospital_id = $1
  AND user_clerk_id = $2
  AND membership_role = 'admin'
  AND is_active = TRUE
  AND deleted_at IS NULL;
