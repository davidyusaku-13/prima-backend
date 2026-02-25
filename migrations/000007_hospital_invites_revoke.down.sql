DROP INDEX IF EXISTS hospital_invites_active_revoke_idx;

ALTER TABLE hospital_invites
    DROP COLUMN IF EXISTS revoked_by_clerk_id,
    DROP COLUMN IF EXISTS revoked_at;
