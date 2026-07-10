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

Stage 2 mein hum checker banayenge:
- `internal/checker/` — Checker interface + HTTPChecker implementation
- context timeout per HTTP check
- defer resp.Body.Close() pattern

---

## Stage 2 — Checker (Interface + HTTP)

### What we built

| File | Purpose |
|---|---|
| `internal/checker/checker.go` | `Checker` interface + `Result` struct + helpers |
| `internal/checker/http_checker.go` | `HTTPChecker` — real HTTP GET using `net/http` |
| `cmd/try-checker/main.go` | Throwaway demo — probe real URLs and print results |

---

### The Three Files Ka Kaam

```
checker.go          → CONTRACT (interface) — kya karna hai
http_checker.go     → IMPLEMENTATION — kaise karna hai
try-checker/main.go → DEMO — dekhne ke liye ki kaam kar raha hai
```

Yeh separation important hai:
- Worker pool (Stage 6) `checker.go` import karega — sirf interface chahiye
- `http_checker.go` sirf `main.go` mein inject hoga (wiring layer)
- Tests mein `fakeChecker` banayenge jo `checker.go` ka interface satisfy kare

---

### Concept 1 — Interface as Contract

**Analogy:** Electric socket. Socket ka shape = interface. Jo bhi plug fit kare = implementation.
Socket ko parwah nahi phone charger hai ya laptop charger — bas shape match honi chahiye.

```go
// checker.go — CONTRACT (socket ka shape)
type Checker interface {
    Check(ctx context.Context, url string) Result
}

// http_checker.go — IMPLEMENTATION (actual device)
type HTTPChecker struct { client *http.Client }
func (h *HTTPChecker) Check(ctx context.Context, url string) Result { ... }

// test mein — FAKE IMPLEMENTATION (test device)
type fakeChecker struct{ result Result }
func (f fakeChecker) Check(_ context.Context, _ string) Result { return f.result }
```

**Go mein interface satisfy karne ka rule:**
Koi bhi type interface satisfy karta hai agar uske paas woh sab methods hain.
`implements` likhna nahi padta — Python/Java jaisa nahi.

```
Python:   class HTTPChecker(Checker): ...    # explicitly inherit karna padta hai
Node.js:  class HTTPChecker implements Checker  # TypeScript mein explicitly likhna padta hai
Go:       kuch nahi likhna — bas method honi chahiye — automatic ✅
```

**WHY interface?**

```
Production:  var c Checker = NewHTTPChecker(10s)  → real HTTP calls
Tests:       var c Checker = fakeChecker{...}      → no network, instant, deterministic
Worker pool: c.Check(ctx, url)                     → same code, different behavior
```

Bina interface ke tests mein real HTTP calls karni padti — flaky, slow, internet chahiye.

---

### Concept 2 — Result struct by value (not pointer)

```go
type Result struct {
    StatusCode int
    LatencyMs  int64
    Up         bool
    Err        error
}

// Returned BY VALUE — not *Result
func (h *HTTPChecker) Check(...) Result { ... }
```

**WHY value, not pointer (`*Result`)?**

```
*Result (pointer) → heap pe allocate hota hai → GC ka kaam badha
 Result (value)  → stack pe copy hota hai    → cheap (~40 bytes)

Pointer ki zaroorat tab hoti hai jab:
  - struct bahut bada ho (100+ bytes)
  - nil return karna ho (failure ka signal)

Result mein Err field hai — nil Result ki zaroorat nahi.
Caller hamesha ek valid Result pata hai.
```

```
Python:  return None ya return Result(...)  — None possible
Node.js: return null ya return result       — null possible
Go:      return Result{...}                 — hamesha valid struct, kabhi nil nahi
```

---

### Concept 3 — `defer resp.Body.Close()`

**Yeh line sabse important hai is file mein.**

```go
resp, err := h.client.Do(req)
latency := time.Since(start)
if err != nil {
    return newErrorResult(latency, err)
}
defer resp.Body.Close()   // ← HAMESHA Do() ke baad, err check ke baad
```

**Kya hota hai agar bhool gaye?**

```
Check() → 1000 baar call hua
  → 1000 TCP connections khulay
  → 0 wapas pool mein gaye (body close nahi hui)
  → OS ka file descriptor limit hit (Linux default: ~1024)
  → "too many open files" → process crash ❌
```

**`defer` kya hai?**

```go
defer resp.Body.Close()
// matlab: "jab bhi yeh function return kare — chahe koi bhi path se —
//          yeh line zaroor chalana"
```

```
Python:  with requests.get(url) as r:   → context manager auto-close karta hai
         try: ... finally: r.close()    → explicit finally block

Node.js: response.body.cancel()         → rarely done explicitly

Go:      defer resp.Body.Close()        → ek line, guaranteed, har return path pe
```

**`defer` ka execution order:**

```go
func example() {
    defer fmt.Println("3rd")   // last registered, first executed
    defer fmt.Println("2nd")
    defer fmt.Println("1st")   // first registered, last executed
}
// Output: 1st, 2nd, 3rd
// LIFO order — Last In, First Out (stack jaisa)
```

---

### Concept 4 — `http.NewRequestWithContext` vs `http.Get`

```go
// ❌ GALAT — context support nahi
resp, err := http.Get(url)

// ✅ SAHI — context se cancel/timeout ho sakta hai
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
resp, err := h.client.Do(req)
```

**Kyon farak padta hai?**

```
Worker pool mein 10 goroutines hain.
Ek goroutine http.Get(url) call karta hai.
URL 60 second tak respond nahi karta.
http.Get cancel nahi ho sakta → goroutine 60s tak STUCK rahega.

SIGTERM aaya → main.go context cancel kiya → kuch nahi hua
Goroutine abhi bhi us URL ka wait kar raha hai.

NewRequestWithContext ke saath:
SIGTERM → context cancel → http transport turant TCP tod deta hai
Goroutine free ho jaata hai → clean shutdown ✅
```

```
Python:  requests.get() — mid-flight cancel nahi ho sakta easily
         httpx.get() with asyncio.CancelledError → better
Node.js: fetch(url, { signal: abortController.signal }) → same concept as context
Go:      http.NewRequestWithContext(ctx, ...) → context cancellation built-in
```

---

### Concept 5 — `Err` field ka matlab

```go
type Result struct {
    ...
    Err error   // nil = HTTP response mila; non-nil = koi response nahi mila
}
```

**Do alag cases hain:**

```
result.Err != nil  →  Network failure — server tak pahuncha hi nahi
                       e.g., DNS error, timeout, connection refused
                       StatusCode = 0 (koi HTTP response nahi)

result.Up == false →  Server reachable hai lekin unhealthy
AND result.Err == nil   e.g., StatusCode = 503
                        Server ne response diya, bas galat code diya
```

**Real example:**

```
https://example.com        → StatusCode=200, Up=true,  Err=nil   ✅ healthy
https://example.com/broken → StatusCode=500, Up=false, Err=nil   ⚠️  reachable but broken
https://no-server.xyz      → StatusCode=0,   Up=false, Err=...   ❌ unreachable
```

---

### `io.Discard` — body drain karna

```go
defer resp.Body.Close()
io.Copy(io.Discard, resp.Body)  // body padho aur phenk do
```

Sirf `Close()` karna kaafi nahi HTTP/1.1 mein:

```
Server ne 500KB ka response body bheja.
Tum sirf status code chahte ho.
Close() bina drain ke → TCP connection reuse nahi ho sakta
                      → pool mein wapas nahi jaata
                      → next request ke liye naya connection banana padega (slow)

io.Discard = /dev/null jaisa — padho aur phenk do
io.Copy(io.Discard, body) → poora body consume karo, connection pool mein wapas
```

---

### Live Demo Output Explained

```bash
go run ./cmd/try-checker
```

```
✅ UP   example.com          status=200  latency=310ms
```
200 aaya, `IsUp(200)` = true → server healthy.

```
❌ DOWN google.com           status=301  latency=278ms
```
301 redirect aaya. Humne `CheckRedirect: return http.ErrUseLastResponse` set kiya —
redirect follow nahi kiya. 301 is not 2xx → `IsUp(301)` = false.

```
❌ DOWN httpstat.us/503      status=0    latency=3413ms | context deadline exceeded
❌ DOWN ...delay/10000       status=0    latency=0ms    | context deadline exceeded
❌ DOWN this.invalid         status=0    latency=0ms    | context deadline exceeded
⚠️  Overall context ended: context deadline exceeded
```
4-second overall context expire ho gayi. Baaki URLs ko context already cancelled milaa —
HTTP transport ne instantly error return kiya bina network call ke. `latency=0ms` proof hai
ki wire pe kuch gaya hi nahi.

**Yahi hai context cancellation ka power** — ek cancel signal poori chain mein propagate ho jaata hai.

---

### Go Gotchas in Stage 2

| Gotcha | Explanation |
|---|---|
| `http.Get` has no context | Use `http.NewRequestWithContext` always — `http.Get` can't be cancelled |
| `defer` runs at function return | Not at end of block — `defer` inside a `for` loop defers to function end, not iteration end |
| Pointer receiver on struct with mutex | `http.Client` has internal mutex — copy karna bug hai. Always use `*HTTPChecker` |
| Interface satisfied implicitly | Go mein `implements` nahi likhte — method signature match karna kaafi hai |
| `io.Copy` before `Close` | Drain body before closing otherwise HTTP/1.1 connection reuse nahi hota |
| `latency` measure timing | `time.Since(start)` `Do()` ke baad measure karo, `Close()` ke baad nahi |

---

### Self-check Questions

1. `*HTTPChecker` `Checker` interface satisfy karta hai — Go yeh kaise decide karta hai? Kya koi `implements` keyword chahiye?

2. `result.Err != nil` aur `result.Up == false` mein kya farak hai? Ek example do jahan `Err == nil` lekin `Up == false` ho.

3. `defer resp.Body.Close()` agar `http.NewRequestWithContext` ke baad aur err check se PEHLE likho toh kya hoga?

---

### What's next — Stage 3

Platform layer aur Repository:
- `internal/platform/` — PostgreSQL (GORM) + Redis + Zerolog logger connect karna
- `internal/repository/` — Monitor aur Check ke liye DB access interfaces + implementations
- GORM auto-migrate — domain structs se tables banana
- `main.go` mein logger wire karna
