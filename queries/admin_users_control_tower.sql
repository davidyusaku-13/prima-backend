-- name: AdminUsersOverview :one
WITH base AS (
    SELECT
        u.clerk_id,
        u.role,
        u.is_active,
        u.last_login_at,
        COALESCE(hm.hospital_id, 0)::bigint AS membership_hospital_id,
        COALESCE(hm.membership_role, '')::text AS membership_role,
        COALESCE(h.is_active, FALSE) AS membership_hospital_active
    FROM users u
    LEFT JOIN hospital_memberships hm
        ON hm.user_clerk_id = u.clerk_id
       AND hm.is_active = TRUE
       AND hm.deleted_at IS NULL
    LEFT JOIN hospitals h
        ON h.id = hm.hospital_id
       AND h.deleted_at IS NULL
    WHERE u.deleted_at IS NULL
)
SELECT
    COUNT(*)::bigint AS total_users,
    COUNT(*) FILTER (WHERE is_active = TRUE)::bigint AS active_users,
    COUNT(*) FILTER (WHERE is_active = FALSE)::bigint AS inactive_users,
    COUNT(*) FILTER (WHERE role <> 'superadmin' AND membership_hospital_id = 0)::bigint AS unassigned_users,
    COUNT(*) FILTER (WHERE role = 'admin' AND membership_role <> 'admin')::bigint AS admin_role_mismatch_users,
    COUNT(*) FILTER (WHERE is_active = FALSE AND membership_hospital_id > 0)::bigint AS inactive_with_membership_users,
    COUNT(*) FILTER (WHERE membership_hospital_id > 0 AND membership_hospital_active = FALSE)::bigint AS assigned_inactive_hospital_users,
    COUNT(*) FILTER (WHERE last_login_at IS NULL)::bigint AS never_logged_in_users
FROM base;

-- name: ListHospitalsForAdminUsers :many
SELECT
    id,
    name,
    slug,
    is_active
FROM hospitals
WHERE deleted_at IS NULL
ORDER BY is_active DESC, name ASC, id ASC;

-- name: SearchAdminUsers :many
WITH base AS (
    SELECT
        u.clerk_id,
        u.name,
        COALESCE(u.email, '')::text AS email,
        COALESCE(u.username, '')::text AS username,
        COALESCE(u.first_name, '')::text AS first_name,
        COALESCE(u.last_name, '')::text AS last_name,
        u.role,
        u.is_active,
        u.created_at,
        u.updated_at,
        u.last_login_at,
        COALESCE(hm.hospital_id, 0)::bigint AS membership_hospital_id,
        COALESCE(h.slug, '')::text AS membership_hospital_slug,
        COALESCE(h.name, '')::text AS membership_hospital_name,
        COALESCE(h.is_active, FALSE) AS membership_hospital_active,
        COALESCE(hm.membership_role, '')::text AS membership_role,
        (u.role <> 'superadmin' AND COALESCE(hm.hospital_id, 0) = 0)::boolean AS anomaly_no_membership,
        (u.role = 'admin' AND COALESCE(hm.membership_role, '') <> 'admin')::boolean AS anomaly_admin_role_mismatch,
        (u.is_active = FALSE AND COALESCE(hm.hospital_id, 0) > 0)::boolean AS anomaly_inactive_with_membership,
        (COALESCE(hm.hospital_id, 0) > 0 AND COALESCE(h.is_active, FALSE) = FALSE)::boolean AS anomaly_assigned_inactive_hospital,
        (u.last_login_at IS NULL)::boolean AS anomaly_never_logged_in
    FROM users u
    LEFT JOIN hospital_memberships hm
        ON hm.user_clerk_id = u.clerk_id
       AND hm.is_active = TRUE
       AND hm.deleted_at IS NULL
    LEFT JOIN hospitals h
        ON h.id = hm.hospital_id
       AND h.deleted_at IS NULL
    WHERE u.deleted_at IS NULL
)
SELECT
    clerk_id,
    name,
    email,
    username,
    first_name,
    last_name,
    role,
    is_active,
    created_at,
    updated_at,
    last_login_at,
    membership_hospital_id,
    membership_hospital_slug,
    membership_hospital_name,
    membership_hospital_active,
    membership_role,
    anomaly_no_membership,
    anomaly_admin_role_mismatch,
    anomaly_inactive_with_membership,
    anomaly_assigned_inactive_hospital,
    anomaly_never_logged_in
FROM base
WHERE
    ($1::text = ''
        OR base.clerk_id ILIKE '%' || $1 || '%'
        OR base.name ILIKE '%' || $1 || '%'
        OR base.email ILIKE '%' || $1 || '%'
        OR base.username ILIKE '%' || $1 || '%')
    AND ($2::text = '' OR base.role = $2)
    AND (
        $3::text = ''
        OR ($3::text = 'active' AND base.is_active = TRUE)
        OR ($3::text = 'inactive' AND base.is_active = FALSE)
    )
    AND ($4::text = '' OR base.membership_hospital_slug = $4)
    AND ($5::text = '' OR base.membership_role = $5)
    AND (
        $6::text = ''
        OR ($6::text = 'no_membership' AND base.anomaly_no_membership)
        OR ($6::text = 'admin_role_mismatch' AND base.anomaly_admin_role_mismatch)
        OR ($6::text = 'inactive_with_membership' AND base.anomaly_inactive_with_membership)
        OR ($6::text = 'assigned_inactive_hospital' AND base.anomaly_assigned_inactive_hospital)
        OR ($6::text = 'never_logged_in' AND base.anomaly_never_logged_in)
    )
    AND (
        $7::text = ''
        OR ($7::text = 'never' AND base.last_login_at IS NULL)
        OR ($7::text = 'gt_30d' AND base.last_login_at < NOW() - INTERVAL '30 days')
        OR ($7::text = 'gt_90d' AND base.last_login_at < NOW() - INTERVAL '90 days')
        OR ($7::text = 'gt_180d' AND base.last_login_at < NOW() - INTERVAL '180 days')
        OR ($7::text = 'recent_30d' AND base.last_login_at >= NOW() - INTERVAL '30 days')
    )
ORDER BY base.created_at DESC, base.clerk_id DESC
LIMIT $8 OFFSET $9;

-- name: CountAdminUsersSearch :one
WITH base AS (
    SELECT
        u.clerk_id,
        u.name,
        COALESCE(u.email, '')::text AS email,
        COALESCE(u.username, '')::text AS username,
        u.role,
        u.is_active,
        u.last_login_at,
        COALESCE(hm.hospital_id, 0)::bigint AS membership_hospital_id,
        COALESCE(h.slug, '')::text AS membership_hospital_slug,
        COALESCE(hm.membership_role, '')::text AS membership_role,
        (u.role <> 'superadmin' AND COALESCE(hm.hospital_id, 0) = 0)::boolean AS anomaly_no_membership,
        (u.role = 'admin' AND COALESCE(hm.membership_role, '') <> 'admin')::boolean AS anomaly_admin_role_mismatch,
        (u.is_active = FALSE AND COALESCE(hm.hospital_id, 0) > 0)::boolean AS anomaly_inactive_with_membership,
        (COALESCE(hm.hospital_id, 0) > 0 AND COALESCE(h.is_active, FALSE) = FALSE)::boolean AS anomaly_assigned_inactive_hospital,
        (u.last_login_at IS NULL)::boolean AS anomaly_never_logged_in
    FROM users u
    LEFT JOIN hospital_memberships hm
        ON hm.user_clerk_id = u.clerk_id
       AND hm.is_active = TRUE
       AND hm.deleted_at IS NULL
    LEFT JOIN hospitals h
        ON h.id = hm.hospital_id
       AND h.deleted_at IS NULL
    WHERE u.deleted_at IS NULL
)
SELECT COUNT(*)::bigint
FROM base
WHERE
    ($1::text = ''
        OR base.clerk_id ILIKE '%' || $1 || '%'
        OR base.name ILIKE '%' || $1 || '%'
        OR base.email ILIKE '%' || $1 || '%'
        OR base.username ILIKE '%' || $1 || '%')
    AND ($2::text = '' OR base.role = $2)
    AND (
        $3::text = ''
        OR ($3::text = 'active' AND base.is_active = TRUE)
        OR ($3::text = 'inactive' AND base.is_active = FALSE)
    )
    AND ($4::text = '' OR base.membership_hospital_slug = $4)
    AND ($5::text = '' OR base.membership_role = $5)
    AND (
        $6::text = ''
        OR ($6::text = 'no_membership' AND base.anomaly_no_membership)
        OR ($6::text = 'admin_role_mismatch' AND base.anomaly_admin_role_mismatch)
        OR ($6::text = 'inactive_with_membership' AND base.anomaly_inactive_with_membership)
        OR ($6::text = 'assigned_inactive_hospital' AND base.anomaly_assigned_inactive_hospital)
        OR ($6::text = 'never_logged_in' AND base.anomaly_never_logged_in)
    )
    AND (
        $7::text = ''
        OR ($7::text = 'never' AND base.last_login_at IS NULL)
        OR ($7::text = 'gt_30d' AND base.last_login_at < NOW() - INTERVAL '30 days')
        OR ($7::text = 'gt_90d' AND base.last_login_at < NOW() - INTERVAL '90 days')
        OR ($7::text = 'gt_180d' AND base.last_login_at < NOW() - INTERVAL '180 days')
        OR ($7::text = 'recent_30d' AND base.last_login_at >= NOW() - INTERVAL '30 days')
    );

-- name: GetAdminUserDetail :one
SELECT
    u.clerk_id,
    u.name,
    COALESCE(u.email, '')::text AS email,
    COALESCE(u.username, '')::text AS username,
    COALESCE(u.first_name, '')::text AS first_name,
    COALESCE(u.last_name, '')::text AS last_name,
    u.role,
    u.is_active,
    u.created_at,
    u.updated_at,
    u.last_login_at,
    COALESCE(hm.hospital_id, 0)::bigint AS membership_hospital_id,
    COALESCE(h.slug, '')::text AS membership_hospital_slug,
    COALESCE(h.name, '')::text AS membership_hospital_name,
    COALESCE(h.is_active, FALSE) AS membership_hospital_active,
    COALESCE(hm.membership_role, '')::text AS membership_role,
    (u.role <> 'superadmin' AND COALESCE(hm.hospital_id, 0) = 0)::boolean AS anomaly_no_membership,
    (u.role = 'admin' AND COALESCE(hm.membership_role, '') <> 'admin')::boolean AS anomaly_admin_role_mismatch,
    (u.is_active = FALSE AND COALESCE(hm.hospital_id, 0) > 0)::boolean AS anomaly_inactive_with_membership,
    (COALESCE(hm.hospital_id, 0) > 0 AND COALESCE(h.is_active, FALSE) = FALSE)::boolean AS anomaly_assigned_inactive_hospital,
    (u.last_login_at IS NULL)::boolean AS anomaly_never_logged_in
FROM users u
LEFT JOIN hospital_memberships hm
    ON hm.user_clerk_id = u.clerk_id
   AND hm.is_active = TRUE
   AND hm.deleted_at IS NULL
LEFT JOIN hospitals h
    ON h.id = hm.hospital_id
   AND h.deleted_at IS NULL
WHERE u.clerk_id = $1
  AND u.deleted_at IS NULL;

-- name: GetUserByClerkIDForUpdate :one
SELECT
    clerk_id,
    name,
    role,
    is_active,
    deleted_at
FROM users
WHERE clerk_id = $1
FOR UPDATE;

-- name: GetHospitalBySlugForUpdate :one
SELECT
    id,
    name,
    slug,
    is_active,
    created_by_clerk_id,
    created_at,
    updated_at,
    deleted_at
FROM hospitals
WHERE slug = $1
  AND deleted_at IS NULL
FOR UPDATE;

-- name: GetActiveHospitalMembershipContextByUser :one
SELECT
    hm.hospital_id,
    hm.membership_role,
    h.slug AS hospital_slug,
    h.is_active AS hospital_is_active
FROM hospital_memberships hm
JOIN hospitals h ON h.id = hm.hospital_id
WHERE hm.user_clerk_id = $1
  AND hm.is_active = TRUE
  AND hm.deleted_at IS NULL
  AND h.deleted_at IS NULL
LIMIT 1;

-- name: DeactivateHospitalMembershipByUser :execrows
UPDATE hospital_memberships
SET
    is_active = FALSE,
    deleted_at = NOW(),
    updated_at = NOW()
WHERE user_clerk_id = $1
  AND is_active = TRUE
  AND deleted_at IS NULL;

-- name: InsertAdminUserAction :exec
INSERT INTO admin_user_actions (
    actor_clerk_id,
    target_clerk_id,
    action_type,
    reason,
    before_state,
    after_state,
    metadata,
    created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW());

-- name: ListAdminUserActionsByTarget :many
SELECT
    id,
    actor_clerk_id,
    target_clerk_id,
    action_type,
    reason,
    before_state,
    after_state,
    metadata,
    created_at
FROM admin_user_actions
WHERE target_clerk_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;
