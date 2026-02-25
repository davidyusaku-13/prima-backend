ALTER TABLE hospital_invites
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS revoked_by_clerk_id TEXT REFERENCES users(clerk_id);

CREATE INDEX IF NOT EXISTS hospital_invites_active_revoke_idx
    ON hospital_invites(hospital_id, revoked_at, consumed_at, expires_at);
