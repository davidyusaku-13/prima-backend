-- name: UpsertByClerkID :exec
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (clerk_id) DO UPDATE
SET username = EXCLUDED.username,
    name     = EXCLUDED.name,
    email    = COALESCE(EXCLUDED.email, users.email);

-- name: ListUsers :many
SELECT id, COALESCE(clerk_id, '')::text AS clerk_id, name, COALESCE(email, '')::text AS email, COALESCE(username, '')::text AS username
FROM users
ORDER BY id DESC;

-- name: DeleteUserByClerkID :exec
DELETE FROM users WHERE clerk_id = $1;
