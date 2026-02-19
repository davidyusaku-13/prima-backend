CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    clerk_id TEXT UNIQUE,
    name TEXT NOT NULL,
    email TEXT,
    username TEXT UNIQUE
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS clerk_id TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS username TEXT;
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS users_clerk_id_uq ON users(clerk_id);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_uq ON users(username);
