# Users Schema — Add New Columns Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `updated_at`, `deleted_at`, `role`, `is_active`, and `last_login_at` columns to the `users` table, wire them through sqlc queries and the Go handler layer.

**Architecture:** A new migration adds the columns with safe defaults. The `queries/users.sql` file is updated to include the new columns in reads/writes, then `sqlc generate` regenerates `internal/db/`. Finally, `main.go` is updated to pass the new fields where appropriate (upsert on webhook, last_login_at on future auth touch points).

**Tech Stack:** PostgreSQL (Neon), sqlc v1.30.0, pgx/v5, golang-migrate, Go/Gin (`main.go`)

---

## Column Decisions

| Column | PG Type | Default | Nullable? | Notes |
|---|---|---|---|---|
| `updated_at` | `TIMESTAMPTZ` | `NOW()` | NOT NULL | Auto-set on insert; updated manually on upsert |
| `deleted_at` | `TIMESTAMPTZ` | — | NULL | NULL = active; non-null = soft-deleted |
| `role` | `TEXT` | `'user'` | NOT NULL | Allowed values: `user`, `admin`, `superadmin` |
| `is_active` | `BOOLEAN` | `TRUE` | NOT NULL | Explicit active flag, separate from soft delete |
| `last_login_at` | `TIMESTAMPTZ` | — | NULL | Set externally; not set by Clerk webhook |

`created_at` is **not** in the existing schema — add it in the same migration as a bonus (it is essentially free alongside `updated_at`).

---

## Task 1: Write migration 000003 (up)

**Files:**
- Create: `backend/migrations/000003_add_user_fields.up.sql`

**Step 1: Create the up migration file**

```sql
-- Add created_at with backfill for existing rows
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Add updated_at with backfill
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Add deleted_at (nullable — NULL means not deleted)
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Add role with default 'user'
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user';

-- Add is_active with default TRUE
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;

-- Add last_login_at (nullable — NULL means never logged in via this system)
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;
```

**Step 2: Verify the file was created correctly**

Check that the file exists and content looks right before proceeding.

---

## Task 2: Write migration 000003 (down)

**Files:**
- Create: `backend/migrations/000003_add_user_fields.down.sql`

**Step 1: Create the down migration file**

```sql
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS role;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE users DROP COLUMN IF EXISTS updated_at;
ALTER TABLE users DROP COLUMN IF EXISTS created_at;
```

---

## Task 3: Run the migration

**Step 1: Run up migration**

From `backend/`:
```bash
migrate -path migrations -database "$DATABASE_URL" up
```

Expected output:
```
000003/migrate (up)
```

**Step 2: Verify columns exist in the live DB**

Connect via psql or any client and run:
```sql
SELECT column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_name = 'users'
ORDER BY ordinal_position;
```

Expected: all 6 new columns appear with correct types and defaults.

---

## Task 4: Update `queries/users.sql`

**Files:**
- Modify: `backend/queries/users.sql`

**Step 1: Replace the entire file with updated queries**

```sql
-- name: UpsertByClerkID :exec
INSERT INTO users (clerk_id, username, name, email, role, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'user', TRUE, NOW(), NOW())
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

> **Note:** Two delete variants are provided — `DeleteUserByClerkID` (hard delete, used by Clerk webhook) and `SoftDeleteUserByClerkID` (for future use). The Clerk webhook continues using the hard delete to stay in sync with Clerk's own user deletion.

---

## Task 5: Regenerate sqlc Go code

**Files:**
- Auto-modified by sqlc: `backend/internal/db/models.go`, `backend/internal/db/querier.go`, `backend/internal/db/users.sql.go`

**Step 1: Run sqlc generate**

From `backend/`:
```bash
sqlc generate
```

Expected output: no errors, silent success.

**Step 2: Verify generated models.go**

Open `backend/internal/db/models.go` and confirm the `User` struct now includes the new fields:

```go
type User struct {
    ID           int64            `json:"id"`
    ClerkID      string           `json:"clerk_id"`
    Name         string           `json:"name"`
    Email        pgtype.Text      `json:"email"`
    Username     pgtype.Text      `json:"username"`
    Role         string           `json:"role"`
    IsActive     bool             `json:"is_active"`
    CreatedAt    pgtype.Timestamptz `json:"created_at"`
    UpdatedAt    pgtype.Timestamptz `json:"updated_at"`
    DeletedAt    pgtype.Timestamptz `json:"deleted_at"`
    LastLoginAt  pgtype.Timestamptz `json:"last_login_at"`
}
```

**Step 3: Verify generated querier.go**

Confirm the `Querier` interface includes the new methods `SoftDeleteUserByClerkID` and `UpdateLastLogin`.

---

## Task 6: Update `main.go` — User struct and ListUsers response

**Files:**
- Modify: `backend/main.go`

**Step 1: Update the local `User` response struct**

Find the `User` struct in `main.go` (around line 20) and replace it:

```go
type User struct {
    ID          int64   `json:"id"`
    ClerkID     string  `json:"clerk_id,omitempty"`
    Name        string  `json:"name"`
    Email       string  `json:"email,omitempty"`
    Username    string  `json:"username,omitempty"`
    Role        string  `json:"role"`
    IsActive    bool    `json:"is_active"`
    CreatedAt   string  `json:"created_at,omitempty"`
    UpdatedAt   string  `json:"updated_at,omitempty"`
    DeletedAt   string  `json:"deleted_at,omitempty"`
    LastLoginAt string  `json:"last_login_at,omitempty"`
}
```

> **Note:** The `GET /users` handler currently returns `[]db.ListUsersRow` directly (the sqlc-generated type), so the local `User` struct is not actually used in the response today. The new fields will automatically appear in `ListUsersRow` after sqlc regeneration — no manual mapping needed in the handler. Keep the local struct in sync as documentation / for future use.

**Step 2: Verify the `/users` handler needs no changes**

The handler is:
```go
r.GET("/users", func(c *gin.Context) {
    users, err := q.ListUsers(c.Request.Context())
    ...
    c.JSON(http.StatusOK, users)
})
```

Since `ListUsers` now SELECTs the new columns and sqlc regenerated `ListUsersRow` to include them, the JSON response will automatically include the new fields. **No handler change required.**

---

## Task 7: Compile and run

**Step 1: Build the Go backend**

From `backend/`:
```bash
go build ./...
```

Expected: no errors.

**Step 2: Run the server locally**

```bash
go run main.go
```

Expected: server starts on `:8080` with no panics.

**Step 3: Smoke test `/users`**

```bash
curl http://localhost:8080/users
```

Expected: JSON array where each user object includes `role`, `is_active`, `created_at`, `updated_at` fields. `deleted_at` and `last_login_at` will be `null` for existing rows until set.

**Step 4: Smoke test `/health`**

```bash
curl http://localhost:8080/health
```

Expected: `{"db":"up","status":"ok"}`

---

## Task 8: Commit

```bash
git add backend/migrations/000003_add_user_fields.up.sql \
        backend/migrations/000003_add_user_fields.down.sql \
        backend/queries/users.sql \
        backend/internal/db/models.go \
        backend/internal/db/querier.go \
        backend/internal/db/users.sql.go \
        backend/main.go
git commit -m "feat: add updated_at, deleted_at, role, is_active, last_login_at to users table"
```

---

## Summary of all files touched

| File | Action |
|---|---|
| `backend/migrations/000003_add_user_fields.up.sql` | **Create** |
| `backend/migrations/000003_add_user_fields.down.sql` | **Create** |
| `backend/queries/users.sql` | **Modify** — add new columns to SELECT, add `SoftDeleteUserByClerkID` and `UpdateLastLogin` queries |
| `backend/internal/db/models.go` | **Auto-generated** by sqlc |
| `backend/internal/db/querier.go` | **Auto-generated** by sqlc |
| `backend/internal/db/users.sql.go` | **Auto-generated** by sqlc |
| `backend/main.go` | **Modify** — update local `User` struct |
