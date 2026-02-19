# Clerk-Only User CRUD Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure users can only be created/updated/deleted in the DB through the Clerk webhook — remove the manual `POST /users` escape hatch, delete the orphan row, and enforce `clerk_id NOT NULL` at the schema level.

**Architecture:** Remove `POST /users` route and the `upsertManual()` helper from `backend/main.go`. Prune the two now-dead sqlc queries (`UpsertByEmail`, `UpsertByUsername`) from `queries/users.sql` and regenerate the DB layer. Add migration `000002` that deletes orphan rows and sets `clerk_id NOT NULL`.

**Tech Stack:** Go 1.25, Gin, sqlc v1.30, pgx/v5, golang-migrate, Neon PostgreSQL

---

### Task 1: Write migration 000002 — delete orphans + enforce NOT NULL

**Files:**
- Create: `backend/migrations/000002_require_clerk_id.up.sql`
- Create: `backend/migrations/000002_require_clerk_id.down.sql`

**Step 1: Create up migration**

`backend/migrations/000002_require_clerk_id.up.sql`:
```sql
DELETE FROM users WHERE clerk_id IS NULL OR clerk_id = '';
ALTER TABLE users ALTER COLUMN clerk_id SET NOT NULL;
```

**Step 2: Create down migration**

`backend/migrations/000002_require_clerk_id.down.sql`:
```sql
ALTER TABLE users ALTER COLUMN clerk_id DROP NOT NULL;
```

**Step 3: Run migration against Neon**

```bash
cd backend
go run ./cmd/migrate/main.go up
```

Expected output: migration version advances to 2, no errors. Verify:
```bash
go run ./cmd/migrate/main.go version
```
Expected: `version: 2, dirty: false`

**Step 4: Verify in DB** (optional sanity check)
The `smoketest@example.com` row should be gone. If you have psql access:
```sql
SELECT * FROM users;
-- Should return only davidyusaku row with a non-null clerk_id
```

---

### Task 2: Prune dead sqlc queries

**Files:**
- Modify: `backend/queries/users.sql`

**Step 1: Remove `UpsertByEmail` and `UpsertByUsername` queries**

Delete these two blocks from `backend/queries/users.sql`:
```sql
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
```

Also remove `UpsertByClerkIDReturning` since it's only called by `upsertManual()` which is being deleted:
```sql
-- name: UpsertByClerkIDReturning :one
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (clerk_id) DO UPDATE
SET username = EXCLUDED.username,
    name     = EXCLUDED.name,
    email    = COALESCE(EXCLUDED.email, users.email)
RETURNING id, COALESCE(clerk_id, '')::text AS clerk_id, name, COALESCE(email, '')::text AS email, COALESCE(username, '')::text AS username;
```

The final `queries/users.sql` should contain only 3 queries: `UpsertByClerkID`, `ListUsers`, `DeleteUserByClerkID`.

**Step 2: Regenerate sqlc**

```bash
cd backend
sqlc generate
```

Expected: no errors. Files regenerated:
- `internal/db/querier.go` — interface now has 3 methods only
- `internal/db/users.sql.go` — only 3 implementations
- `internal/db/models.go` — unchanged

---

### Task 3: Remove `POST /users` route and dead code from `main.go`

**Files:**
- Modify: `backend/main.go`

**Step 1: Remove `UpsertUserInput` struct** (lines 33–38)

Delete:
```go
type UpsertUserInput struct {
	ClerkID  string `json:"clerk_id,omitempty"`
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}
```

**Step 2: Remove `upsertManual()` function** (lines 76–113)

Delete the entire function:
```go
func upsertManual(ctx context.Context, q *db.Queries, in UpsertUserInput) (User, error) {
    ...
}
```

**Step 3: Remove `POST /users` route** (lines 254–266)

Delete:
```go
r.POST("/users", func(c *gin.Context) {
    var in UpsertUserInput
    if err := c.ShouldBindJSON(&in); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    u, err := upsertManual(c.Request.Context(), q, in)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "provide at least one of: clerk_id, username, or email"})
        return
    }
    c.JSON(http.StatusOK, u)
})
```

**Step 4: Clean up unused imports**

After removing `upsertManual()`, `net/http` may lose its only non-constant use of `http.ErrMissingFile`. Check remaining uses — `http.StatusBadRequest`, `http.StatusOK`, etc. are still used, so `net/http` stays. `context` is still used by `upsertManual`'s signature being gone — check if it's still needed elsewhere in main.go. It is (used in `pgxpool.New` context). No import changes needed.

**Step 5: Build to verify no compile errors**

```bash
cd backend
go build ./...
```

Expected: exits with code 0, no output.

---

### Task 4: Commit

```bash
cd backend
git add migrations/000002_require_clerk_id.up.sql \
        migrations/000002_require_clerk_id.down.sql \
        queries/users.sql \
        internal/db/querier.go \
        internal/db/users.sql.go \
        main.go
git commit -m "security: restrict user CRUD to Clerk webhook only

- Remove POST /users manual upsert route and upsertManual() helper
- Prune dead UpsertByEmail, UpsertByUsername, UpsertByClerkIDReturning queries
- Regenerate sqlc layer (querier now has 3 methods)
- Add migration 000002: delete orphan rows + enforce clerk_id NOT NULL"
```
