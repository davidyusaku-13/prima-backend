CREATE TABLE IF NOT EXISTS hospitals (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by_clerk_id TEXT NOT NULL REFERENCES users(clerk_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS hospitals_active_idx ON hospitals(is_active, deleted_at);

CREATE TABLE IF NOT EXISTS hospital_memberships (
    id BIGSERIAL PRIMARY KEY,
    hospital_id BIGINT NOT NULL REFERENCES hospitals(id),
    user_clerk_id TEXT NOT NULL REFERENCES users(clerk_id),
    membership_role TEXT NOT NULL CHECK (membership_role IN ('admin', 'user')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    invited_by_clerk_id TEXT REFERENCES users(clerk_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS hospital_memberships_user_active_uq
    ON hospital_memberships(user_clerk_id);
CREATE INDEX IF NOT EXISTS hospital_memberships_hospital_idx
    ON hospital_memberships(hospital_id, membership_role, is_active, deleted_at);
CREATE INDEX IF NOT EXISTS hospital_memberships_user_idx
    ON hospital_memberships(user_clerk_id, is_active, deleted_at);

CREATE TABLE IF NOT EXISTS hospital_invites (
    id BIGSERIAL PRIMARY KEY,
    hospital_id BIGINT NOT NULL REFERENCES hospitals(id),
    token_hash TEXT NOT NULL UNIQUE,
    invite_role TEXT NOT NULL CHECK (invite_role IN ('user')),
    created_by_clerk_id TEXT NOT NULL REFERENCES users(clerk_id),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    consumed_by_clerk_id TEXT REFERENCES users(clerk_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS hospital_invites_hospital_idx
    ON hospital_invites(hospital_id, expires_at, consumed_at);
