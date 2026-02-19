-- Add created_at with backfill for existing rows
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Add updated_at with backfill
-- Note: updated_at is managed by application queries (SET updated_at = NOW()), not a trigger.
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Add deleted_at (nullable — NULL means not deleted)
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Add role with constraint: only 'user', 'admin', 'superadmin' allowed
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin', 'superadmin'));

-- Add is_active with default TRUE
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;

-- Add last_login_at (nullable — NULL means never logged in via this system)
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;
