-- name: UpsertByClerkID :exec
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (clerk_id) DO UPDATE
SET username = EXCLUDED.username,
    name     = EXCLUDED.name,
    email    = COALESCE(EXCLUDED.email, users.email);

-- name: UpsertByClerkIDReturning :one
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (clerk_id) DO UPDATE
SET username = EXCLUDED.username,
    name     = EXCLUDED.name,
    email    = COALESCE(EXCLUDED.email, users.email)
RETURNING id, COALESCE(clerk_id, '')::text AS clerk_id, name, COALESCE(email, '')::text AS email, COALESCE(username, '')::text AS username;

-- name: UpsertByEmail :one
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE
SET name     = EXCLUDED.name,
    username = COALESCE(EXCLUDED.username, users.username)
RETURNING id, COALESCE(clerk_id, '')::text AS clerk_id, name, COALESCE(email, '')::text AS email, COALESCE(username, '')::text AS username;

-- name: UpsertByUsername :one
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (username) DO UPDATE
SET name = EXCLUDED.name
RETURNING id, COALESCE(clerk_id, '')::text AS clerk_id, name, COALESCE(email, '')::text AS email, COALESCE(username, '')::text AS username;

-- name: ListUsers :many
SELECT id, COALESCE(clerk_id, '')::text AS clerk_id, name, COALESCE(email, '')::text AS email, COALESCE(username, '')::text AS username
FROM users
ORDER BY id DESC;

-- name: DeleteUserByClerkID :exec
DELETE FROM users WHERE clerk_id = $1;
