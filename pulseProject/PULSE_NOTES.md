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

Worker Pool:
- `internal/monitor/pool.go` — bounded worker pool: goroutines, `sync.WaitGroup`, fan-out/fan-in
- `internal/monitor/pool_test.go` — race-clean test: 20 jobs, 5 workers
- Context cancellation guard — goroutine leak prevention

---

## Stage 3 — Worker Pool (Concurrency Core)

### What we built

| File | Purpose |
|---|---|
| `internal/monitor/pool.go` | `RunPool` — bounded worker pool, fan-out/fan-in, graceful shutdown |
| `internal/monitor/pool_test.go` | `fakeChecker` + two tests: happy path (20 jobs / 5 workers) + cancel |

---

### The Big Picture — Pool ka Pulse mein kaam

```
DB mein 1000 monitors hain
         │
         ▼
    Scheduler (Stage 4 mein banega)
    "kaunse monitors ka check time aa gaya?"
         │  Job{MonitorID:5, URL:"https://swiggy.com"} bhejta hai
         ▼
   [jobs channel]  ←── producer (scheduler)
         │
         ▼  FAN-OUT
   Worker 0 → swiggy.com check
   Worker 1 → zomato.com check      5 goroutines, ek saath
   Worker 2 → razorpay.com check
         │
         ▼  FAN-IN
   [out channel]  ──► Result Handler (Stage 5 mein banega)
                       DB mein save, incident open/close, alert bhejo
```

**Pool ke bina:** 1000 monitors × 200ms = 200 seconds. Ek minute mein khatam nahi hoga.
**Pool ke saath (10 workers):** 1000 ÷ 10 × 200ms = 20 seconds. ✅

---

### Concept 1 — Goroutine

```go
go func() {
    // yeh code ek alag goroutine mein chalega
}()
```

**Goroutine** = Go ka ultra-light thread.

```
OS Thread:    ~1 MB stack, OS manage karta hai, context switch expensive
Goroutine:    ~2 KB stack, Go runtime manage karta hai, context switch cheap

1000 OS threads = ~1 GB RAM + slow
1000 goroutines = ~2 MB RAM + fast ✅
```

```
Python:   asyncio.create_task(coro())  — event loop pe schedule karta hai
Node.js:  JS single-threaded hai — I/O concurrency event loop se aata hai,
          CPU work ke liye worker_threads
Go:       go func(){}()               — real parallel execution (GOMAXPROCS CPUs)
```

**Key difference:** Python/Node ka concurrency cooperative hai (ek kaam manually yield karta hai). Go goroutines preemptive hain — runtime forcibly switch kar sakta hai. Real parallelism milta hai multi-core machines pe.

---

### Concept 2 — Channel (goroutines ke beech data bhejne ka rasta)

```go
jobs := make(chan Job, 100)   // buffered channel
out  := make(chan Outcome, 5) // buffered channel

// ek goroutine mein
jobs <- Job{URL: "https://example.com"}   // SEND

// doosri goroutine mein
job := <-jobs                              // RECEIVE
```

**Channel rules:**
```
Unbuffered channel:  send BLOCK karta hai jab tak koi receive na kare (aur ulta)
Buffered channel:    send BLOCK karta hai sirf jab buffer FULL ho

Channel close karna: close(ch)
  - Baad mein send → PANIC ❌
  - Receive pe: value milegi (agar buffer mein kuch hai), phir zero value + ok=false
  - range ch: automatically ruk jaata hai jab channel close ho
```

```
Python:   asyncio.Queue()    — async get() / put()
Node.js:  EventEmitter ya    — stream.write() / stream.on('data')
          Readable streams
Go:       channel            — built-in language feature, type-safe, goroutine-safe
```

**Sabse important rule:** Channel **sirf woh goroutine close kare** jo producer hai — jisne data daala. Multiple goroutines close karein toh PANIC.

---

### Concept 3 — `sync.WaitGroup` (N goroutines ka wait)

```go
var wg sync.WaitGroup

wg.Add(5)        // counter = 5 (workers launch karne se PEHLE)

for i := 0; i < 5; i++ {
    go func() {
        defer wg.Done()  // counter-- jab goroutine exit kare
        // kaam karo
    }()
}

wg.Wait()        // BLOCK karo jab tak counter 0 na ho
```

```
Python:   asyncio.gather(*tasks)   — await karo sab tasks ka
Node.js:  Promise.all(promises)    — resolve hoga jab sab settle ho
Go:       sync.WaitGroup           — explicit manual counter, low-level
```

**`wg.Add(N)` goroutines launch karne se PEHLE kyun?**
```
Agar loop ke andar wg.Add(1) karo aur goroutine wg.Done() pehle call kar le
toh counter negative ho sakta hai → PANIC.
Safe pattern: ek baar wg.Add(totalWorkers) phir launch karo.
```

---

### Concept 4 — Fan-out aur Fan-in

```
Fan-out: 1 channel → N readers
         Har Job exactly EK worker ko milta hai (channel guarantee)
         Restaurant mein: order board → 5 cooks mein se koi ek uthata hai

Fan-in:  N writers → 1 channel
         Sab workers ek hi `out` channel mein likhte hain
         Caller ko ek clean merged stream milta hai
         Restaurant mein: sab cooks serving counter pe plate rakhte hain
```

```go
// FAN-OUT: ek jobs channel, N workers
for i := 0; i < workers; i++ {
    go func() {
        for { select { case job := <-jobs: ... } }
    }()
}

// FAN-IN: N workers, ek out channel
case out <- Outcome{Job: job, Result: result}:
```

```
Python:  asyncio mein:  asyncio.Queue() se multiple workers consume karte hain
Node.js: Node streams:  readable.pipe() → multiple writable destinations
Go:      channels:      built-in, type-safe fan-out/fan-in
```

---

### Concept 5 — `ctx.Done()` guard — Goroutine Leak rokna

**Goroutine leak kya hota hai:**

```
Worker jobs channel pe wait kar raha hai...
No more jobs aayenge (scheduler band ho gaya)
jobs channel close bhi nahi hua...
ctx cancel bhi nahi hua...
Worker HAMESHA ke liye memory mein phansa = GOROUTINE LEAK
```

**Leak ke 2 jagah pool mein:**

```go
// Jagah 1: job ka wait karte waqt
select {
case job, ok := <-jobs:   // kaam mila → karo
    if !ok { return }     // channel closed → ghar jao
case <-ctx.Done():        // ← YEH GUARD hai — ctx cancel → ghar jao
    return
}

// Jagah 2: outcome send karte waqt
select {
case out <- Outcome{...}: // send successful → agle job pe jao
case <-ctx.Done():        // ← YEH GUARD hai — consumer band ho gaya → ghar jao
    return
}
```

**`ctx.Done()` kya hai?**
```go
ctx.Done()  // ek channel hai jo CLOSE ho jaata hai jab ctx cancel hoti hai
            // closed channel se receive karna immediately return karta hai
            // isliye select mein yeh "signal" ki tarah kaam karta hai
```

```
Python asyncio: task.cancel() → CancelledError raise hoti hai coroutine mein
Node.js:        AbortSignal.aborted → true ho jaata hai, event fire hota hai
Go:             ctx.Done() channel close hota hai → select case trigger hota hai
```

---

### Concept 6 — Closer Goroutine (channel safely close karna)

**Problem:** `close(out)` kab karein?

```
Option 1: RunPool mein wg.Wait() phir close(out)

  RunPool → wg.Wait() pe BLOCK
  Caller ko `out` nahi mila abhi tak
  Caller read nahi kar sakta
  Workers `out` pe BLOCK (full ya unbuffered)
  wg.Done() nahi bolta
  wg.Wait() kabhi return nahi karta
  = DEADLOCK ❌
```

```
Option 2: close(out) wg.Wait() se PEHLE

  Workers abhi bhi `out` pe likh rahe hain
  close() + write = PANIC ❌
```

```
Option 3 (SAHI): alag goroutine mein wg.Wait() phir close(out)

  RunPool `out` return kar deta hai (non-blocking)
  Caller read karna shuru karta hai
  Workers unblock ho ke likhte hain
  wg.Done() sab call karte hain
  Closer goroutine: wg.Wait() return karta hai → close(out) ✅
  Caller ka `range out` naturally end hota hai ✅
```

```go
go func() {
    wg.Wait()   // sab workers done hone ka wait
    close(out)  // ab safe hai close karna
}()

return out      // TURANT return — no blocking
```

```
Python:  asyncio.gather() ke baad queue.join() implicitly handle hota hai
Node.js: Promise.all(workers).then(() => stream.end())
Go:      manually closer goroutine — explicit control, zero magic
```

---

### Domain structs se connection

```go
// Domain Monitor → Job (scheduler banata hai)
Job{
    MonitorID: monitor.ID,              // domain.Monitor.ID
    URL:       monitor.URL,             // domain.Monitor.URL
}
// monitor.IntervalSeconds → scheduler decide karta hai kab Job banana hai
// monitor.TimeoutSeconds  → ctx ka timeout yahan se aayega
// monitor.Active == false → Job banana hi mat

// Outcome.Result → domain Check row (result handler save karega)
Check{
    MonitorID:      outcome.Job.MonitorID,      // domain.Check.MonitorID
    StatusCode:     outcome.Result.StatusCode,  // domain.Check.StatusCode
    ResponseTimeMs: outcome.Result.LatencyMs,   // domain.Check.ResponseTimeMs
    Up:             outcome.Result.Up,          // domain.Check.Up
    Error:          outcome.Result.Err.Error(), // domain.Check.Error
}

// Agar Up == false → domain Incident open hoga (Stage 5)
// Agar Up == true aur pehle incident tha → Incident.ResolvedAt set hoga
```

---

### Go Gotchas in Stage 3

| Gotcha | Explanation |
|---|---|
| `close()` sirf producer kare | Multiple goroutines close karein → PANIC. Closer goroutine pattern isliye use karte hain. |
| `wg.Add()` launch se pehle | Loop ke andar `wg.Add(1)` karo lekin goroutine pehle Done() bole → counter negative → PANIC |
| `range jobs` bina `ctx.Done()` | Context cancel pe goroutine phansa rehta hai — goroutine leak |
| `wg.Wait()` RunPool mein block karna | Deadlock — caller ko `out` milta nahi, workers block ho jaate hain |
| `send on closed channel` | close() ke baad koi write kare → runtime panic. wg.Wait() guarantee karta hai sab done hain. |
| Goroutine argument capture | Loop variable `i` goroutine mein directly use karo → data race. Hamesha argument pass karo ya closure ke andar fresh variable. |

---

### Self-check Questions

1. **`close(out)` `wg.Wait()` se pehle kyun nahi kar sakte?** Kya hoga agar karo?

2. **`for job := range jobs` kyun use nahi kiya? `select` mein kya extra milta hai?**

3. **`wg.Add(workers)` loop ke andar karna (`wg.Add(1)` per goroutine) safe kyun nahi hoga yahan?**
   Hint: `wg.Wait()` closer goroutine mein pehle se chal raha hai.

4. **`out` channel ko `make(chan Outcome, workers)` buffered banaya — unbuffered hota toh kya change hota?**

---

### What's next — Stage 4

Scheduler:
- `internal/monitor/scheduler.go` — ticker goroutine jo DB query karke due monitors ke liye Jobs channel mein daalega
- `internal/platform/` — PostgreSQL (GORM) + Zerolog logger connect karna
- `internal/repository/` — Monitor DB access interface + implementation
- `main.go` mein Scheduler + Pool wire karna

---

## Stage 4 — Scheduler (context + select + ticker)

### What we built

| File | Purpose |
|---|---|
| `internal/monitor/scheduler.go` | `Scheduler` — ticks on interval, enqueues Jobs, closes channel on ctx cancel |
| `internal/monitor/scheduler_test.go` | Happy path (jobs enqueued) + cancel test (channel closes) |
| `cmd/try-pool/main.go` | Runnable demo: Scheduler + RunPool wired together, live output |

---

### The Full Pipeline — ab dono pieces hain

```
cancel()
   │
   ▼
context cancelled
   │
   ├──► Scheduler goroutine
   │      ctx.Done() case fires
   │      defer close(jobs) fires        ← Scheduler owns close(jobs)
   │
   ├──► Workers (pool.go)
   │      case job, ok := <-jobs
   │        ok=false → return            ← jobs closed signal
   │      defer wg.Done() × N
   │
   └──► Closer goroutine (pool.go)
          wg.Wait() returns (all workers done)
          close(out) fires               ← safe: no writers left
          caller's `range out` exits ✅
```

**Ek `cancel()` call → poori chain band. Koi goroutine leak nahi.**

---

### Concept 1 — `time.Ticker`

```go
ticker := time.NewTicker(200 * time.Millisecond)
defer ticker.Stop()   // timer goroutine release karo

for {
    select {
    case <-ticker.C:
        // har 200ms yeh case fire hota hai
    }
}
```

`ticker.C` ek channel hai. Har `tick` duration ke baad current time iss channel mein aata hai.

```
Python:  asyncio.sleep(tick) loop mein — cooperative yield
Node.js: setInterval(fn, tick) → clearInterval(id) cleanup mein
Go:      time.NewTicker(tick) → ticker.C channel → defer ticker.Stop()
```

**`defer ticker.Stop()` kyun zaruri hai?**
```
NewTicker internally ek goroutine start karta hai jo channel mein time bhejta hai.
Stop() nahi kiya → woh goroutine hamesha ke liye chal raha rahega.
= Timer goroutine leak.
```

---

### Concept 2 — `defer close(jobs)` — Producer ka Rule

```go
func Scheduler(...) <-chan Job {
    jobs := make(chan Job, 100)

    go func() {
        defer close(jobs)   // ← yeh line sabse important hai
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C: // jobs bhejo
            case <-ctx.Done(): return  // close trigger hoga defer se
            }
        }
    }()

    return jobs
}
```

**Sirf producer channel close kare — rule kyun?**

```
Agar RunPool workers bhi close(jobs) try karein:
  Worker 1: close(jobs) → OK
  Worker 2: close(jobs) → PANIC: close of closed channel ❌

Scheduler ek hi goroutine hai jo jobs mein likhta hai.
Isliye sirf Scheduler defer close(jobs) kare — guaranteed safe.
```

**`defer` vs explicit `close()` at function end:**
```go
// ❌ fragile — future mein koi `return` add kare aur close miss ho jaaye
func() {
    for { ... }
    close(jobs)  // yeh line kabhi kabhi skip ho sakti hai
}()

// ✅ guaranteed — function kisi bhi path se return kare, close hoga
func() {
    defer close(jobs)
    for { ... }
}()
```

---

### Concept 3 — Context Cancellation Propagation

Context ek **family tree** hai. Parent cancel ho toh sab children cancel ho jaate hain.

```go
ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
//              └── parent context (root)
//                  700ms baad auto-cancel

jobs := Scheduler(ctx, ...)
//      Scheduler usi ctx ko use karta hai

outcomes := RunPool(ctx, ...)
//           RunPool usi ctx ko use karta hai
```

```
context.Background()   ← root (kabhi cancel nahi hota)
       │
  ctx (700ms timeout)  ← cancel hoga 700ms baad ya explicit cancel() se
       │
  Scheduler uses ctx   ← ctx.Done() → scheduler stops
  RunPool uses ctx     ← ctx.Done() → workers stop mid-send
  HTTPChecker uses ctx ← ctx.Done() → TCP connection abort
```

**Python analogy:**
```python
# asyncio mein TaskGroup similar hai:
async with asyncio.TaskGroup() as tg:
    tg.create_task(scheduler())
    tg.create_task(pool())
# Ek task raise kare toh sab cancel ho jaate hain
```

**Node.js analogy:**
```js
const controller = new AbortController()
setTimeout(() => controller.abort(), 700)
// scheduler aur pool dono signal: controller.signal use karte hain
// abort event fire ho → sab band
```

---

### Concept 4 — `due func() []domain.Monitor` — Dependency Injection

```go
func Scheduler(ctx context.Context, tick time.Duration, due func() []domain.Monitor) <-chan Job
```

`due` ek **function parameter** hai — Scheduler ko parwah nahi kaise monitors milte hain.

```
Production mein:
  due = func() []domain.Monitor {
      monitors, _ := repo.FindDueMonitors(ctx)  // DB query
      return monitors
  }

Tests mein:
  due = func() []domain.Monitor {
      return []domain.Monitor{{ID: 1, URL: "..."}}  // hardcoded
  }
```

```
Python: dependency_injector ya simple constructor parameter
Node.js: constructor injection — new Scheduler(dueQuery)
Go:      function as parameter — same concept, cleaner syntax
```

**Yahi pattern hai poore Pulse mein:**
- Scheduler ko DB ka knowledge nahi chahiye
- Pool ko Scheduler ka knowledge nahi chahiye
- Sab pieces independently testable hain

---

### Teardown Trace — step by step

`cancel()` call hone ke baad exactly kya hota hai:

```
T+0ms  cancel() called

T+0ms  Scheduler goroutine:
         select mein ctx.Done() case fire hota hai
         return statement execute hota hai
         defer ticker.Stop() → timer goroutine free
         defer close(jobs) → jobs channel close hota hai

T+0ms  Worker 0, 1, 2 (pool.go):
         teeno kisi ek jagah pe hain:
         (a) `case <-ticker.C` se naya job aane ka wait kar rahe hain
             → `case <-ctx.Done()` fire hota hai → return
         (b) jobs se job uthake chk.Check() chal raha hai
             → ctx cancel → HTTPChecker TCP abort karta hai → return
         (c) `case job, ok := <-jobs` try kar rahe hain
             → jobs closed → ok=false → return

T+0ms  Har worker: defer wg.Done() fire karta hai
         wg counter: 3 → 2 → 1 → 0

T+0ms  Closer goroutine (pool.go):
         wg.Wait() return karta hai (counter = 0)
         close(out) fire hota hai

T+0ms  Caller (main.go / test):
         `range outcomes` loop exit hota hai
         Program cleanly khatam ✅
```

---

### Go Gotchas in Stage 4

| Gotcha | Explanation |
|---|---|
| `ticker.Stop()` bhool gaye | Internal timer goroutine hamesha ke liye chalta rahega — leak |
| `defer close(jobs)` bhool gaye | jobs kabhi close nahi hogi → pool workers kabhi nahi niklenge → deadlock |
| `due()` ko ctx pass nahi kiya | DB query timeout nahi hogi — slow query goroutine ko hang karegi |
| Scheduler ko `jobs` close nahi karna | Agar Scheduler kabhi return na kare, close kabhi nahi hoga — pipeline hang |
| `context.Background()` directly use karna | Root context cancel nahi hota — graceful shutdown impossible. Hamesha cancellable ctx use karo. |

---

### Self-check Questions

1. **cancel() call hone ke baad poori sequence trace karo** — Scheduler se lekar `range outcomes` exit tak. Kitne goroutines hain aur kaunsa kab exit karta hai?

2. **Scheduler `close(jobs)` karta hai, RunPool workers nahi — kyun?** Kya hoga agar ek worker bhi close karne ki koshish kare?

3. **`defer close(jobs)` vs function ke end mein `close(jobs)` — kya farak hai practically?**

4. **`due` function parameter kyun hai? Direct DB call kyun nahi kiya Scheduler mein?**

---

### What's next — Stage 5

Persistence — GORM + Repository pattern:
- `internal/platform/database/` — PostgreSQL connect + connection pool + AutoMigrate
- `internal/repository/` — MonitorRepository, CheckRepository, UserRepository, IncidentRepository interfaces + GORM implementations
- Error translation: `gorm.ErrRecordNotFound` → `domain.ErrNotFound`

---

## Stage 5 — Persistence (GORM + Repository Pattern)

### What we built

| File | Purpose |
|---|---|
| `internal/platform/database/database.go` | `Connect()` — opens Postgres, tunes pool; `AutoMigrate()` — syncs schema |
| `internal/repository/errors.go` | `translateError()` — converts GORM errors to domain errors |
| `internal/repository/monitor_repo.go` | `MonitorRepository` interface + GORM impl (Create, ByID, ListDue, ListByUser, UpdateStatus, UpdateNextCheck, Delete) |
| `internal/repository/check_repo.go` | `CheckRepository` interface + GORM impl (Save, History, LatestByMonitor) |
| `internal/repository/user_repo.go` | `UserRepository` interface + GORM impl (Create, ByID, ByEmail) |
| `internal/repository/incident_repo.go` | `IncidentRepository` interface + GORM impl (Create, OpenByMonitor, Resolve, ListByMonitor) |
| `domain/models.go` (updated) | Added `NextCheckAt time.Time` to Monitor — needed by ListDue |
| `cmd/server/main.go` (updated) | Step 2: `database.Connect` + `database.AutoMigrate` at startup |

---

### The Repository Pattern — Ek Simple Analogy

Socho ek **librarian (library wala)** hai.

- **Service (caller)** = student. "Mujhe 'Clean Code' book chahiye."
- **LibraryRepository (interface)** = reception counter. Counter ka koi bhi method call karo — tumhe nahi pata andar kya ho raha hai.
- **GormLibraryRepository (implementation)** = actual andar ka banda jo shelves pe jaata hai aur GORM se Postgres mein dhundhta hai.
- **FakeLibraryRepository (test double)** = test mein ek banda jo seedha "yeh lo book" bol deta hai — shelves nahi, Postgres nahi.

```
Student (service)
  ↓ calls interface method
Reception Counter (LibraryRepository interface)  ← boundary
  ↓ implemented by
GormLibraryRepository  OR  FakeLibraryRepository
```

**Interface ke bina:**
```go
// service seedha *gorm.DB use karta hai
func (s *Service) GetMonitor(id uint) (*domain.Monitor, error) {
    var m domain.Monitor
    s.db.First(&m, id)  // ← gorm seedha service mein
    return &m, nil
}
// Test mein: real Postgres chahiye. Slow. Flaky. CI mein pain.
```

**Interface ke saath:**
```go
// service sirf interface jaanta hai
func (s *Service) GetMonitor(ctx context.Context, id uint) (*domain.Monitor, error) {
    return s.monitors.ByID(ctx, id)  // ← interface call, gorm ka pata nahi
}
// Test mein: fakeMonitorRepo inject karo → hardcoded data → no DB needed ✅
```

---

### Concept 1 — GORM ka `WithContext`

```go
r.db.WithContext(ctx).First(&m, id)
```

**Har GORM query ke saath `WithContext(ctx)` lagao — hamesha.**

```
Bina WithContext:
  DB query shuru hui
  Request timeout ho gayi (client chala gaya)
  Query ABHI BHI chal rahi hai Postgres mein
  Wasted CPU, wasted DB connection

WithContext ke saath:
  ctx cancel → GORM immediately query abort karta hai
  DB connection pool mein wapas jaata hai
  Resources free ✅
```

```
Python/SQLAlchemy: session.execute(stmt, execution_options={"timeout": 5})
Node.js/Sequelize: Model.findOne({ transaction, lock: true })  — no direct ctx equiv
Go/GORM:          db.WithContext(ctx).First(...)  — idiomatic, always use it
```

---

### Concept 2 — Error Translation

```go
// errors.go
func translateError(err error) error {
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return domain.ErrNotFound
    }
    return err
}
```

**Kyun translate karna zaroori hai?**

```
Bina translation:
  Repository returns: gorm.ErrRecordNotFound
  Service ko import karna padega: "gorm.io/gorm"
  Handler ko bhi pata hoga GORM ka naam
  Kal GORM replace karoge → service + handler DONO todna padega

Translation ke saath:
  Repository returns: domain.ErrNotFound  (apna error)
  Service checks:     errors.Is(err, domain.ErrNotFound)
  Handler returns:    HTTP 404
  Kal GORM replace karo → sirf repository todna padega ✅
```

```
Python: SQLAlchemy NoResultFound → service catches AppNotFoundError
Node.js: Sequelize EmptyResultError → service catches NotFoundError
Go: gorm.ErrRecordNotFound → repository translates → domain.ErrNotFound
```

**Aur yeh kyun `errors.Is()` use karta hai `==` nahi?**
```go
// GORM error wrap karke return karta hai agar chain mein ho:
err = fmt.Errorf("First: %w", gorm.ErrRecordNotFound)

errors.Is(err, gorm.ErrRecordNotFound)  // ✅ true — chain mein dhundhta hai
err == gorm.ErrRecordNotFound           // ❌ false — direct compare, wrap miss
```

---

### Concept 3 — AutoMigrate

```go
db.AutoMigrate(
    &domain.User{},
    &domain.Monitor{},
    &domain.Check{},
    &domain.Incident{},
)
```

GORM domain structs ke field + tags ko read karta hai aur Postgres schema se compare karta hai.

```
Agar table nahi hai    → CREATE TABLE ... banata hai
Agar column nahi hai   → ALTER TABLE ... ADD COLUMN banata hai
Agar column hai already → kuch nahi karta (safe)
Agar column HATAYA struct se → GORM kuch nahi karta (column raha rahega)
```

**AutoMigrate kabhi column drop nahi karta** — production DB mein data safe.

```
Python/Alembic:    alembic upgrade head   ← SQL migration files generate karta hai
Python/GORM:       Base.metadata.create_all(engine)  ← auto, no migration files
Node.js/Sequelize: sequelize.sync({ alter: true })   ← same as GORM AutoMigrate
Go/GORM:           db.AutoMigrate(...)               ← development ke liye perfect
```

---

### Concept 4 — Connection Pool

```go
sqlDB.SetMaxOpenConns(25)        // max DB connections
sqlDB.SetMaxIdleConns(10)        // idle connections (pre-warmed)
sqlDB.SetConnMaxLifetime(5 * time.Minute)  // max age
```

**Bina pool tuning ke kya hoga?**

```
Default: unlimited connections
1000 concurrent requests aaye → 1000 DB connections open
Postgres ka default limit: 100 connections
Postgres crash → "too many connections" error ❌
```

**Pool ke saath:**
```
Max 25 connections → request 26 wait karta hai queue mein
Postgres comfortable → no crash ✅
```

```
ConnMaxLifetime kyun? 
  Ek connection 2 ghante old hai
  Postgres restart hua bich mein
  Old connection broken hai — GORM ko pata nahi
  Next query fail hogi
  5 minute max age → GORM automatically new connection banata hai ✅
```

---

### Concept 5 — `RowsAffected` check UPDATE mein

```go
result := r.db.WithContext(ctx).
    Model(&domain.Monitor{}).
    Where("id = ?", id).
    Updates(map[string]any{"active": active})

if result.RowsAffected == 0 {
    return domain.ErrNotFound  // monitor exists hi nahi
}
```

**Kyun?**
```
SELECT First() → row nahi mila → GORM returns gorm.ErrRecordNotFound ✅
UPDATE         → row nahi mila → GORM returns nil error, RowsAffected = 0 ❌

UPDATE pe GORM error nahi deta — tumhe khud RowsAffected check karna padta hai.
```

---

### Concept 6 — `Updates(map)` vs `Save(struct)`

```go
// ✅ SAHI — sirf active field update hoga
r.db.Updates(map[string]any{"active": false})

// ❌ GALAT — POORA struct save hoga, zero values bhi
r.db.Save(&monitor)  // NextCheckAt = time.Time{} → DB mein overwrite ❌
```

```
GORM Save():    "write karo SAB fields — zero values bhi"
GORM Updates(): "write karo sirf WOH fields jo map mein hain"

Rule: partial update karna hai toh hamesha Updates(map) use karo.
```

---

### Where does the interface "live"?

**Question se seedha:**
> "Why define repository as interface? Where does interface live and why?"

**Short answer:** Interface consumer ke paas hona chahiye (service layer), implementation producer ke paas (repository layer).

```
Ideal Go pattern:
  service/interfaces.go → MonitorRepository interface define karta hai
  repository/monitor_repo.go → interface satisfy karta hai

Humara Stage 5:
  repository/monitor_repo.go mein dono hain (interface + impl)
  Reason: service layer abhi bana nahi (Stage 6 mein banega)
  Stage 6 mein interface move hoga service/ mein
```

**Kyun interface service mein hona chahiye?**
```
Service sirf woh methods define kare jo USE karta hai.
Agar repository mein 10 methods hain lekin service sirf 3 use karta hai,
service ke interface mein sirf 3 honge.
Test mein fakeRepo sirf 3 methods implement karega — simpler.

Go proverb: "Accept interfaces, return structs."
Consumer (service) accepts interface.
Producer (repository New*) returns concrete struct (as interface).
```

---

### Go Gotchas in Stage 5

| Gotcha | Explanation |
|---|---|
| `First()` vs `Find()` | `First()` returns `ErrRecordNotFound` if 0 rows. `Find()` returns empty slice silently. Use First for single-row lookups. |
| `Save()` overwrites zero-values | GORM `Save(&struct)` writes ALL fields. Use `Updates(map)` for partial updates. |
| UPDATE `RowsAffected == 0` | GORM doesn't error on "no rows updated". Check manually. |
| `WithContext` miss karna | Query won't respect deadlines → goroutine leak under load. |
| `AutoMigrate` in production | AutoMigrate never drops columns. Safe for dev. In prod, use proper migration files (goose / migrate). |
| Multiple goroutines calling `close()` | If two goroutines both close the same channel → PANIC. Producer (Scheduler) owns close. |

---

### Self-check Questions

1. **Interface service mein define kyun karna chahiye — repository mein kyun nahi?** Go proverb kya hai?

2. **`gorm.ErrRecordNotFound` ko `domain.ErrNotFound` mein kyun translate karte hain?** Agar nahi karein toh kya toota?

3. **`Updates(map)` vs `Save(struct)` — kab kaunsa use karein?**

4. **`RowsAffected == 0` check kyun karna padta hai UPDATE queries mein?** SELECT mein kyun nahi?

5. **Connection pool mein `ConnMaxLifetime` kyun set kiya? Bina iske kya problem hoti?**

---

### What's next — Stage 6

Result Handler + Service layer:
- `internal/monitor/result_handler.go` — `outcomes` channel drain karna, Check save karna, Incident open/close karna
- `internal/service/` — business logic layer (MonitorService)
- Full pipeline wire in `main.go`: Scheduler + Pool + ResultHandler sab ek saath
