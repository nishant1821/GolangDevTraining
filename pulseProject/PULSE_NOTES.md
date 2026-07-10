# Pulse — Learning Notes

> One file that grows with every stage. Each section explains WHAT we built,
> WHY we built it that way, and how it maps to Python / Node.js concepts you know.

---

## Stage 1 — Skeleton, Config & Domain

### What we built

| File | Purpose |
|---|---|
| `go.mod` | Declares the module name and Go version (like `package.json` or `pyproject.toml`) |
| `config/config.go` | Loads settings from env vars with sensible defaults |
| `internal/domain/models.go` | Core data types: Monitor, Check, User, Incident |
| `internal/domain/errors.go` | Named sentinel errors for every failure mode |
| `cmd/server/main.go` | Wires everything, starts chi server, handles graceful shutdown |
| `.env.example` | Template of all env vars — copy to `.env` for local dev |

---

### The Folder Layout — Clean Architecture

```
pulseProject/
├── cmd/
│   └── server/
│       └── main.go          ← entry point (wiring only, minimal logic)
│
├── config/
│   └── config.go            ← env-var config (no HTTP, no DB)
│
├── internal/                ← Go enforces: code in internal/ can only be
│   │                            imported by code in the SAME module
│   ├── domain/              ← Layer 0: pure Go structs + errors. imports NOTHING
│   │   ├── models.go
│   │   └── errors.go
│   │
│   ├── repository/          ← Layer 1: DB access (GORM). imports domain
│   ├── service/             ← Layer 2: business logic. imports repository + domain
│   ├── handler/             ← Layer 3: HTTP. imports service + domain
│   │
│   ├── checker/             ← does HTTP pings (context timeout per check)
│   ├── monitor/             ← scheduler + worker pool
│   ├── middleware/          ← JWT auth, request-id, rate-limit
│   └── platform/            ← DB/Redis/logger bootstrap
```

#### The Dependency Rule (most important thing in clean architecture)

```
cmd/server/main.go
       ↓
   handler   ←── middleware
       ↓
   service
       ↓
  repository
       ↓
    domain          ← imports NOTHING from this project
```

Arrows point **downward only**. `handler` can call `service`, but `service`
can NEVER import `handler`. `domain` can never import anything from this project.

**Why?**
- You can swap out the DB (GORM → raw SQL) without touching handlers or services.
- You can unit-test services without starting an HTTP server.
- You can test repositories without registering routes.
- Each layer only knows the abstraction below it — not the concrete implementation.

Python analogy: Django's three-layer cake: views.py → services.py → models.py,
where models.py has no Django view imports.

Node.js analogy: controllers → services → repositories, where the repository
is injected via the constructor (Dependency Injection).

---

### Concept 1 — Structs & Tags

**Analogy:** A Go struct is like a Python dataclass or a TypeScript interface,
but it can carry *annotations* called tags that other libraries read at runtime.

```go
type Monitor struct {
    ID  uint   `gorm:"primarykey" json:"id"`
    URL string `gorm:"not null"   json:"url"`
}
```

The backtick strings after field names are **struct tags**.
- `gorm:"primarykey"` → tells GORM: use this as the PRIMARY KEY column.
- `json:"id"` → tells `encoding/json`: serialise this field as `"id"` in JSON.
- `json:"-"` → **never** include this field in any JSON output (used for Password).
- `json:"error,omitempty"` → omit the key entirely when the value is the zero value
  (empty string, 0, nil, false).

```
Python equivalent:
  @dataclass
  class Monitor:
      id: int = field(metadata={"json": "id"})   # no built-in tag system; libraries vary

Node.js equivalent:
  // TypeScript decorators (@Column(), @PrimaryKey()) do the same thing.
  // Plain JS has no struct tags — you handle serialisation manually.
```

**Key gotcha:** struct tags must be spelled EXACTLY right. A typo like
`json: "id"` (space after colon) is silently ignored — the field will use its
Go name in JSON output and you'll wonder why your API returns `"ID"` not `"id"`.

---

### Concept 2 — The `*time.Time` Pointer for Soft Delete

```go
DeletedAt *time.Time `gorm:"index" json:"-"`
```

A pointer (`*time.Time`) can be **nil** — which maps to SQL NULL.
When GORM sees a field named `DeletedAt` of type `*time.Time`, it:
1. Sets this field to `NOW()` instead of running DELETE.
2. Adds `WHERE deleted_at IS NULL` to every SELECT automatically.

So records are "deleted" logically, not physically. You can recover them.

```
Python/Django:  Field(null=True, blank=True) + django-safedelete library
Node.js/Sequelize: { paranoid: true } in model definition
Go/GORM:        *time.Time field named DeletedAt — GORM handles it automatically
```

Why does domain/models.go use `*time.Time` instead of `gorm.DeletedAt`?
→ `gorm.DeletedAt` is a type from the `gorm.io/gorm` package. Importing it would
mean domain/ depends on gorm — breaking the clean-architecture rule that domain
imports nothing external. `*time.Time` achieves the same soft-delete behaviour
using only the standard library.

---

### Concept 3 — Sentinel Errors

**Analogy:** Sentinel errors are like Python's built-in `FileNotFoundError` vs
creating a new `Exception("file not found")` string every time. The named
exception can be caught by type; the raw string cannot.

```go
// domain/errors.go
var ErrNotFound = errors.New("not found")

// repository layer
func (r *monitorRepo) FindByID(ctx context.Context, id uint) (*domain.Monitor, error) {
    var m domain.Monitor
    if err := r.db.First(&m, id).Error; err != nil {
        return nil, fmt.Errorf("FindByID: %w", domain.ErrNotFound)  // wrap with %w
    }
    return &m, nil
}

// handler layer
err := svc.GetMonitor(ctx, id)
if errors.Is(err, domain.ErrNotFound) {   // unwraps through the %w chain
    http.Error(w, "not found", 404)
    return
}
```

`%w` in `fmt.Errorf` **wraps** the error — the original error is preserved
inside the new error. `errors.Is()` unwraps the chain to check if any error
in the chain matches `domain.ErrNotFound`.

```
Python: raise FileNotFoundError("x")  →  except FileNotFoundError
Node.js: throw new NotFoundError()    →  catch(e) { if (e instanceof NotFoundError) }
Go:      return fmt.Errorf("...: %w", domain.ErrNotFound)  →  errors.Is(err, domain.ErrNotFound)
```

Why define them in ONE place (domain)?
→ If `repository` defines `ErrNotFound` and `service` defines its own `ErrNotFound`,
they are two different values. `errors.Is()` compares by identity — the two would
NOT match. One canonical definition means every layer uses the same value.

---

### Concept 4 — Graceful Shutdown

```
SIGTERM / SIGINT
      ↓
  <-quit (channel receive — blocks until signal)
      ↓
  context.WithTimeout(10s)
      ↓
  srv.Shutdown(ctx)
      ↓
  Stop accepting new connections
  Wait for in-flight requests to finish (up to 10s)
      ↓
  main() returns → process exits 0
```

Why does this matter?
Without graceful shutdown, a Kubernetes pod restart drops every request that
was being processed mid-flight. With graceful shutdown, those requests complete,
clients get proper responses, and only THEN does the process exit.

```
Python (uvicorn): uvicorn handles SIGTERM gracefully by default
Node.js:          server.close(callback) — stops new connections, waits for active ones
Go:               srv.Shutdown(ctx) — explicit, timeout-bounded
```

---

### Concept 5 — Buffered Channel for Signals

```go
quit := make(chan os.Signal, 1)   // buffered, size 1
signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
<-quit
```

Why `make(chan os.Signal, 1)` and not `make(chan os.Signal)`?

If the OS delivers the signal BEFORE main() reaches `<-quit`, an **unbuffered**
channel has no one reading from it — the signal is dropped and the process
never shuts down.

A **buffered** channel of size 1 stores the signal in its internal buffer even
if no one is reading yet. When `<-quit` is reached, it picks up the stored signal.

Python has no direct equivalent — its signal handlers are function callbacks, not channels.
Node.js `process.on('SIGTERM', fn)` is also callback-based — the runtime queues the callback.

---

### How to run it

```bash
# Option A: set env var inline (single run, not persisted)
PORT=9090 go run ./cmd/server/main.go

# Option B: export (persisted for the shell session)
export PORT=9090
go run ./cmd/server/main.go

# Option C: copy .env.example and source it
cp .env.example .env
# edit .env as needed
set -a && source .env && set +a
go run ./cmd/server/main.go

# Test it
curl http://localhost:9090/health
# → {"status":"ok","service":"pulse"}

# Stop it
Ctrl+C  (sends SIGINT → graceful shutdown)
```

---

### Go Gotchas in Stage 1

| Gotcha | Explanation |
|---|---|
| Headers before `WriteHeader` | Once `w.WriteHeader(code)` is called, headers are locked. Set `Content-Type` first. |
| Struct tag typo | `json: "id"` (space after colon) silently fails — field uses Go name instead. |
| Panic in goroutine | A panic in a goroutine kills the process unless `recover()` is called. `chiMW.Recoverer` does this for handler goroutines. |
| Unbuffered signal channel | An unbuffered signal channel can miss signals. Always use capacity 1. |
| `http.ErrServerClosed` | When you call `srv.Shutdown()`, `ListenAndServe()` returns this error. It's NOT a real error — check for it explicitly. |
| `defer cancel()` | Always defer the cancel from `context.WithTimeout`. Forgetting leaks the timer goroutine. |

---

### Self-check Questions

1. **Why does `domain/models.go` import only `"time"` and nothing from `gorm.io/gorm`?**
   Hint: what would happen to unit tests if domain imported gorm?

2. **What is a sentinel error and why is `errors.Is()` better than comparing error strings?**
   Hint: what does `%w` in `fmt.Errorf` do to the error chain?

3. **Why is `make(chan os.Signal, 1)` buffered with size 1 instead of unbuffered?**
   Hint: what happens if the OS delivers SIGTERM before `<-quit` is reached?

---

### What's next — Stage 2

We'll bring the database to life:
- `internal/platform/` — connect to PostgreSQL via GORM and Redis via go-redis
- `internal/repository/` — `MonitorRepository` and `UserRepository` interfaces + GORM implementations
- GORM auto-migrate to create the tables from our domain structs
- Zerolog structured logger wired into `main.go`
