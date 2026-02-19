# sqlc Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace all inline raw SQL in `main.go` with type-safe, code-generated query functions from sqlc, using the existing `pgx/v5` driver and `golang-migrate` toolchain.

**Architecture:** Introduce a `internal/db/` package that contains sqlc-generated code (models, querier interface, typed query functions). All DB access in `main.go` is replaced by calls into this package. The `pgxpool.Pool` is still created in `main()` and passed down — no new frameworks, no DI container.

**Tech Stack:** Go 1.25, sqlc v1.x (pgx/v5 engine), pgx/v5, Neon PostgreSQL, golang-migrate (unchanged), Gin (unchanged)

---

## Prerequisites

- `sqlc` CLI must be installed: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- Verify: `sqlc version`

---

### Task 1: Install sqlc CLI and add Go dependency

**Files:**
- Modify: `backend/go.mod` (indirect — sqlc generates code that may use pgx types; no direct import of sqlc runtime needed)

**Step 1: Install sqlc CLI globally**

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

**Step 2: Verify installation**

```bash
sqlc version
```

Expected: prints a version like `v1.28.0`

**Step 3: Tidy go.mod (no changes expected yet)**

```bash
go mod tidy
```

---

### Task 2: Create `sqlc.yaml` configuration

**Files:**
- Create: `backend/sqlc.yaml`

**Step 1: Create the config file**

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries/"
    schema: "migrations/"
    gen:
      go:
        package: "db"
        out: "internal/db"
        sql_driver: "pgx/v5"
        emit_interface: true
        emit_json_tags: true
        emit_pointers_for_null_columns: false
        null_str: ""
```

Key choices:
- `engine: postgresql` — required for Postgres-specific syntax (`ON CONFLICT`, `RETURNING`, etc.)
- `sql_driver: pgx/v5` — generates code using `pgx/v5` types directly (matches existing pool)
- `emit_interface: true` — generates a `Querier` interface, useful for mocking later
- `emit_pointers_for_null_columns: false` + `null_str: ""` — nullable TEXT columns scan as empty string `""` instead of `*string`, matching the existing `COALESCE(...,'')` pattern

**Step 2: Verify config is valid**

```bash
sqlc vet
```

Expected: `"queries/" directory not found` or similar — that's fine, we haven't created queries yet.

---

### Task 3: Create the queries SQL file

**Files:**
- Create: `backend/queries/users.sql`

**Step 1: Create the queries directory and file**

Write all existing queries as named sqlc-annotated SQL. Each query corresponds to one place in `main.go` where DB access happens:

```sql
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
RETURNING id, COALESCE(clerk_id, '') AS clerk_id, name, COALESCE(email, '') AS email, COALESCE(username, '') AS username;

-- name: UpsertByEmail :one
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE
SET name     = EXCLUDED.name,
    username = COALESCE(EXCLUDED.username, users.username)
RETURNING id, COALESCE(clerk_id, '') AS clerk_id, name, COALESCE(email, '') AS email, COALESCE(username, '') AS username;

-- name: UpsertByUsername :one
INSERT INTO users (clerk_id, username, name, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (username) DO UPDATE
SET name = EXCLUDED.name
RETURNING id, COALESCE(clerk_id, '') AS clerk_id, name, COALESCE(email, '') AS email, COALESCE(username, '') AS username;

-- name: ListUsers :many
SELECT id, COALESCE(clerk_id, '') AS clerk_id, name, COALESCE(email, '') AS email, COALESCE(username, '') AS username
FROM users
ORDER BY id DESC;

-- name: DeleteUserByClerkID :exec
DELETE FROM users WHERE clerk_id = $1;
```

**Step 2: Run sqlc generate**

```bash
sqlc generate
```

Expected: creates `backend/internal/db/` with at minimum:
- `db.go`
- `models.go`
- `users.sql.go`
- `querier.go` (because `emit_interface: true`)

If there are errors, they will be SQL-level type errors — fix the SQL before proceeding.

---

### Task 4: Review generated code and fix `go.mod`

**Files:**
- Read: `backend/internal/db/models.go`
- Read: `backend/internal/db/users.sql.go`
- Read: `backend/internal/db/querier.go`
- Modify: `backend/go.mod` (run tidy)

**Step 1: Inspect generated models**

Open `backend/internal/db/models.go`. The `User` struct should look roughly like:

```go
type User struct {
    ID       int64  `json:"id"`
    ClerkID  string `json:"clerk_id"`
    Name     string `json:"name"`
    Email    string `json:"email"`
    Username string `json:"username"`
}
```

**Step 2: Inspect generated query functions**

Open `backend/internal/db/users.sql.go`. Each `-- name: QueryName :method` produces a typed function. For example:

```go
func (q *Queries) ListUsers(ctx context.Context) ([]ListUsersRow, error)
func (q *Queries) UpsertByClerkID(ctx context.Context, arg UpsertByClerkIDParams) error
```

**Step 3: Run go mod tidy**

```bash
go mod tidy
```

Expected: sqlc-generated code imports `pgx/v5` which is already in `go.mod`. No new dependencies should be added.

**Step 4: Verify the package compiles**

```bash
go build ./internal/db/...
```

Expected: exits 0 with no output.

---

### Task 5: Refactor `upsertByClerkID` in `main.go`

**Files:**
- Modify: `backend/main.go`

Replace the `upsertByClerkID` standalone function (lines 76–91) with a call to the generated `Queries` type.

**Step 1: Add import for the new internal/db package**

In the import block of `main.go`, add:

```go
"backend/internal/db"
```

**Step 2: Replace `upsertByClerkID` function**

Delete the entire `upsertByClerkID` function (lines 76–91). It will be replaced by a direct call to `q.UpsertByClerkID(...)` at the call site (Task 7).

**Step 3: Verify build still compiles (expected to fail until Task 7)**

```bash
go build ./...
```

Expected: compile error about undefined `upsertByClerkID` — that's OK, will be fixed in Task 7.

---

### Task 6: Refactor `upsertManual` in `main.go`

**Files:**
- Modify: `backend/main.go`

Replace the `upsertManual` function (lines 93–140) with a version that calls generated query functions.

**Step 1: Rewrite `upsertManual`**

The new version accepts a `*db.Queries` instead of `*pgxpool.Pool` directly:

```go
func upsertManual(ctx context.Context, q *db.Queries, in UpsertUserInput) (db.User, error) {
    name     := strings.TrimSpace(in.Name)
    email    := strings.ToLower(strings.TrimSpace(in.Email))
    username := strings.TrimSpace(in.Username)
    clerkID  := strings.TrimSpace(in.ClerkID)

    switch {
    case clerkID != "":
        return q.UpsertByClerkIDReturning(ctx, db.UpsertByClerkIDReturningParams{
            ClerkID:  nullableTrimmed(clerkID),
            Username: nullableTrimmed(username),
            Name:     name,
            Email:    nullableTrimmed(email),
        })

    case email != "":
        return q.UpsertByEmail(ctx, db.UpsertByEmailParams{
            ClerkID:  nullableTrimmed(clerkID),
            Username: nullableTrimmed(username),
            Name:     name,
            Email:    nullableTrimmed(email),
        })

    case username != "":
        return q.UpsertByUsername(ctx, db.UpsertByUsernameParams{
            ClerkID:  nullableTrimmed(clerkID),
            Username: nullableTrimmed(username),
            Name:     name,
            Email:    nullableTrimmed(email),
        })

    default:
        return db.User{}, http.ErrMissingFile
    }
}
```

> **Note on nullableTrimmed:** The existing `nullableTrimmed` returns `any`. sqlc params expect specific types. Since the columns are `TEXT` (nullable), sqlc with `pgx/v5` and `emit_pointers_for_null_columns: false` will use `pgtype.Text` or plain `string`. Check the generated param structs and adjust accordingly. If params use `pgtype.Text`, pass via `pgtype.Text{String: v, Valid: v != ""}`.

**Step 2: Also update the `User` type reference in main.go**

The `User` struct defined in `main.go` (lines 22–28) and `UpsertUserInput` (lines 30–35) are kept as-is — they are used for JSON binding and response. The generated `db.User` is the internal return type. If you want to unify, delete the `User` struct in `main.go` and use `db.User` everywhere, adjusting JSON tags if needed.

**Step 3: Verify build**

```bash
go build ./...
```

May still fail if call sites haven't been updated — proceed to Task 7.

---

### Task 7: Update all call sites in `main.go` route handlers

**Files:**
- Modify: `backend/main.go`

**Step 1: Instantiate `*db.Queries` in `main()`**

After the pool is created and pinged, add:

```go
q := db.New(db)  // NOTE: variable name clash — rename pool to `pool`
```

Rename `db` (the pool variable) to `pool` throughout `main()` to avoid collision with the imported `db` package:

```go
pool, err := pgxpool.New(ctx, dsn)
// ...
if err := pool.Ping(ctx); err != nil { ... }
// ...
q := db.New(pool)
```

**Step 2: Update the health check handler**

```go
r.GET("/health", func(c *gin.Context) {
    var v int
    if err := pool.QueryRow(c.Request.Context(), "SELECT 1").Scan(&v); err != nil {
        c.JSON(http.StatusOK, gin.H{"status": "degraded", "db": "down"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "up"})
})
```

(Health check uses `SELECT 1` which is not a domain query — keeping it as a raw pool call is fine and intentional.)

**Step 3: Update the `GET /users` handler**

```go
r.GET("/users", func(c *gin.Context) {
    users, err := q.ListUsers(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, users)
})
```

**Step 4: Update the `POST /users` handler**

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

**Step 5: Update the `POST /webhooks/clerk` handler**

The `upsertByClerkID` call site (line 344) becomes:

```go
case "user.created", "user.updated":
    if strings.TrimSpace(evt.Data.ID) == "" {
        c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": "missing id", "type": evt.Type})
        return
    }
    err := q.UpsertByClerkID(c.Request.Context(), db.UpsertByClerkIDParams{
        ClerkID:  nullableTrimmed(evt.Data.ID),
        Username: nullableTrimmed(evt.Data.Username),
        Name:     name,
        Email:    nullableTrimmed(email),
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
case "user.deleted":
    if strings.TrimSpace(evt.Data.ID) != "" {
        if err := q.DeleteUserByClerkID(c.Request.Context(), evt.Data.ID); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    }
```

**Step 6: Verify build compiles cleanly**

```bash
go build ./...
```

Expected: exits 0 with no output. Fix any type mismatches reported by the compiler.

---

### Task 8: Clean up `main.go` — remove unused `User` struct if unified

**Files:**
- Modify: `backend/main.go`

**Step 1: Decide whether to remove the hand-written `User` struct**

If `db.User` (generated) has compatible JSON tags (`json:"id"`, `json:"clerk_id"`, etc.), delete the hand-written `User` struct (lines 22–28) from `main.go` and replace all usages with `db.User`.

If JSON tags differ (e.g., generated struct uses different field names), keep both or add a mapping function.

**Step 2: Final build and vet**

```bash
go build ./...
go vet ./...
```

Expected: both exit 0.

---

### Task 9: Smoke test the running server

**Files:** none

**Step 1: Start the server**

```bash
go run ./main.go
```

Expected: `[GIN-debug] Listening and serving HTTP on :8080`

**Step 2: Hit the health endpoint**

```bash
curl http://localhost:8080/health
```

Expected: `{"db":"up","status":"ok"}`

**Step 3: Hit the users list endpoint**

```bash
curl http://localhost:8080/users
```

Expected: a JSON array (may be empty `[]` or contain existing users).

**Step 4: Create a user**

```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"name":"Test User","email":"test@example.com"}'
```

Expected: a JSON object with `id`, `name`, `email` populated.

**Step 5: Delete the test user via a manual DB query or re-list to confirm persistence.**

---

### Task 10: Update `go.mod` and commit

**Step 1: Tidy dependencies**

```bash
go mod tidy
```

**Step 2: Verify `internal/db/` is in version control**

The generated files in `internal/db/` **should be committed** (sqlc convention — generated code is checked in, not gitignored, so the build doesn't require sqlc at runtime).

**Step 3: Commit**

```bash
git add sqlc.yaml queries/ internal/db/ main.go go.mod go.sum
git commit -m "feat: replace inline raw SQL with sqlc-generated type-safe queries"
```

---

## File Map After Migration

```
backend/
├── cmd/migrate/main.go          ← unchanged
├── docs/plans/                  ← this file
├── internal/
│   └── db/
│       ├── db.go                ← generated: Queries struct, New()
│       ├── models.go            ← generated: User struct
│       ├── querier.go           ← generated: Querier interface
│       └── users.sql.go         ← generated: all typed query functions
├── migrations/
│   ├── 000001_users_bootstrap.up.sql    ← unchanged (sqlc reads this)
│   └── 000001_users_bootstrap.down.sql  ← unchanged
├── queries/
│   └── users.sql                ← NEW: sqlc-annotated SQL source
├── main.go                      ← modified: imports internal/db, uses q.*
├── go.mod                       ← modified: go mod tidy'd
├── go.sum                       ← modified
└── sqlc.yaml                    ← NEW: sqlc config
```

---

## Gotchas to Watch For

1. **`nullableTrimmed` return type** — it returns `any`. sqlc-generated param structs use typed fields. You'll need to either use `pgtype.Text` for nullable TEXT params or change the helper to return `pgtype.Text`. Check the generated `*Params` structs before wiring up call sites.

2. **`db` variable name collision** — the local variable `db` in `main()` (the pool) will collide with the imported `"backend/internal/db"` package. Rename the pool variable to `pool` before instantiating `db.New(pool)`.

3. **`COALESCE` in SELECT** — sqlc may or may not infer the output column as non-null depending on the Postgres version/inference. If the generated `ListUsersRow.ClerkID` is `pgtype.Text` instead of `string`, either adjust the SQL to use `COALESCE(clerk_id, '')::text` with an explicit cast, or set `emit_pointers_for_null_columns: false` in `sqlc.yaml` (already set in this plan).

4. **`emit_interface: true`** — generates a `Querier` interface. This is useful for future mocking in tests but adds no overhead at runtime.

5. **Schema idempotency** — `migrations/000001_users_bootstrap.up.sql` uses `IF NOT EXISTS` and `ADD COLUMN IF NOT EXISTS` which sqlc parses fine. No changes needed to the migration file.
