# First-User Superadmin Bootstrap — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** When the first user registers via Clerk, automatically assign them `role = 'superadmin'`; all subsequent users get `role = 'user'`.

**Architecture:** Replace the existing `UpsertByClerkID` sqlc query with a new `UpsertUserWithRole` query that uses an atomic `CASE` subquery to determine the role at insert time. Run `sqlc generate` to regenerate the Go layer, then update the single call site in `main.go`.

**Tech Stack:** PostgreSQL (Neon), sqlc v1.30.0, pgx/v5, Go/Gin (`main.go`)

---

## Task 1: Update `queries/users.sql`

**Files:**
- Modify: `backend/queries/users.sql`

**Step 1: Replace `UpsertByClerkID` with `UpsertUserWithRole`**

Open `backend/queries/users.sql` and replace the first query block (lines 1–8):

```sql
-- name: UpsertByClerkID :exec
INSERT INTO users (clerk_id, username, name, email, role, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'user', TRUE, NOW(), NOW())
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    email      = COALESCE(EXCLUDED.email, users.email),
    updated_at = NOW();
```

with:

```sql
-- name: UpsertUserWithRole :exec
INSERT INTO users (clerk_id, username, name, email, role, is_active, created_at, updated_at)
VALUES (
  $1, $2, $3, $4,
  CASE WHEN (SELECT COUNT(*) FROM users WHERE deleted_at IS NULL) = 0
       THEN 'superadmin'
       ELSE 'user'
  END,
  TRUE, NOW(), NOW()
)
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    email      = COALESCE(EXCLUDED.email, users.email),
    updated_at = NOW();
```

> Note: `role` is intentionally absent from `DO UPDATE SET` — existing users keep their current role when Clerk fires `user.updated`.

**Step 2: Verify the file looks correct**

The full file after the change should be:

```sql
-- name: UpsertUserWithRole :exec
INSERT INTO users (clerk_id, username, name, email, role, is_active, created_at, updated_at)
VALUES (
  $1, $2, $3, $4,
  CASE WHEN (SELECT COUNT(*) FROM users WHERE deleted_at IS NULL) = 0
       THEN 'superadmin'
       ELSE 'user'
  END,
  TRUE, NOW(), NOW()
)
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    email      = COALESCE(EXCLUDED.email, users.email),
    updated_at = NOW();

-- name: ListUsers :many
SELECT
    id,
    COALESCE(clerk_id, '')::text      AS clerk_id,
    name,
    COALESCE(email, '')::text         AS email,
    COALESCE(username, '')::text      AS username,
    role,
    is_active,
    created_at,
    updated_at,
    deleted_at,
    last_login_at
FROM users
ORDER BY id DESC;

-- name: DeleteUserByClerkID :exec
DELETE FROM users WHERE clerk_id = $1;

-- name: SoftDeleteUserByClerkID :exec
UPDATE users
SET deleted_at = NOW(), is_active = FALSE, updated_at = NOW()
WHERE clerk_id = $1;

-- name: UpdateLastLogin :exec
UPDATE users
SET last_login_at = NOW(), updated_at = NOW()
WHERE clerk_id = $1;
```

---

## Task 2: Regenerate sqlc Go code

**Files:**
- Auto-modified: `backend/internal/db/users.sql.go`
- Auto-modified: `backend/internal/db/querier.go`

**Step 1: Run sqlc generate**

From `backend/`:
```bash
sqlc generate
```

Expected output: silent success (no errors printed).

**Step 2: Verify `users.sql.go`**

Open `backend/internal/db/users.sql.go` and confirm:
- `UpsertByClerkID` and `UpsertByClerkIDParams` are **gone**
- `UpsertUserWithRole` function exists
- `UpsertUserWithRoleParams` struct has the same 4 fields:
  ```go
  type UpsertUserWithRoleParams struct {
      ClerkID  string      `json:"clerk_id"`
      Username pgtype.Text `json:"username"`
      Name     string      `json:"name"`
      Email    pgtype.Text `json:"email"`
  }
  ```

**Step 3: Verify `querier.go`**

Open `backend/internal/db/querier.go` and confirm:
- `UpsertByClerkID` is **gone** from the interface
- `UpsertUserWithRole(ctx context.Context, arg UpsertUserWithRoleParams) error` is present

---

## Task 3: Update `main.go` call site

**Files:**
- Modify: `backend/main.go`

**Step 1: Replace the call in the webhook handler**

In `backend/main.go`, find the `user.created` / `user.updated` case (around line 253):

```go
if err := q.UpsertByClerkID(c.Request.Context(), db.UpsertByClerkIDParams{
    ClerkID:  strings.TrimSpace(evt.Data.ID),
    Username: toText(evt.Data.Username),
    Name:     name,
    Email:    toText(email),
}); err != nil {
```

Replace with:

```go
if err := q.UpsertUserWithRole(c.Request.Context(), db.UpsertUserWithRoleParams{
    ClerkID:  strings.TrimSpace(evt.Data.ID),
    Username: toText(evt.Data.Username),
    Name:     name,
    Email:    toText(email),
}); err != nil {
```

Only the function name and params type name change. Everything else (args, error handling, surrounding code) stays identical.

---

## Task 4: Build and verify

**Step 1: Build the Go backend**

From `backend/`:
```bash
go build ./...
```

Expected: no errors, no output.

**Step 2: Run the server**

```bash
go run main.go
```

Expected: server starts on `:8080` with no panics.

**Step 3: Smoke test `/health`**

```bash
curl http://localhost:8080/health
```

Expected: `{"db":"up","status":"ok"}`

**Step 4: Smoke test `/users`**

```bash
curl http://localhost:8080/users
```

Expected: JSON array. Any existing users should still have their current `role` values.

---

## Task 5: Commit

```bash
git add backend/queries/users.sql \
        backend/internal/db/users.sql.go \
        backend/internal/db/querier.go \
        backend/main.go
git commit -m "feat: promote first registered user to superadmin atomically"
```

---

## Summary of files touched

| File | Action |
|---|---|
| `backend/queries/users.sql` | Modify — replace `UpsertByClerkID` with `UpsertUserWithRole` |
| `backend/internal/db/users.sql.go` | Auto-generated by sqlc |
| `backend/internal/db/querier.go` | Auto-generated by sqlc |
| `backend/main.go` | Modify — update webhook handler call site (2 word changes) |
