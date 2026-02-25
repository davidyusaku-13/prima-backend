-- name: HasActiveHospitalMembership :one
SELECT EXISTS (
  SELECT 1
  FROM hospital_memberships hm
  JOIN hospitals h ON h.id = hm.hospital_id
  WHERE hm.user_clerk_id = $1
    AND hm.is_active = TRUE
    AND hm.deleted_at IS NULL
    AND h.is_active = TRUE
    AND h.deleted_at IS NULL
) AS has_membership;

-- name: HasActiveAdminMembership :one
SELECT EXISTS (
  SELECT 1
  FROM hospital_memberships hm
  JOIN hospitals h ON h.id = hm.hospital_id
  WHERE hm.user_clerk_id = $1
    AND hm.membership_role = 'admin'
    AND hm.is_active = TRUE
    AND hm.deleted_at IS NULL
    AND h.is_active = TRUE
    AND h.deleted_at IS NULL
) AS has_admin_membership;

-- name: GetActiveHospitalMembershipByUser :one
SELECT
  hospital_id,
  membership_role
FROM hospital_memberships
WHERE user_clerk_id = $1
  AND is_active = TRUE
  AND deleted_at IS NULL;

-- name: UpsertHospitalUserMembership :exec
INSERT INTO hospital_memberships (
    hospital_id,
    user_clerk_id,
    membership_role,
    is_active,
    invited_by_clerk_id,
    created_at,
    updated_at
)
VALUES ($1, $2, $3, TRUE, $4, NOW(), NOW())
ON CONFLICT (user_clerk_id) DO UPDATE
SET hospital_id = EXCLUDED.hospital_id,
    membership_role = EXCLUDED.membership_role,
    is_active = TRUE,
    invited_by_clerk_id = EXCLUDED.invited_by_clerk_id,
    deleted_at = NULL,
    updated_at = NOW();
