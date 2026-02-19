DELETE FROM users WHERE clerk_id IS NULL OR clerk_id = '';
ALTER TABLE users ALTER COLUMN clerk_id SET NOT NULL;
