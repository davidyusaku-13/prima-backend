-- name: CreateHospitalInvite :one
INSERT INTO hospital_invites (
    hospital_id,
    token_hash,
    invite_role,
    created_by_clerk_id,
    expires_at,
    created_at
)
VALUES ($1, $2, 'user', $3, $4, NOW())
RETURNING id, hospital_id, token_hash, invite_role, created_by_clerk_id, expires_at, consumed_at, consumed_by_clerk_id, created_at;

-- name: ListHospitalInvitesBySlug :many
SELECT
  hi.id,
  hi.hospital_id,
  hi.invite_role,
  hi.created_by_clerk_id,
  hi.expires_at,
  hi.consumed_at,
  hi.consumed_by_clerk_id,
  hi.revoked_at,
  hi.revoked_by_clerk_id,
  hi.created_at
FROM hospital_invites hi
JOIN hospitals h ON h.id = hi.hospital_id
WHERE h.slug = $1
  AND h.deleted_at IS NULL
ORDER BY hi.created_at DESC, hi.id DESC;

-- name: GetHospitalInviteByTokenHash :one
SELECT
  hi.id,
  hi.hospital_id,
  hi.token_hash,
  hi.invite_role,
  hi.created_by_clerk_id,
  hi.expires_at,
  hi.consumed_at,
  hi.consumed_by_clerk_id,
  hi.revoked_at,
  hi.revoked_by_clerk_id,
  hi.created_at,
  h.is_active AS hospital_is_active,
  h.deleted_at AS hospital_deleted_at
FROM hospital_invites hi
JOIN hospitals h ON h.id = hi.hospital_id
WHERE hi.token_hash = $1;

-- name: GetHospitalInviteByIDForUpdate :one
SELECT
  id,
  hospital_id,
  invite_role,
  expires_at,
  consumed_at,
  revoked_at
FROM hospital_invites
WHERE id = $1
  AND hospital_id = $2
FOR UPDATE;

-- name: RevokeHospitalInvite :execrows
UPDATE hospital_invites
SET revoked_at = NOW(),
    revoked_by_clerk_id = $3
WHERE id = $1
  AND hospital_id = $2
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- name: ConsumeHospitalInvite :execrows
UPDATE hospital_invites
SET consumed_at = NOW(),
    consumed_by_clerk_id = $2
WHERE id = $1
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > NOW();
