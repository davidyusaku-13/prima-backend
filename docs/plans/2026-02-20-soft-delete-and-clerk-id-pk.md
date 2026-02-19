# Soft Delete + clerk_id as Primary Key — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Switch Clerk `user.deleted` webhook to soft-delete rows instead of hard-deleting them, fix the `ON CONFLICT` clause to reset soft-delete fields on re-upsert, and promote `clerk_id` to `PRIMARY KEY` by dropping the `id BIGINT SERIAL` column.

**Architecture:** Three independent changes applied in dependency order: (1) migration to restructure schema, (2) SQL query updates + sqlc regeneration, (3) Go code updates in `main.go`. No new dependencies required.

**Tech Stack:** PostgreSQL (Neon), sqlc v1.30.0, pgx/v5, golang-migrate, Go/Gin (`main.go`)

---

## Task 1: Write migration 000004 — drop `id`, promote `clerk_id` to PK

**Files:**
- Create: `backend/migrations/000004_clerk_id_pk.up.sql`
- Create: `backend/migrations/000004_clerk_id_pk.down.sql`

**Step 1: Create the up migration**

`backend/migrations/000004_clerk_id_pk.up.sql`:
```sql
ALTER TABLE users DROP COLUMN id;
ALTER TABLE users ADD PRIMARY KEY (clerk_id);
```

**Step 2: Create the down migration**

`backend/migrations/000004_clerk_id_pk.down.sql`:
```sql
ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE users ADD COLUMN id BIGSERIAL PRIMARY KEY;
```

> Note: the down migration restores the column structure but cannot restore original auto-increment values for existing rows. Acceptable for a development-stage project.

**Step 3: Run the migration against Neon**

From `backend/`:
```bash
go run ./cmd/migrate/main.go up
```

Expected output: migration version advances to 4, no errors.

**Step 4: Verify migration version**

```bash
go run ./cmd/migrate/main.go version
```

Expected: `version: 4, dirty: false`

**Step 5: Commit migration files**

```bash
git add migrations/000004_clerk_id_pk.up.sql migrations/000004_clerk_id_pk.down.sql
git commit -m "feat: promote clerk_id to primary key, drop id column"
```

---

## Task 2: Update `queries/users.sql`

**Files:**
- Modify: `backend/queries/users.sql`

**Step 1: Fix `UpsertUserWithRole` — reset soft-delete fields on conflict**

In `backend/queries/users.sql`, replace the `ON CONFLICT` clause of `UpsertUserWithRole`:

Old:
```sql
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    email      = COALESCE(EXCLUDED.email, users.email),
    updated_at = NOW();
```

New:
```sql
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    email      = COALESCE(EXCLUDED.email, users.email),
    is_active  = TRUE,
    deleted_at = NULL,
    updated_at = NOW();
```

> `role` remains intentionally absent — existing users keep their role when Clerk fires `user.updated`.

**Step 2: Remove `id` from `ListUsers` SELECT**

Replace the `ListUsers` query:

Old:
```sql
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
```

New:
```sql
-- name: ListUsers :many
SELECT
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
ORDER BY created_at DESC;
```

> `ORDER BY id DESC` is replaced with `ORDER BY created_at DESC` since `id` no longer exists.

**Step 3: Verify the full file**

The complete `backend/queries/users.sql` after changes:

```sql
-- name: UpsertUserWithRole :exec
INSERT INTO users (clerk_id, username, name, email, role, is_active, created_at, updated_at)
VALUES (
  $1, $2, $3, $4,
  CASE WHEN NOT EXISTS (SELECT 1 FROM users WHERE deleted_at IS NULL)
       THEN 'superadmin'
       ELSE 'user'
  END,
  TRUE, NOW(), NOW()
)
ON CONFLICT (clerk_id) DO UPDATE
SET username   = EXCLUDED.username,
    name       = EXCLUDED.name,
    email      = COALESCE(EXCLUDED.email, users.email),
    is_active  = TRUE,
    deleted_at = NULL,
    updated_at = NOW();

-- name: ListUsers :many
SELECT
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
ORDER BY created_at DESC;

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

## Task 3: Regenerate sqlc Go code

**Files:**
- Auto-modified: `backend/internal/db/models.go`
- Auto-modified: `backend/internal/db/querier.go`
- Auto-modified: `backend/internal/db/users.sql.go`

**Step 1: Run sqlc generate**

From `backend/`:
```bash
sqlc generate
```

Expected output: silent success (no errors).

**Step 2: Verify `models.go`**

Open `backend/internal/db/models.go` and confirm:
- `ID int64` field is **gone** from the `User` struct
- `ClerkID string` remains (it is now the PK and still NOT NULL)

Expected struct:
```go
type User struct {
    ClerkID     string             `json:"clerk_id"`
    Name        string             `json:"name"`
    Email       pgtype.Text        `json:"email"`
    Username    pgtype.Text        `json:"username"`
    CreatedAt   pgtype.Timestamptz `json:"created_at"`
    UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
    DeletedAt   pgtype.Timestamptz `json:"deleted_at"`
    Role        string             `json:"role"`
    IsActive    bool               `json:"is_active"`
    LastLoginAt pgtype.Timestamptz `json:"last_login_at"`
}
```

**Step 3: Verify `users.sql.go`**

Open `backend/internal/db/users.sql.go` and confirm:
- `ListUsersRow` no longer has an `ID int64` field
- The `rows.Scan(...)` call in `ListUsers` no longer scans `&i.ID`
- `upsertUserWithRole` SQL constant includes `is_active = TRUE, deleted_at = NULL` in the `DO UPDATE SET` clause

**Step 4: Build to check for compile errors**

```bash
go build ./...
```

Expected: exits with code 0.

> If you see a compile error about `i.ID` in `users.sql.go`, sqlc did not regenerate correctly — re-run `sqlc generate` and check the sqlc.yaml config.

---

## Task 4: Update `main.go` — swap webhook + fix `User` struct

**Files:**
- Modify: `backend/main.go`

**Step 1: Remove `ID` from the local `User` struct**

Find the `User` struct at line 25:

```go
type User struct {
	ID          int64  `json:"id"`
	ClerkID     string `json:"clerk_id,omitempty"`
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	Username    string `json:"username,omitempty"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	DeletedAt   string `json:"deleted_at,omitempty"`
	LastLoginAt string `json:"last_login_at,omitempty"`
}
```

Replace with (remove the `ID` field):

```go
type User struct {
	ClerkID     string `json:"clerk_id,omitempty"`
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	Username    string `json:"username,omitempty"`
	Role        string `json:"role"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	DeletedAt   string `json:"deleted_at,omitempty"`
	LastLoginAt string `json:"last_login_at,omitempty"`
}
```

**Step 2: Swap `user.deleted` handler to soft delete**

Find the `user.deleted` case at line 262:

```go
case "user.deleted":
    if strings.TrimSpace(evt.Data.ID) != "" {
        if err := q.DeleteUserByClerkID(c.Request.Context(), strings.TrimSpace(evt.Data.ID)); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    }
```

Replace with:

```go
case "user.deleted":
    if strings.TrimSpace(evt.Data.ID) != "" {
        if err := q.SoftDeleteUserByClerkID(c.Request.Context(), strings.TrimSpace(evt.Data.ID)); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    }
```

Only `DeleteUserByClerkID` → `SoftDeleteUserByClerkID` changes. Everything else stays identical.

**Step 3: Build to verify no compile errors**

```bash
go build ./...
```

Expected: exits with code 0, no output.

---

## Task 5: Smoke test

**Step 1: Run the server**

```bash
go run main.go
```

Expected: server starts on `:8080` with no panics.

**Step 2: Smoke test `/health`**

```bash
curl http://localhost:8080/health
```

Expected: `{"db":"up","status":"ok"}`

**Step 3: Smoke test `/users`**

```bash
curl http://localhost:8080/users
```

Expected: JSON array. Each user object should **not** contain an `id` field. Should contain `clerk_id`, `name`, `email`, `username`, `role`, `is_active`, `created_at`, `updated_at`, `deleted_at`, `last_login_at`.

---

## Task 6: Commit

```bash
git add queries/users.sql \
        internal/db/models.go \
        internal/db/querier.go \
        internal/db/users.sql.go \
        main.go
git commit -m "feat: soft-delete on Clerk user.deleted + clerk_id as PK

- Swap user.deleted webhook to SoftDeleteUserByClerkID (preserve row)
- Fix ON CONFLICT to reset is_active=TRUE, deleted_at=NULL on re-upsert
- Remove id from ListUsers SELECT (id column dropped in migration 000004)
- Remove ID field from main.go User struct"
```

---

## Summary of files touched

| File | Action |
|---|---|
| `backend/migrations/000004_clerk_id_pk.up.sql` | Create |
| `backend/migrations/000004_clerk_id_pk.down.sql` | Create |
| `backend/queries/users.sql` | Modify — fix ON CONFLICT, remove id from SELECT, reorder by created_at |
| `backend/internal/db/models.go` | Auto-generated by sqlc |
| `backend/internal/db/querier.go` | Auto-generated by sqlc |
| `backend/internal/db/users.sql.go` | Auto-generated by sqlc |
| `backend/main.go` | Modify — swap webhook handler, remove ID from User struct |
