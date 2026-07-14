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

---

## Stage 6 — Service Layer + Pipeline Wiring

### What we built

| File | Purpose |
|---|---|
| `internal/service/monitor_service.go` | Business logic: RecordOutcome, CreateMonitor, GetMonitor, ListMonitors, PauseMonitor, ResumeMonitor, DeleteMonitor |
| `internal/monitor/pool.go` (updated) | `IntervalSeconds` field added to `Job` struct |
| `internal/monitor/scheduler.go` (updated) | `IntervalSeconds` passed in Job construction |
| `cmd/server/main.go` (rewritten) | Full wiring: config → db → repos → service → checker → scheduler → pool → outcome goroutine → HTTP server |

---

### The Complete Pipeline — Ab Poora Kaam Karta Hai

```
main.go
  │
  ├── config.Load()                      ← env vars
  ├── database.Connect() + AutoMigrate() ← Postgres
  ├── repository.New*()                  ← DB access interfaces
  ├── service.NewMonitorService()        ← business logic
  ├── checker.NewHTTPChecker()           ← HTTP probe tool
  │
  ├── monitor.Scheduler()  ──► [jobs chan]
  │      ↓ har tick pe
  │      monitorRepo.ListDue() → due monitors
  │      har monitor → Job{ID, URL, IntervalSeconds}
  │
  ├── monitor.RunPool()    ──► [outcomes chan]
  │      ↓ har job pe
  │      httpChecker.Check(ctx, url) → Result
  │
  ├── go func() { for o := range outcomes }
  │      ↓ har outcome pe
  │      svc.RecordOutcome(ctx, o)
  │        ├── checks.Save()               ← DB mein log
  │        ├── monitors.UpdateNextCheck()  ← agla check schedule
  │        └── incidents: open/close       ← alerting logic
  │
  └── http.Server  ← Stage 7 mein handlers aayenge
```

---

### Concept 1 — Dependency Injection By Hand

**Go mein koi DI framework nahi** (no Spring, no FastAPI Depends, no NestJS).
Sab kuch manually `main.go` mein wire karte hain.

```go
// ── INNERMOST LAYER PEHLE ──
db := database.Connect(cfg.DatabaseURL)      // needs: config

// ── LAYER 2: Repositories ──
monitorRepo := repository.NewMonitorRepository(db)   // needs: *gorm.DB
checkRepo   := repository.NewCheckRepository(db)
incidentRepo:= repository.NewIncidentRepository(db)

// ── LAYER 3: Service ──
svc := service.NewMonitorService(            // needs: repository interfaces
    monitorRepo,
    checkRepo,
    incidentRepo,
)

// ── LAYER 4: Pipeline ──
jobs     := monitor.Scheduler(ctx, tick, due)         // needs: ctx, due func
outcomes := monitor.RunPool(ctx, httpChecker, 10, jobs) // needs: ctx, checker, jobs
```

**Rule:** "Jis cheez ko doosri cheez chahiye, woh pehle banao."

```
Python: 
  repos = create_repos(db)
  svc   = MonitorService(repos)  ← manually inject

Node.js:
  const repos = createRepos(db)
  const svc   = new MonitorService(repos)  ← manually inject

Go:
  same — explicit, no magic ✅
```

---

### Concept 2 — Service Layer Kya Hai / Kyun Chahiye?

```
Handler    ← "HTTP request aaya, kya karna hai?"
  ↓
Service    ← "Business rule kya hai?"       ← YEH STAGE 6
  ↓
Repository ← "DB mein kaise store karein?"
  ↓
Domain     ← "Data kaisa dikhta hai?"
```

**Service ke 3 rules:**
1. **No HTTP** — `http.Request`, `http.ResponseWriter`, status codes — kuch nahi
2. **No SQL** — `*gorm.DB`, SQL strings — kuch nahi
3. **Only domain language** — `domain.Monitor`, `domain.ErrNotFound`, `domain.ErrValidation`

**Kyun?**

```
Agar service HTTP jaane:
  Service test karne ke liye HTTP server chalana padega
  Ek line change karo → poora integration test fail

Agar service SQL jaane:
  Service test karne ke liye Postgres chahiye
  CI mein pain

Service SIRF interfaces jaane:
  Test mein: fakeMonitorRepo inject karo → no DB, no HTTP, instant ✅
```

---

### Concept 3 — `RecordOutcome` — Business Logic Ka Dil

Yeh method wo hai jo Outcome lete hai (pool se) aur sab kuch karta hai:

```go
func (s *monitorService) RecordOutcome(ctx, o monitor.Outcome) error {
    // Step 1: Check log mein save karo
    c := &domain.Check{
        MonitorID:      o.Job.MonitorID,
        StatusCode:     o.Result.StatusCode,
        ResponseTimeMs: o.Result.LatencyMs,
        Up:             o.Result.Up,
        Error:          o.Result.Err.Error(), // agar error ho
    }
    s.checks.Save(ctx, c)

    // Step 2: Agla check kab? = Now + IntervalSeconds
    next := time.Now().Add(time.Duration(o.Job.IntervalSeconds) * time.Second)
    s.monitors.UpdateNextCheck(ctx, o.Job.MonitorID, next)

    // Step 3: Incident logic
    if !o.Result.Up {
        // Site DOWN hai
        _, err := s.incidents.OpenByMonitor(ctx, id)
        if ErrNotFound → // Pehli baar down → Incident kholo
        if nil         → // Already open incident hai → kuch mat karo
    } else {
        // Site UP hai
        incident, err := s.incidents.OpenByMonitor(ctx, id)
        if nil         → // Open incident tha → Resolve karo ✅
        if ErrNotFound → // Pehle se UP tha → kuch mat karo
    }
}
```

**Business rules yahan hain — repository mein nahi:**
- "Ek monitor ke liye ek hi open incident hoga"
- "Pehli baar down hone pe hi incident bano"
- "Wapas up aane pe incident resolve ho"

---

### Concept 4 — Service Handler Ko Import Kyun Nahi Kar Sakta?

```
handler imports service  ← OK ✅
service imports handler  ← CIRCULAR IMPORT ❌
```

```
handler → service → repository → domain
   ↑___________________________________↑
   Agar service handler import kare → yeh arrow circle ban jaata hai
   Go compiler immediately error deta hai:
   "import cycle not allowed"
```

**Practical example:**
```go
// handler/monitor_handler.go
import "service"  // OK

// service/monitor_service.go
import "handler"  // ❌ COMPILE ERROR: import cycle
```

**Isliye arrows sirf EK TARAF jaate hain:**
```
main.go → handler → service → repository → domain
                              checker ──┘
```

---

### Concept 5 — Pipeline ka Alag Context Kyun?

```go
pipelineCtx, pipelineCancel := context.WithCancel(context.Background())
// ... HTTP server ...
shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
```

**Do alag contexts — kyun?**

```
Scenario 1: Ek hi context (GALAT)

  SIGTERM aaya
  context cancel → pipeline band AND HTTP server band ek saath
  HTTP requests jo chal rahi thi → force close ❌
  Probe jo chal rahi thi → force abort ❌
  DB writes incomplete ❌

Scenario 2: Alag contexts (SAHI)

  SIGTERM aaya
  Step 1: pipelineCancel()
            → Scheduler rukta hai
            → Workers outcomes bhejte hain
            → RecordOutcome sab DB writes complete karta hai
            → Pipeline cleanly band ✅

  Step 2: srv.Shutdown(10s deadline)
            → HTTP server naye connections accept karna band
            → In-flight HTTP requests 10s mein complete hote hain ✅
```

---

### Shutdown Sequence — Poori Chain

```
Ctrl+C / SIGTERM
  │
  ▼
pipelineCancel() called
  │
  ├─► Scheduler: ctx.Done() → return → defer close(jobs)
  │
  ├─► Workers (x10): jobs closed → ok=false → return → wg.Done()
  │
  ├─► Closer goroutine: wg.Wait() → close(outcomes)
  │
  ├─► Outcome goroutine: range outcomes exits → goroutine ends
  │
  ▼
srv.Shutdown(10s)
  │
  ├─► No new HTTP connections
  ├─► In-flight requests finish
  │
  ▼
main() returns → os.Exit(0)

Total: ~0-2 seconds for pipeline + up to 10s for HTTP = clean shutdown ✅
```

---

### Go Gotchas in Stage 6

| Gotcha | Explanation |
|---|---|
| Service imports handler | Circular import — Go compiler rejects it. Arrows go ONE way only. |
| Service uses `*gorm.DB` | Now you can't test without Postgres. Always use repo interface. |
| Shared context for pipeline + HTTP | Both cancel together — in-flight requests and DB writes fail. Use separate contexts. |
| `UpdateNextCheck` error returned | If fatal, incident logic would be skipped. Log and continue — check was already saved. |
| Ownership check missing | Any user could read/modify any monitor. Always check `m.UserID == userID`. |
| `interval <= 0` default | If IntervalSeconds is 0 (new monitor bug), scheduler would spam checks. Defend with a minimum. |

---

### Self-check Questions

1. **"Dependency Injection by hand" ka matlab kya hai Go mein?** Framework se kya farak hai?

2. **Service handler import kyun nahi kar sakta?** Go mein error kya aayega?

3. **`RecordOutcome` mein `UpdateNextCheck` fail ho → kya return karo?** Kyun?

4. **Pipeline ka alag context kyun banaya? Agar shared hota toh kya problem hoti?**

5. **Ownership check service mein kyun hai — repository mein kyun nahi?**

---

### What's next — Stage 7

HTTP Handlers + Auth (JWT):
- `internal/handler/monitor_handler.go` — REST endpoints: POST /monitors, GET /monitors, PATCH /monitors/:id/pause
- `internal/middleware/auth.go` — JWT verification middleware
- `bcrypt` password hashing in UserService
- Full auth flow: register → login → JWT → protected routes

---

## Stage 7 — HTTP Handlers + JWT Auth

### Kya banaya (Files)

| File | Kya karta hai |
|---|---|
| `internal/service/user_service.go` | Register (bcrypt hash) + Login (bcrypt verify + JWT sign) |
| `internal/service/monitor_service.go` | `MonitorService` interface added — handler is decoupled |
| `internal/service/user_service.go` | `UserService` interface added — same reason |
| `internal/middleware/auth.go` | Bearer token extract → JWT validate → UserID inject in context |
| `internal/handler/respond.go` | Shared JSON writer + domain-error → HTTP status mapper |
| `internal/handler/auth_handler.go` | POST /auth/register + POST /auth/login |
| `internal/handler/monitor_handler.go` | POST/GET/DELETE /monitors + PATCH pause/resume |
| `cmd/server/main.go` | Wired userRepo, userSvc, authH, monitorH, protected route group |

---

### Auth Flow — Poora picture

```
Client                        Server
  │                              │
  │  POST /auth/register         │
  │  {"email":"…","password":"…"}│
  │─────────────────────────────►│  AuthHandler.Register()
  │                              │    → UserService.Register()
  │                              │        → bcrypt.GenerateFromPassword(pw, 12)
  │                              │        → userRepo.Create(user)
  │◄─────────────────────────────│  201 {"id":1,"email":"…"}
  │                              │
  │  POST /auth/login            │
  │  {"email":"…","password":"…"}│
  │─────────────────────────────►│  AuthHandler.Login()
  │                              │    → UserService.Login()
  │                              │        → userRepo.ByEmail(email)
  │                              │        → bcrypt.CompareHashAndPassword()
  │                              │        → jwt.NewWithClaims(HS256, {UserID:1, exp:+24h})
  │                              │        → token.SignedString(secret)
  │◄─────────────────────────────│  200 {"token":"eyJhbGci…"}
  │                              │
  │  GET /monitors               │
  │  Authorization: Bearer eyJ…  │
  │─────────────────────────────►│  middleware.Auth()
  │                              │    → jwt.ParseWithClaims(token, secret)
  │                              │    → context.WithValue(ctx, userIDKey, 1)
  │                              │  MonitorHandler.List()
  │                              │    → middleware.UserIDFromCtx(ctx) == 1
  │                              │    → svc.ListMonitors(ctx, 1, page, size)
  │◄─────────────────────────────│  200 [{…},{…}]
```

---

### Concept 1 — bcrypt kya hota hai?

**Password ko kabhi bhi plain text store nahi karte.** Agar DB leak ho jaye, sab passwords exposed ho jaayenge.

bcrypt ek **one-way hash function** hai:
- Plain text `"mypassword"` → `"$2a$12$abc...xyz"` (60 char hash)
- Hash se original password **kabhi recover nahi ho sakta** — yahi point hai
- Login ke time `bcrypt.CompareHashAndPassword(storedHash, givenPassword)` — dono compare karta hai internally same process run karke

```go
// Register ke time:
hashed, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), 12)
// DB mein store: "$2a$12$..."

// Login ke time:
err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("mypassword"))
// err == nil → password sahi hai
// err != nil → galat password
```

**Python analogy:** `bcrypt.hashpw(password.encode(), bcrypt.gensalt(rounds=12))`
**Node.js analogy:** `await bcrypt.hash(password, 12)` + `await bcrypt.compare(password, hash)`

**Cost factor 12 kyun?**
- Cost 4 = bahut fast = brute force easy
- Cost 12 = ~250ms per hash = brute force bahut slow
- Cost 15+ = login sluggish ho jaata hai
- 12 industry standard hai web apps ke liye

---

### Concept 2 — JWT kya hota hai?

JWT = **JSON Web Token** — ek signed string jo identity prove karta hai.

Format: `header.payload.signature` (teen parts, dot se separated, base64url encoded)

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9   ← header (algo: HS256)
.
eyJ1c2VyX2lkIjoxLCJleHAiOjE3...        ← payload (UserID: 1, exp: ...)
.
abcdef123456...                          ← signature (HMAC-SHA256 of header+payload)
```

**Kaise kaam karta hai:**
1. Server `jwt.NewWithClaims(HS256, {UserID: 1, exp: now+24h})` create karta hai
2. `.SignedString(secret)` se sign karta hai
3. Client ko bhejta hai
4. Client har request mein `Authorization: Bearer <token>` bhejta hai
5. Server signature verify karta hai — agar kisi ne payload tamper kiya to signature mismatch hogi

**Secret key kabhi client ko nahi milta** — server ke paas hai sirf.

```go
// Sign karna:
claims := PulseClaims{
    UserID: u.ID,
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24*time.Hour)),
    },
}
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
tokenStr, _ := token.SignedString(jwtSecret)

// Verify karna (middleware mein):
claims := &PulseClaims{}
token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
    return jwtSecret, nil
})
// claims.UserID → 1 (agar valid)
```

**Python analogy:** `jwt.encode({"user_id": 1, "exp": ...}, secret, algorithm="HS256")`
**Node.js analogy:** `jwt.sign({ userId: 1 }, secret, { expiresIn: "24h" })`

---

### Concept 3 — context.WithValue se UserID pass karna

Middleware aur handler ek hi HTTP request lifecycle mein hote hain, lekin **alag functions** hain. Data share karne ka tarika hai Go `context`.

```go
// Middleware mein (auth.go):
ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
next.ServeHTTP(w, r.WithContext(ctx))

// Handler mein (monitor_handler.go):
userID, ok := middleware.UserIDFromCtx(r.Context())
// userID == claims.UserID — wahi value jo middleware ne set ki thi
```

**Kyun custom type as key?**
```go
type ctxKey string
const userIDKey ctxKey = "userID"
```
Agar plain string use karo — `context.WithValue(ctx, "userID", 1)` — to koi bhi package same key use kar sakta hai aur overwrite kar sakta hai. Custom type **compile-time unique** hoti hai — collision impossible.

**Python analogy:** `request.state.user_id = 1` (FastAPI)
**Node.js analogy:** `req.userId = 1` (Express middleware)

---

### Concept 4 — Service Interface kyun banaya?

Stage 6 mein `NewMonitorService` return karta tha `*monitorService` (unexported concrete type). Handler package is type ko reference nahi kar sakta tha (lowercase = unexported).

**Solution:** Service package mein hi interface define karo:

```go
// service/monitor_service.go
type MonitorService interface {
    CreateMonitor(ctx context.Context, ...) (*domain.Monitor, error)
    GetMonitor(ctx context.Context, id, userID uint) (*domain.Monitor, error)
    // ...
}

func NewMonitorService(...) MonitorService { // returns interface, not *monitorService
    return &monitorService{...}
}
```

**Handler:**
```go
type MonitorHandler struct {
    svc service.MonitorService  // interface — not *monitorService
}
```

**Fayde:**
1. Handler test mein fake MonitorService inject kar sakte ho — real DB ki zarurat nahi
2. Handler ka concrete struct se koi direct dependency nahi
3. Circular import impossible — handler → service (interface), service → repository

**Python analogy:** `class MonitorServiceProtocol(Protocol): ...` (typing.Protocol)
**Node.js/TypeScript:** `interface MonitorService { createMonitor(...): Promise<Monitor> }`

---

### Concept 5 — "none algorithm" attack aur signing method check

JWT mein ek purani vulnerability thi — attacker `alg: "none"` header set karta tha aur bina signature ke token bhejta tha. Naïve libraries is token ko valid maan leti thein.

**Fix — middleware mein signing method check:**
```go
jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
    // Ye check zaruri hai:
    if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
        return nil, errors.New("unexpected signing method")
    }
    return jwtSecret, nil
})
```

Agar koi `alg: "none"` bheje → type assertion fail → error → 401.

---

### Concept 6 — Domain error → HTTP status mapping

Ek jagah mapping honi chahiye — **respond.go ka `Err()` function**.

```go
func Err(w http.ResponseWriter, err error) {
    code := http.StatusInternalServerError
    switch {
    case errors.Is(err, domain.ErrNotFound):     code = 404
    case errors.Is(err, domain.ErrValidation):   code = 422
    case errors.Is(err, domain.ErrConflict):     code = 409
    case errors.Is(err, domain.ErrUnauthorized): code = 401
    }
    // ...
}
```

**Kyun ek jagah?**
- Agar har handler mein alag status code likho → inconsistency aayegi
- Service `domain.ErrNotFound` return karti hai — handler ko pata nahi hona chahiye woh 404 hai; `Err()` jaanta hai

**Note:** Internal 500 errors mein original message nahi bhejte client ko:
```go
if code == http.StatusInternalServerError {
    msg = "internal server error"  // SQL error user ko mat dikhao
}
```

---

### Route Structure — Chi router groups

```go
r := chi.NewRouter()
r.Use(chiMW.Logger)      // global: sab requests log hote hain
r.Use(chiMW.Recoverer)   // global: panic → 500 instead of crash

// Public routes — no JWT
r.Get("/health", healthHandler)
r.Post("/auth/register", authH.Register)
r.Post("/auth/login", authH.Login)

// Protected routes — JWT required
r.Route("/monitors", func(r chi.Router) {
    r.Use(middleware.Auth([]byte(cfg.JWTSecret)))  // sirf /monitors/* ke liye
    r.Post("/", monitorH.Create)
    r.Get("/", monitorH.List)
    r.Get("/{id}", monitorH.Get)
    r.Patch("/{id}/pause", monitorH.Pause)
    r.Patch("/{id}/resume", monitorH.Resume)
    r.Delete("/{id}", monitorH.Delete)
})
```

`r.Route` ek sub-router banata hai. `r.Use` us sub-router ke andar sirf wahan apply hota hai — `/health` pe `Auth` middleware nahi chalega.

**Python/FastAPI analogy:**
```python
monitors_router = APIRouter(prefix="/monitors", dependencies=[Depends(get_current_user)])
app.include_router(monitors_router)
```

**Node.js/Express analogy:**
```javascript
const monitorRouter = express.Router()
monitorRouter.use(authMiddleware)
app.use("/monitors", monitorRouter)
```

---

### Gotchas

| Gotcha | Solution |
|---|---|
| `json:"-"` on Password miss ho jaye | Password JSON mein leak ho jaayega — **security bug** |
| Email enumeration | Login mein `domain.ErrUnauthorized` return karo — "user not found" mat batao |
| bcrypt cost bahut low | Cost 4-6 = brute force fast hoga. Cost 12 minimum rakho. |
| `alg: "none"` attack | `jwt.ParseWithClaims` ke key function mein signing method type-assert karo |
| Context key collision | Plain string ki jagah custom `type ctxKey string` use karo |
| `monitors == nil` in list | `nil` slice JSON mein `null` ban jaata hai — `[]domain.Monitor{}` se empty array bhejo |
| 500 error details leak | Internal errors (`gorm:...`) client ko mat dikhao — generic "internal server error" bhejo |

---

### Self-check Questions

1. **bcrypt ka "cost factor" kya control karta hai?** Production mein 12 kyun rakha?

2. **JWT ka header, payload, signature kya hai?** Agar payload tamper kiya to kya hoga?

3. **`context.WithValue` mein custom type key kyun use ki?** String key se kya problem aati?

4. **`MonitorService` interface service package mein kyun define ki — handler package mein kyun nahi?**

5. **`middleware.Auth` ne context mein UserID store kiya — ab handler kaise nikale?** Agar Auth middleware chain mein nahi hota to kya hota?

6. **`Err(w, err)` internal error ka original message kyun nahi bhejta client ko?**

7. **Email enumeration attack kya hai? Iska kya practical impact hai?**

---

### What's next — Stage 8

- Handler unit tests (`httptest.NewRecorder`, mock service)
- Incident list endpoint: `GET /api/monitors/{id}/incidents`
- Rate limiting middleware (chi-ratelimit)

---

## Stage 7 (Revised) — RequestID + zerolog + Validator + Consistent Envelope

### Kya banaya (nayi cheezein)

| File | Kya karta hai |
|---|---|
| `internal/middleware/request_id.go` | UUID generate → context + `X-Request-ID` header |
| `internal/middleware/logger.go` | zerolog structured request log (method, path, status, latency, request_id) |
| `internal/handler/respond.go` | Consistent envelope `{success, data/error, message, request_id}` |
| `internal/handler/monitor_handler.go` | go-playground/validator on create body; `GET /api/monitors/{id}/checks` paginated |
| `internal/repository/check_repo.go` | `History` signature: added `offset int` for real pagination |
| `internal/service/monitor_service.go` | `GetCheckHistory` added to interface + implementation |
| `cmd/server/main.go` | zerolog init; `RequestID → Logger → Recoverer` middleware order; `/api` prefix |

---

### Concept 1 — Middleware Chain: Andar aur Bahar ka order

Middleware ek **stack** ki tarah kaam karta hai — LIFO (Last In, First Out).

```
r.Use(middleware.RequestID)        ← 1st registered
r.Use(middleware.NewLogger(log))   ← 2nd registered
r.Use(chiMW.Recoverer)             ← 3rd registered
```

**Request aane par (IN — outermost se innermost):**
```
Request → RequestID → Logger → Recoverer → Handler
```

**Response jaane par (OUT — innermost se outermost):**
```
Handler → Recoverer → Logger (yahan status capture hota hai) → RequestID
```

Socho ek onion (pyaaz):
- Har `r.Use()` ek layer add karta hai
- Request center ki taraf jaati hai (sab layers se guzarti hai)
- Response wapas center se bahaar aati hai (wahi layers ulte order mein)

**WHY RequestID pehle hona chahiye?**
Logger `RequestIDFromCtx()` call karta hai. Agar RequestID baad mein hota, context mein UUID nahi hota — log mein `request_id: ""` dikha.

```
WRONG order:
  Logger → RequestID → Handler
  Logger: RequestIDFromCtx(ctx) == ""  ← UUID abhi context mein nahi!

CORRECT order:
  RequestID → Logger → Handler
  Logger: RequestIDFromCtx(ctx) == "abc-123"  ← UUID already set hai
```

**Python/FastAPI analogy:**
```python
# FastAPI mein LAST add = FIRST run (ulta order)
app.add_middleware(LoggingMiddleware)   ← second run
app.add_middleware(RequestIDMiddleware) ← first run  (kyunki last add kiya)
```

**Node.js/Express analogy:**
```javascript
// Express mein FIRST app.use = FIRST run (same order)
app.use(requestIdMiddleware)   // first run
app.use(loggerMiddleware)      // second run
```

---

### Concept 2 — Handler Request ID kaise padhta hai

`context.WithValue` → `context.Value` ka direct pipeline:

```
RequestID middleware:
  ctx := context.WithValue(r.Context(), reqIDKey{}, "abc-123")
  next.ServeHTTP(w, r.WithContext(ctx))
         ↓
Logger middleware:
  id := ctx.Value(reqIDKey{}).(string)  // "abc-123"
         ↓
respond.go (JSON / Err):
  id := middleware.RequestIDFromCtx(r.Context())  // "abc-123"
  // envelope mein daal do:
  json.Encode(envelope{RequestID: id, ...})
         ↓
Client:
  Response header: X-Request-ID: abc-123
  Response body:   {"success":true,"data":{...},"request_id":"abc-123"}
```

**Ek UUID — teen jagah dikhta hai:**
1. `X-Request-ID` response header
2. Log line mein `"request_id":"abc-123"`
3. JSON response body mein `"request_id":"abc-123"`

Client bug report karta hai: "mujhe ye error aaya" + request_id copy karta hai.
Support engineer logs mein grep karta hai: ek second mein poori request ki story mil jaati hai.

---

### Concept 3 — responseWriter wrapper kyun?

`http.ResponseWriter` ek interface hai. Iske paas status code nikalne ka koi method nahi hai agar handler ne already likh diya.

```go
// Standard ResponseWriter — status code likhne ke baad bahar nahi milta
w.WriteHeader(201)
// w.StatusCode()  ← ye method exist nahi karta!
```

**Solution:** Wrap karo — method override karo:
```go
type responseWriter struct {
    http.ResponseWriter
    status int  // apna capture field
}

func (rw *responseWriter) WriteHeader(status int) {
    rw.status = status              // capture karo
    rw.ResponseWriter.WriteHeader(status)  // real writer ko bhi bhejo
}
```

Logger middleware:
```go
rw := &responseWriter{ResponseWriter: w, status: 200}
next.ServeHTTP(rw, r)  // handler rw pe likhta hai
// handler return ke baad:
log.Info().Int("status", rw.status)...  // captured!
```

**Python WSGI analogy:** `start_response` ko wrap karna taaki status capture ho sake.
**Node.js Express analogy:** `res.json` ya `res.send` ko monkey-patch karna.

---

### Concept 4 — Consistent Envelope

Pehle wali respond.go: `JSON(w, status, data)` — simple, no envelope.

Nayi respond.go: `JSON(w, r, status, data)` — `r *http.Request` isliye add kiya ki context se request_id nikaal sakein.

```go
// Success response:
{
  "success":    true,
  "data":       {"id": 1, "url": "https://example.com", ...},
  "request_id": "550e8400-..."
}

// Error response:
{
  "success":    false,
  "error":      "validation_failed",   ← machine-readable
  "message":    "URL: url; IntervalSeconds: min=5",  ← human-readable
  "request_id": "550e8400-..."
}
```

**`json:"omitempty"` kya karta hai?**
- Success pe: `error` aur `message` empty string hain → JSON mein field hi nahi aata
- Failure pe: `data` nil hai → JSON mein field nahi aata
- Result: har response mein sirf relevant fields

**Python analogy:** `dataclass` with `Optional` fields + custom `__post_init__`
**Node.js:** `JSON.stringify` skips `undefined` values

---

### Concept 5 — go-playground/validator

Service layer mein bhi validation hai (e.g., `intervalSecs < 5`). Handler mein validator add kyun kiya?

**Defense in depth (baar baar guard):**
- Handler validator → fast fail, specific field errors, before service call
- Service validation → domain rules (e.g., minimum 5 seconds for interval)

Validator tags `createReq` struct pe:
```go
type createReq struct {
    URL             string `json:"url"              validate:"required,url"`
    Name            string `json:"name"             validate:"required,min=1,max=255"`
    IntervalSeconds int    `json:"interval_seconds" validate:"required,min=5,max=86400"`
    TimeoutSeconds  int    `json:"timeout_seconds"  validate:"required,min=1,max=60"`
}
```

`url` tag kya check karta hai?
- Scheme present hona chahiye (https:// ya http://)
- Host present hona chahiye
- `"example.com"` → fail (no scheme)
- `"https://example.com"` → pass

Error output:
```
URL: url; IntervalSeconds: min=5
```
(`validationMsg()` function `validator.ValidationErrors` ko readable string mein convert karta hai)

---

### Concept 6 — zerolog structured logging

```go
logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).
    With().Timestamp().Logger()
```

`zerolog.ConsoleWriter` dev mein pretty print karta hai:
```
12:00:00 INF request method=GET path=/health status=200 latency_ms=0 request_id=abc-123
12:00:01 INF request method=POST path=/api/monitors status=201 latency_ms=4 request_id=def-456
12:00:02 WRN request method=GET path=/api/monitors/99 status=404 latency_ms=1 request_id=ghi-789
```

Production mein JSON output:
```json
{"level":"info","method":"POST","path":"/api/monitors","status":201,"latency_ms":4,"request_id":"def-456","time":"2025-07-14T12:00:01Z"}
```

**Log levels:**
- `logger.Info()` → normal requests (2xx, 3xx)
- `logger.Warn()` → client errors (4xx)
- `logger.Error()` → server errors (5xx)

**Builder pattern:** zerolog uses method chaining instead of variadic args:
```go
logger.Info().
    Str("method", "POST").
    Int("status", 201).
    Msg("request")  // ← Msg() flushes the event; MUST call this
```

---

### Gotchas

| Gotcha | Solution |
|---|---|
| RequestID baad mein add kiya | Logger empty UUID log karta — RequestID PEHLE hona chahiye |
| `WriteHeader` twice | `wroteHeader bool` guard se sirf ek baar fire karo |
| `validate` singleton nahi | Har request pe `validator.New()` → reflection overhead. Package-level `var` use karo |
| `fmt.Sprintf` ke saath `domain.ErrValidation` | `errors.Is()` wrapped errors traverse karta hai — `fmt.Errorf("%w: ...")` use karo |
| `monitors == nil` list response | GORM empty query `nil` return kar sakta hai — `[]domain.Monitor{}` se safe empty array bhejo |
| `json:"omitempty"` miss | Error aur success fields dono response mein dikhenge — noise for clients |

---

### Self-check Questions

1. **Middleware order mein RequestID pehle aur Logger baad mein kyun hai?** Agar ulta karo to kya hoga?

2. **`responseWriter` wrapper kyun banaya?** `http.ResponseWriter` seedha status code expose kyun nahi karta?

3. **`context.WithValue` mein custom struct type `reqIDKey{}` kyun use kiya?** String `"requestID"` se kya problem hoti?

4. **Middleware "way in" pe outermost se innermost kyu chalta hai — "way out" pe kyun ulta?**

5. **`validate:"required,url"` mein `"example.com"` kyun fail hoga?** Kya pass karna hoga?

6. **`json:"omitempty"` ke bina envelope kaisi dikhegi?** Kya farak padta hai client ko?

---

### What's next — Stage 8

- Handler unit tests with `httptest.NewRecorder` + mock `MonitorService`
- `GET /api/monitors/{id}/incidents` endpoint
- Rate limiting middleware

---

## Stage 8 — Redis cache + rate limiting

### Kya banaya

| File | Kya karta hai |
|---|---|
| `internal/platform/cache/cache.go` | go-redis wrapper: Get/Set/Del/Incr/Expire + `ErrCacheMiss` sentinel |
| `internal/service/monitor_service.go` | cache-aside in `GetMonitor`; `cacheDelMonitor` on pause/resume/delete |
| `internal/middleware/ratelimit.go` | INCR+EXPIRE pattern: 429 after limit, `X-RateLimit-*` headers |
| `cmd/server/main.go` | Redis connect (graceful degrade), cache→service, RateLimit on `/api` |

---

### Concept 1 — Cache-aside pattern

Cache-aside = **application controls caching** (not the DB driver or ORM).

**READ path (miss):**
```
GET /api/monitors/42
  → service.GetMonitor(ctx, 42, userID)
      → cache.Get("pulse:monitor:42")  → ErrCacheMiss
      → monitors.ByID(ctx, 42)         → DB query (slow)
      → cache.Set("pulse:monitor:42", json, 30s)
      → return monitor
```

**READ path (hit):**
```
GET /api/monitors/42  (again, within 30s)
  → service.GetMonitor(ctx, 42, userID)
      → cache.Get("pulse:monitor:42")  → "{...json...}"  ← DB nahi chua!
      → json.Unmarshal → *domain.Monitor
      → ownership check
      → return monitor
```

**WRITE path (invalidation):**
```
PATCH /api/monitors/42/pause
  → service.PauseMonitor(ctx, 42, userID)
      → monitors.UpdateStatus(ctx, 42, false)  → DB update
      → cache.Del("pulse:monitor:42")           → key remove
```

Ab next GET pe cache miss hoga → fresh data DB se aayega.

**Socho ek newspaper ki tarah:**
- Cache = newspaper stand ka copy (fast, stale ho sakta hai)
- DB = printing press (slow, always authoritative)
- Cache-aside = "stand pe copy hai? wahi do. nahi hai? press se nikalo aur stand pe rakh do."

**Python/Redis analogy:**
```python
def get_monitor(id):
    cached = redis.get(f"pulse:monitor:{id}")
    if cached:
        return json.loads(cached)          # cache hit
    monitor = db.query(Monitor, id)        # cache miss → DB
    redis.setex(f"pulse:monitor:{id}", 30, json.dumps(monitor))
    return monitor
```

---

### Concept 2 — TTL (Time To Live) kyun chahiye?

TTL = Redis ke andar ek countdown timer. Countdown khatam → key automatically delete.

```
SET pulse:monitor:42 "..." EX 30
              ↓
  30 seconds baad:  GET pulse:monitor:42 → (nil) = ErrCacheMiss
```

**TTL ke bina kya hoga?**
- Monitor pause kiya → cache.Del kiya → theek hai
- Lekin: monitor kuch aur jagah se update hua (direct DB, migration, etc.) → cache mein stale data hamesha rahega
- TTL = automatic safety net — worst case 30s baad fresh data

**Short TTL (30s) kyun?**
- Monitor zyada change nahi hota (URL, interval — rarely updated)
- 30s enough hai ki burst of requests (10 in 5s) DB ko na chhue
- `active` field change hoti hai pause/resume pe — invalidation handle karta hai

---

### Concept 3 — Invalidation AFTER DB write kyun?

```go
// GALAT order:
cache.Del(key)          // 1. cache clear
monitors.Update(...)    // 2. DB update (fails!)
// Result: cache cleared, DB old — next GET repopulates with old DB value
//         User got 204 success but DB didn't change → data inconsistency

// SAHI order:
monitors.Update(...)    // 1. DB update (succeeds)
cache.Del(key)          // 2. cache clear
// Result: DB has new data; next GET fetches fresh data from DB
```

**Rule: Pehle DB likhho, phir cache invalidate karo.**

Ek edge case: DB update succeed, cache.Del fail → stale cache for max TTL (30s). Acceptable kyunki:
1. TTL baad automatically expire hoga
2. 30s stale data is much better than data inconsistency

---

### Concept 4 — INCR kyun GET+SET se safe hai

**GET + SET — race condition:**

```
Goroutine A              Goroutine B          Redis key
GET rl:1.2.3.4  → 5                          5
                    GET rl:1.2.3.4  → 5      5    ← A ne likha nahi abhi!
SET rl:1.2.3.4 6                              6
                    SET rl:1.2.3.4 6          6    ← B ne A ka write overwrite kiya
```
Result: do requests aaye, sirf ek count hua. Rate limiter bypass ho sakta hai.

**INCR — atomic (Redis single-threaded execution):**

```
Goroutine A              Goroutine B          Redis key
INCR rl:1.2.3.4 → 6                          6    ← read+write atomic
                    INCR rl:1.2.3.4 → 7      7    ← A ka result already reflected
```
Result: do requests → count 6 aur 7 — correct.

**Redis single-threaded** matlab: ek time pe sirf ek command execute hota hai. INCR = read+add+write ek hi operation mein. No two goroutines can interleave inside it.

**Mutex Go mein kyun nahi chahiye?**
Mutex sirf ek process ke andar goroutines protect karta hai. Multiple app servers (3 pods in Kubernetes) mein Go mutex kuch nahi karega — lekin Redis INCR tab bhi atomic rahega kyunki Redis ek single server hai.

```
App Server 1 → INCR "pulse:rl:1.2.3.4:bucket" → 1
App Server 2 → INCR "pulse:rl:1.2.3.4:bucket" → 2   ← correct!
App Server 3 → INCR "pulse:rl:1.2.3.4:bucket" → 3   ← correct!
```

---

### Concept 5 — EXPIRE sirf count==1 pe kyun?

```go
count, _ := c.Incr(ctx, key)
if count == 1 {
    c.Expire(ctx, key, window)  // sirf ek baar!
}
```

Agar har request pe Expire call karo:

```
t=0s  INCR → 1,  EXPIRE key 60s  → key expires at t=60s
t=5s  INCR → 2,  EXPIRE key 60s  → key expires at t=65s  ← window pushed!
t=10s INCR → 3,  EXPIRE key 60s  → key expires at t=70s  ← further pushed!
...continues forever while traffic flows...
```
Key kabhi expire nahi hoga — rate limit window kabhi reset nahi hoga.

**count==1 pe sirf ek baar:**
```
t=0s  INCR → 1,  EXPIRE key 60s  → key expires at t=60s
t=5s  INCR → 2   (no EXPIRE call)
t=10s INCR → 3   (no EXPIRE call)
...
t=60s key expires → next INCR creates fresh key → new window
```

---

### Concept 6 — Graceful degradation (fail-open)

```go
var cacheClient *cache.Cache
if c, err := cache.New(cfg.RedisURL); err != nil {
    logger.Warn().Err(err).Msg("redis unavailable — running without cache")
} else {
    cacheClient = c
}
svc := service.NewMonitorService(..., cacheClient)  // nil is OK
```

Service mein:
```go
if s.cache != nil {
    if m, ok := s.cacheGetMonitor(ctx, id); ok {
        return m, nil  // cache hit
    }
}
// cache nil ya miss → DB se lo
```

RateLimit middleware:
```go
if c == nil {
    next.ServeHTTP(w, r)  // no cache → no rate limit → proceed
    return
}
```

**Fail-open** = Redis down hone pe service kaam karta rahega (sirf thoda slow). DB hi source of truth hai.

**Fail-closed** = Redis down → service completely down. Ye wrong hai — cache is an optimization, not a requirement.

---

### Rate-limit response headers

```
X-RateLimit-Limit:     200          ← max per window
X-RateLimit-Remaining: 197          ← requests left
X-RateLimit-Reset:     1705234620   ← Unix timestamp when window resets
```

429 response:
```json
{
  "success": false,
  "error": "rate_limit_exceeded",
  "message": "too many requests — retry after 45s",
  "request_id": "abc-123"
}
```

`Retry-After` header bhi set hota hai — clients API ko intelligently back off karte hain.

---

### Gotchas

| Gotcha | Solution |
|---|---|
| Cache invalidate BEFORE DB write | Stale data repopulates on next read. Always Del AFTER DB write. |
| EXPIRE on every request | Window kabhi reset nahi hoga. EXPIRE sirf count==1 pe. |
| GET+SET for rate limiting | Race condition — use INCR (atomic). |
| Redis nil cache panic | `if s.cache == nil` guard har jagah. |
| Forgetting TTL on Set | Key grows forever — always pass TTL. |
| Long TTL for mutable data | 30s is short enough; pause/resume/delete invalidation handles the rest. |

---

### Self-check Questions

1. **Cache-aside hit path trace karo step by step.** DB kab chhuta nahi?

2. **INCR kyun GET+SET se safe hai?** Race condition explain karo.

3. **EXPIRE sirf count==1 pe kyun call karte hain?** Har request pe call karne se kya hoga?

4. **Cache invalidation DB write AFTER kyun?** Pehle karne se kya problem?

5. **Redis down hone pe kya hoga?** Service crash karegi?

6. **3 app servers (Kubernetes pods) mein Go mutex rate limiting kyun fail hoga?** Redis INCR kyun kaam karega?

---

## Stage 9 — Incidents & Notifications (Transactions + Interface)

### What we built

| File | Change |
|------|--------|
| `internal/domain/models.go` | Added `Status string` field to `Monitor` ("unknown" / "up" / "down") |
| `internal/repository/tx.go` | NEW — `Transactor` interface + `gormTransactor` using `db.Transaction()` |
| `internal/notifier/notifier.go` | NEW — `Notifier` interface + `LogNotifier` (zerolog) |
| `internal/repository/monitor_repo.go` | Added `SetStatus(ctx, id, status)` to interface + implementation |
| `internal/repository/incident_repo.go` | Made `Create` and `Resolve` transaction-aware via `txDB(ctx, r.db)` |
| `internal/service/monitor_service.go` | Added `transactor`/`notifier` fields; rewrote `RecordOutcome` Step 3 |
| `cmd/server/main.go` | Wired `repository.NewTransactor(db)` and `notifier.NewLogNotifier(logger)` |

---

### Concept 1 — GORM Transactions (`db.Transaction`)

**The core mechanic:**

```go
db.Transaction(func(tx *gorm.DB) error {
    tx.Create(&incident)         // Step A
    tx.Update("status", "down")  // Step B
    return nil                   // ← nil = COMMIT; error = ROLLBACK
})
```

GORM wraps your function with:
- `BEGIN` before calling your function
- `COMMIT` if you return `nil`
- `ROLLBACK` if you return any error (or if your function panics)

**Python/SQLAlchemy equivalent:**
```python
with session.begin():
    session.add(incident)
    monitor.status = "down"
# COMMIT on __exit__(None), ROLLBACK on exception
```

**Node.js/Knex equivalent:**
```javascript
await knex.transaction(async trx => {
    await trx('incidents').insert(incident)
    await trx('monitors').where({ id }).update({ status: 'down' })
})
// COMMIT on resolve, ROLLBACK on reject
```

---

### Concept 2 — Why Two Writes MUST Be Atomic

**UP→DOWN transition creates TWO separate DB writes:**
1. `INSERT INTO incidents (monitor_id, started_at, ...)`
2. `UPDATE monitors SET status='down' WHERE id=?`

**Without a transaction — crash between writes:**
```
Write 1 ✅  Incident row created → "monitor is DOWN"
[CRASH]
Write 2 ❌  monitors.status stays "up"
```

Result: incident table says "DOWN", monitors table says "UP" → **inconsistent data**. The API would show the monitor as "up" while an active incident record sits in the DB.

**With a transaction:**
```
BEGIN
  Write 1 — incident INSERT (not yet committed)
  Write 2 — status UPDATE  (not yet committed)
COMMIT ← both land together, atomically
```

Either both writes are visible, or neither. No half-written state can exist.

---

### Concept 3 — The `Transactor` Pattern (hiding `*gorm.DB` from the service)

The service layer must never import `gorm.io/gorm` — that would couple business logic to a DB driver.

**How we solve it:**

```
service ──uses──► repository.Transactor  (interface, no gorm import needed)
                         ▲
                         │ implements
                  gormTransactor (in repository pkg — gorm allowed here)
```

The `Transactor` interface exposes only:
```go
type Transactor interface {
    RunInTx(ctx context.Context, fn TxFn) error
}
```

`service.RecordOutcome` calls it like this:
```go
s.transactor.RunInTx(ctx, func(ctx context.Context) error {
    s.incidents.Create(ctx, incident)    // uses tx from ctx
    s.monitors.SetStatus(ctx, id, "down") // uses tx from ctx
    return nil // commit
})
```

No `*gorm.DB` anywhere in the service. The service doesn't know or care how the transaction is implemented.

---

### Concept 4 — Context-carried Transaction (`txDB`)

The trick is storing the `*gorm.DB` transaction handle inside `context.Context`:

```go
// Inside RunInTx:
txCtx := context.WithValue(ctx, txKey{}, tx)  // store tx in ctx
fn(txCtx)                                       // pass to repo methods

// Inside each repo method:
db := txDB(ctx, r.db)  // extract tx if present; otherwise use r.db
db.WithContext(ctx).Create(...)
```

**Why an unexported struct key?**
```go
type txKey struct{}  // zero-size, unexported
```
Using `"tx"` (a string) as the key would collide with any other package that also stores `"tx"` in context. An unexported struct type can only be referenced from within the `repository` package — guaranteed uniqueness.

**Python analogy:** SQLAlchemy uses scoped sessions (thread-local) to share the same session across function calls. Context is Go's equivalent of thread-local.

---

### Concept 5 — `Notifier` Interface (swappable alerting)

```go
type Notifier interface {
    Notify(ctx context.Context, m domain.Monitor, downSince time.Time) error
}
```

Today: `LogNotifier` — writes a zerolog error line.  
Tomorrow: `EmailNotifier`, `SlackNotifier`, `FanOutNotifier` — zero service changes.

**Service injects `notifier.Notifier`, not `*LogNotifier`:**
```go
type monitorService struct {
    notifier notifier.Notifier  // interface — any implementation works
}
```

This is the same pattern as repositories in Stage 5. Interface = swap point.

---

### Concept 6 — Notify AFTER the Transaction Commits

**Wrong approach (notify inside the transaction):**
```
BEGIN
  INSERT incident         ← OK
  UPDATE status='down'    ← OK
  CALL slack.Send(alert)  ← external HTTP call
ROLLBACK (some error)     ← incident and status rolled back
                          ← but Slack already sent the alert! 😱
```

The on-call engineer gets paged for a phantom outage — the data was never saved.

**Right approach (notify after commit):**
```go
if txErr := s.transactor.RunInTx(ctx, func(ctx context.Context) error {
    s.incidents.Create(ctx, incident)
    s.monitors.SetStatus(ctx, id, "down")
    return nil
}); txErr != nil { return txErr }

// Transaction committed — now safe to call external systems.
s.notifier.Notify(ctx, monitor, downSince)  // ← happens OUTSIDE the transaction
```

**Rule:** Side-effects that can't be rolled back (HTTP calls, emails, queue messages) must happen AFTER the transaction commits.

**Python/Django equivalent:**
```python
with transaction.atomic():
    Incident.objects.create(...)
    Monitor.objects.filter(id=id).update(status="down")
# committed — safe to send email now
send_alert(monitor)
```

---

### Concept 7 — `domain.Monitor.Status` field

Added to `domain.Monitor`:
```go
Status string `gorm:"not null;default:'unknown';size:10" json:"status"`
```

Three values:
| Value | Meaning |
|-------|---------|
| `"unknown"` | Monitor just created, never checked |
| `"up"` | Last probe succeeded, no open incident |
| `"down"` | Last probe failed, incident is open |

`gorm:"default:'unknown'"` means AutoMigrate sets the DB column default — existing rows get the value automatically on schema migration.

**Why separate from `Active bool`?**

| Field | Controls |
|-------|---------|
| `active` | Whether the scheduler checks this monitor |
| `status` | What the last probe result was |

A paused monitor (`active=false`) still has `status="down"` if it was down when paused. They're orthogonal.

---

### Hinglish recap — Tujhe kya samajh aaya

Yaar, socho ek situation: tumhara monitor down ho gaya. Do cheezein honi chahiye ek saath:

1. **Incident row banana** — "haan yaar, yeh monitor 3:42pm pe down hua"
2. **Monitor ka status update karna** — `"down"` set karna API ke liye

**Problem:** Dono alag-alag DB writes hain. Agar beech mein kuch crash ho gaya:
- Incident bana, status nahi bana → API pe "UP" dikhta hai, incident row bola "DOWN" — confusing!
- Status bana, incident nahi bana → on-call ko pata hi nahi kab down hua!

**Solution: Transaction** — dono writes ek packet mein bandh karo.

```
Transaction start karo
  Write 1: incident insert karo
  Write 2: status "down" karo
Transaction commit karo → DONO ek saath land karte hain
```

Agar kuch bhi fail ho → **ROLLBACK** → dono writes cancel. Koi half-baked state nahi.

**GORM mein kaise?**
```go
db.Transaction(func(tx *gorm.DB) error {
    // nil return karo → COMMIT
    // error return karo → ROLLBACK (GORM automatically karta hai)
    return nil
})
```

**Notifier kab call karte ho?**  
Transaction ke BAAD. Socho: agar email pehle bheja, phir transaction rollback ho gaya... innocent engineer ko fake alert mila. Transaction commit ke baad notify karo — "agar alert gaya, toh data pakka save hai."

**Notifier interface kyun?**  
Aaj `LogNotifier` hai (bas log karta hai). Kal `SlackNotifier` ya `EmailNotifier` banana ho toh service ka ek line bhi nahi badlega — sirf `main.go` mein naya notifier inject karo.

Yahi hai **dependency inversion** — service ko parwah nahi "kaun notify karega", sirf "Notify() ka ek method milna chahiye."

---

### Self-check questions — Stage 9

1. **Transaction ka callback `nil` return karne se kya hota hai? `error` return karne se?**

2. **`txDB(ctx, r.db)` function kya karta hai? Ise sirf `r.db` use karne se kya fark hai?**

3. **`txKey struct{}` unexported kyun hai? `"tx"` string use karne se kya problem?**

4. **Notifier ko transaction ke andar call karne ki kya problem hai?**

5. **`Active bool` aur `Status string` dono alag-alag kyun rakhte hain?**

6. **`gorm:"default:'unknown'"` kya karta hai existing rows ke liye jab migration chalti hai?**

---

### What's next — Stage 10

- Handler unit tests (`httptest.NewRecorder`, mock `MonitorService`)
- `GET /api/monitors/{id}/incidents` endpoint
- Prometheus metrics (`/metrics` endpoint)

---

## Stage 10 — JWT Auth (already built in Stage 7)

> **Note:** Stage 10's spec (register, login, auth middleware, protected monitor routes) was implemented in full during Stage 7. This section documents the concepts and answers the pre-check questions.

### Files that implement Stage 10

| File | Role |
|------|------|
| `internal/service/user_service.go` | bcrypt hashing, JWT signing |
| `internal/handler/auth_handler.go` | `POST /api/auth/register`, `POST /api/auth/login` |
| `internal/middleware/auth.go` | JWT verify middleware, `UserIDFromCtx` |
| `cmd/server/main.go` | Route wiring: public `/auth/*`, protected `/monitors` |

---

### Concept 1 — bcrypt password hashing

**One-way hash — you can NEVER reverse it:**

```go
hashed, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
// stored in DB: "$2a$12$<22-char-salt><31-char-hash>"

bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(inputPassword))
// re-hashes inputPassword with the embedded salt, compares in constant time
```

**Cost 12 = ~250ms on modern hardware.** Each +1 to cost doubles the work.
- Cost 4: brute-force is cheap (attacker can test millions of passwords/second)
- Cost 12: standard for web apps (login feels instant, brute-force is expensive)
- Cost 15+: login feels slow

**The salt is embedded in the hash string itself.** No separate salt column needed.

**Python:** `bcrypt.hashpw(password.encode(), bcrypt.gensalt(rounds=12))`  
**Node.js:** `await bcrypt.hash(password, 12)`

---

### Concept 2 — JWT structure (header.payload.signature)

A JWT is three base64url strings joined by `.`:

```
eyJhbGciOiJIUzI1NiJ9  .  eyJ1c2VyX2lkIjoxfQ  .  abc123xyz
    ↑ header                ↑ payload               ↑ signature
  {"alg":"HS256"}        {"user_id":1,"exp":...}   HMAC-SHA256 of header+payload
```

**Signing (login):**
```go
claims := PulseClaims{
    UserID: u.ID,
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
        Issuer: "pulse",
    },
}
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
tokenStr, _ := token.SignedString(jwtSecret)
```

**Verifying (middleware):**
```go
claims := &PulseClaims{}
token, err := jwt.ParseWithClaims(tokenStr, claims, keyFunc)
// ParseWithClaims: decodes → verifies signature → checks expiry
```

**Python:** `jwt.encode(payload, secret, algorithm="HS256")` / `jwt.decode(token, secret, algorithms=["HS256"])`  
**Node.js:** `jwt.sign(payload, secret, { expiresIn: "24h" })` / `jwt.verify(token, secret)`

---

### Concept 3 — The "none algorithm" attack (why the signing method check matters)

**The attack:**

```
Attacker crafts a token:
  header:  { "alg": "none" }
  payload: { "user_id": 1, "exp": 9999999999 }
  signature: (empty)
```

If the JWT library skips the algorithm check, it calls the keyfunc with a "none" method, gets back the secret, then says "oh, this uses 'none' — no signature needed" and **accepts the token as valid** with no verification at all.

**The guard in auth.go:**
```go
func(t *jwt.Token) (any, error) {
    if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
        return nil, errors.New("unexpected signing method")
    }
    return jwtSecret, nil
}
```

If `t.Method` is anything other than `*jwt.SigningMethodHMAC` — "none", RSA, ECDSA — the keyfunc returns an error **before** the secret is used. `jwt.ParseWithClaims` sees the error and fails the parse.

**Same rule in other languages:**
- Python: `jwt.decode(token, secret, algorithms=["HS256"])` — explicitly whitelist algorithms
- Node.js: `jwt.verify(token, secret, { algorithms: ["HS256"] })` — same

---

### Concept 4 — `claims["sub"]` float64 problem and how `PulseClaims` avoids it

**The problem with `jwt.MapClaims`:**

```go
// MapClaims = map[string]interface{}
// JSON decoder maps ALL numbers to float64 in Go
claims := jwt.MapClaims{}
jwt.ParseWithClaims(tokenStr, &claims, ...)

userID := claims["user_id"]   // type: interface{}
// You need:
id := uint(claims["user_id"].(float64))  // awkward; loses precision above 2^53
```

This happens because JSON has no integer type — `42` and `42.0` are the same. Go's `encoding/json` always decodes JSON numbers into `float64` when the target is `interface{}`.

**How `PulseClaims` avoids it:**

```go
type PulseClaims struct {
    UserID uint `json:"user_id"`  // ← Go knows the target type at decode time
    jwt.RegisteredClaims          // handles exp, iat, iss natively
}

claims := &PulseClaims{}
jwt.ParseWithClaims(tokenStr, claims, ...)

userID := claims.UserID  // ← already uint; no conversion, no type assertion
```

When you parse into a typed struct, `encoding/json` decodes the JSON number directly into `uint` — no float64 intermediate.

**Rule of thumb:** Always parse JWT into a typed struct when you have a known schema. Use `MapClaims` only when the schema is dynamic.

---

### Concept 5 — The login → token → verify flow end-to-end

```
1. Client          →   POST /api/auth/login {"email":"…","password":"…"}
2. AuthHandler     →   h.users.Login(ctx, email, password)
3. UserService     →   users.ByEmail(ctx, email)              DB fetch
4. UserService     →   bcrypt.CompareHashAndPassword(...)     constant-time compare
5. UserService     →   jwt.NewWithClaims(HS256, PulseClaims{UserID: u.ID, ...})
6. UserService     →   token.SignedString(jwtSecret)          HMAC-SHA256 sign
7. AuthHandler     ←   returns {"token":"eyJ..."}

─── next request ───────────────────────────────────────────────────────────────

8. Client          →   GET /api/monitors  Authorization: Bearer eyJ...
9. Auth middleware →   jwt.ParseWithClaims(tokenStr, &claims, keyFunc)
                         a. Decode header + payload
                         b. Check alg == HMAC (none-algorithm guard)
                         c. Verify HMAC-SHA256 signature
                         d. Check ExpiresAt > now
10. Auth middleware→   context.WithValue(ctx, userIDKey, claims.UserID)
11. MonitorHandler →   middleware.UserIDFromCtx(r.Context())  → userID = 1
12. MonitorService →   ListMonitors(ctx, userID=1, ...)       → only user 1's monitors
```

The server never stores sessions. Every request is self-contained — the token carries the identity.

---

### Hinglish recap — Tujhe kya samajh aaya

Bhai, yeh poora flow samjho:

**Registration pe kya hota hai?**
- Password `bcrypt.GenerateFromPassword` se hash hota hai — original password permanently gone.
- Hash DB mein save hota hai. Koi plain password kabhi DB mein nahi jaata.
- `cost=12` matlab attacker ke liye ek password try karna 250ms lagta hai. 1 crore tries = 29 din.

**Login pe kya hota hai?**
- DB se hashed password fetch karo.
- `bcrypt.CompareHashAndPassword` — input password ko same salt se dobara hash karo, compare karo.
- Match? → JWT sign karo aur return karo.
- JWT = credit card jaisa — tumhare credentials baar baar nahi bhejna, token bhejo.

**Auth middleware kya karta hai?**
- Har protected request pe: "Authorization: Bearer eyJ..." header padhta hai.
- JWT verify karta hai: signature sahi hai? Expire nahi hua?
- `alg=none` attack guard: sirf HMAC tokens accept karo, baaki sab reject.
- UserID context mein daalta hai → handler `UserIDFromCtx` se nikalta hai.

**`claims["sub"]` float64 kyun?**
- JSON mein integers ka koi alag type nahi. `42` aur `42.0` same hai.
- Go ke `encoding/json` har number ko `float64` mein decode karta hai jab target `interface{}` ho.
- Fix: typed struct `PulseClaims{UserID uint}` — JSON directly `uint` mein jaata hai.

---

### Self-check questions — Stage 10

1. **bcrypt `CompareHashAndPassword` ko original password kyun nahi chahiye? Salt kahan se aata hai?**

2. **JWT ka `alg=none` attack kya hai? Guard kaisa kaam karta hai?**

3. **`jwt.MapClaims` se `claims["user_id"]` nikalne pe float64 kyun milta hai?**

4. **Server ke paas session store kyun nahi hai? Stateless auth ka matlab kya hai?**

5. **bcrypt cost 4 vs cost 12 — dono mein kya fark hai? Kya cost 20 use karna chahiye?**

---

### What's next — Stage 11

- Handler unit tests (`httptest.NewRecorder`, mock `MonitorService`)
- `GET /api/monitors/{id}/incidents` endpoint
- Prometheus metrics (`/metrics` endpoint)

---

## Stage 11 — Logging, Graceful Shutdown, /health

### What we built

| File | Change |
|------|--------|
| `internal/platform/cache/cache.go` | Added `Ping(ctx) error` — used by health handler |
| `internal/service/monitor_service.go` | Injected `zerolog.Logger`; replaced two `fmt.Printf` with `s.log.Warn()` |
| `cmd/server/main.go` | Replaced `fmt.Fprintf` in `due` closure; real `/health` pinging Postgres + Redis |

---

### Concept 1 — Structured logging with zerolog

**The problem with `fmt.Printf`:**
```
⚠️  record outcome: update next check for monitor 42: pq: connection reset
```
This is a plain string. You can't filter it by monitor_id, you can't search by error type, you can't count "how many times did this happen for monitor 42 in the last hour?"

**Structured logging (zerolog):**
```go
s.log.Warn().
    Err(err).
    Uint("monitor_id", o.Job.MonitorID).
    Msg("record outcome: update next check failed (non-fatal)")
```

Produces JSON (in production):
```json
{"level":"warn","error":"pq: connection reset","monitor_id":42,"message":"record outcome: update next check failed (non-fatal)","time":"2026-07-14T10:23:01Z"}
```

Now Datadog/Loki/Splunk can query: `level:warn AND monitor_id:42` — instant filter. Fields are typed, not embedded in a string.

**zerolog builder pattern:**
- Each `.Str(k, v)` / `.Uint(k, v)` / `.Err(err)` adds a JSON field to the event buffer.
- `.Msg("...")` is the terminal call — flushes the event. Nothing is written until `.Msg()`.
- `logger.Info()` creates a new event; `.Error()` / `.Warn()` for higher severity.

**Python:** `logger.warning("...", extra={"monitor_id": 42})`  
**Node.js/pino:** `log.warn({ monitorId: 42, err }, "...")`

---

### Concept 2 — Why inject logger into the service (not use a global)

We added `log zerolog.Logger` to `monitorService` and pass it in `NewMonitorService`.

**Global logger problem:**
```go
// Anywhere in the codebase:
log.Error().Msg("something")   // zerolog/log global
```
- Tests can't replace it — every test shares the same global logger.
- No per-component context (can't tag all service logs with `"component":"monitor_service"`).
- Harder to test that a specific warning was emitted.

**Injected logger:**
```go
// In tests:
testLog := zerolog.Nop()   // discard all logs — no noise in test output
svc := service.NewMonitorService(..., testLog)
```

This is the same dependency-injection principle as repositories — any dependency that has side effects (I/O, logging) should be injected, not imported as a global.

---

### Concept 3 — Graceful shutdown: the exact sequence

```
SIGTERM received
      │
      ▼
pipelineCancel()          ← cancels the scheduler context
      │
      │  scheduler goroutine sees ctx.Done() → stops ticking
      │  workers drain their current check (takes ≤ checkTimeout seconds)
      │  outcomes channel closes → outcome recorder goroutine exits
      ▼
srv.Shutdown(10s timeout)  ← stops accepting new connections
      │
      │  in-flight HTTP requests complete
      │  idle connections close
      ▼
main() returns             ← process exits cleanly
```

**Why cancel the pipeline BEFORE `srv.Shutdown`?**

Option A (current — pipeline first):
- Pipeline cancel is non-blocking — it just signals the context.
- Workers finish their current check concurrently while HTTP server is still live.
- `srv.Shutdown` then drains the HTTP layer.
- Total time ≈ max(worker drain, HTTP drain) — they overlap.

Option B (HTTP first):
- `srv.Shutdown` blocks for up to 10s (HTTP drain).
- THEN pipeline cancel + worker drain (another checkTimeout seconds).
- Total time ≈ HTTP drain + worker drain — sequential, longer.

Option A is better. Pipeline cancel is cheap (channel close), so doing it first costs nothing and gives workers the maximum time to finish.

**In production, SIGTERM comes from:**
- **Kubernetes:** rolling deploy / `kubectl delete pod` → sends SIGTERM to PID 1, waits `terminationGracePeriodSeconds` (default 30s), then SIGKILL.
- **systemd:** `systemctl stop` → SIGTERM → SIGKILL after `TimeoutStopSec`.
- **Docker:** `docker stop` → SIGTERM → SIGKILL after 10s.
- **Cloud Run / ECS / Fly.io:** same SIGTERM → SIGKILL pattern.

If your app ignores SIGTERM and takes > 30s to stop, Kubernetes sends SIGKILL — in-flight requests are dropped immediately. Graceful shutdown is what buys you those 30 seconds.

---

### Concept 4 — Real `/health` endpoint

**What a health endpoint is for:**
Load balancers and Kubernetes readiness probes call `/health` every few seconds. A non-200 response means "stop routing traffic to this pod."

**Old (fake) health:**
```go
w.Write([]byte(`{"status":"ok"}`))  // always 200, even if DB is down
```
Useless — the load balancer routes traffic to a broken pod.

**New (real) health:**
```go
// Ping Postgres:
sqlDB, _ := db.DB()
sqlDB.PingContext(ctx)   // sends "SELECT 1" equivalent

// Ping Redis:
c.Ping(ctx)              // sends Redis PING command
```

Response codes:
- **200** — both OK → load balancer routes traffic here
- **503** — Postgres or Redis down → load balancer routes AWAY from this pod

**3-second deadline (`context.WithTimeout`):**
Health checks must complete fast. If the DB is hung and the health check takes 30s, the load balancer thinks the pod is slow to respond and marks it as unhealthy regardless. A 3s deadline forces a fast failure.

**`disabled` vs `down` for Redis:**
If Redis wasn't configured (`cacheClient == nil`), the service deliberately runs without it — that's not a failure, it's a config choice. We report `"redis":"disabled"` and don't trigger 503. Only a configured-but-unreachable Redis returns `"down"`.

**Python/Flask:**
```python
@app.route("/health")
def health():
    checks = {"postgres": ping_db(), "redis": ping_redis()}
    code = 503 if "down" in checks.values() else 200
    return jsonify({"status": "ok" if code == 200 else "degraded", **checks}), code
```

**Node.js/Express:**
```javascript
app.get("/health", async (req, res) => {
    const pg = await pool.query("SELECT 1").then(() => "ok").catch(() => "down")
    const redis = await redisClient.ping().then(() => "ok").catch(() => "down")
    const ok = pg === "ok" && redis === "ok"
    res.status(ok ? 200 : 503).json({ status: ok ? "ok" : "degraded", pg, redis })
})
```

---

### Hinglish recap — Tujhe kya samajh aaya

**fmt.Printf hataaya kyun?**

`fmt.Printf("monitor 42 failed: pq: connection reset")` — yeh ek plain string hai. Log aggregator ko kaise pata chalega ki "monitor_id=42" ke liye filter karna hai? Kaise count karein kitni baar yeh error aaya?

`zerolog` structured events likhta hai:
```json
{"level":"warn","monitor_id":42,"error":"pq: connection reset"}
```
Ab Datadog mein `monitor_id:42 AND level:warn` search karo — seedha milega.

**Logger inject kyun kiya service mein?**

Global logger (`log.Error().Msg(...)`) use karne se test mein noise aata hai aur tum test nahi kar sakte "kya warning aayi ya nahi." Inject karo → test mein `zerolog.Nop()` pass karo (silent logger) → clean tests.

**SIGTERM kya hota hai?**

Kubernetes deploy karta hai → purana pod band karna hai → `SIGTERM` bhejta hai → "bhai, gracefully band ho ja." 30 seconds ke baad agar band nahi hua → `SIGKILL` → forcefully murder.

Tumhara app agar SIGTERM handle nahi karta, in-flight HTTP requests drop ho jaate hain — client ko 502 milta hai.

**Graceful shutdown ka sequence:**
1. `pipelineCancel()` → scheduler ruk jaata hai, workers apna current check finish karte hain
2. `srv.Shutdown(10s)` → HTTP server nayi connections nahi leta, purani finish hone deta hai
3. Main exits → process cleanly band

**Health check 200 vs 503 kyun important hai?**

Load balancer har pod ko ping karta hai. 200 → "yahan traffic bhejo." 503 → "is pod se traffic hata lo, DB down hai." Agar fake 200 return karo toh broken pod pe traffic jaata rehega — users ko errors milte hain.

---

### Self-check questions — Stage 11

1. **`fmt.Printf` se zerolog mein switch karne ka main faida kya hai log aggregation ke context mein?**

2. **SIGTERM aur SIGKILL mein kya fark hai? Kya tum SIGKILL ko gracefully handle kar sakte ho?**

3. **Pipeline `srv.Shutdown` se pehle kyun cancel karte hain? Ulta karne se kya hoga?**

4. **Health check 503 return kare toh Kubernetes kya karta hai? 200 return karta raha toh kya problem?**

5. **`context.WithTimeout(r.Context(), 3*time.Second)` health handler mein kyun use karte hain?**

6. **Logger ko global kyun nahi rakhte? `zerolog.Nop()` tests mein kya karta hai?**

---

### What's next — Stage 12 (completed below)

---

## Stage 12 — Testing (Table-driven tests, mocking via interfaces, httptest, -race)

### Kya bana Stage 12 mein?

Teen cheezein test kiye:

1. **Worker Pool** (`internal/monitor/pool_test.go`) — pehle se exist karta tha Stage 7 se. `fakeChecker` ek fake HTTP client hai jo real network call nahi karta — seedha success ya failure return karta hai. 20 jobs → 5 workers → sab 20 outcomes produce karte hain. Context cancel test bhi.

2. **Service tests** (`internal/service/monitor_service_test.go`) — stub types har interface ke liye. Koi real DB, Redis, ya network nahi.

3. **Handler tests** (`internal/handler/monitor_handler_test.go`) — `httptest.NewRecorder` aur `httptest.NewRequest` se fake HTTP. Handler function directly call karte hain — koi running server nahi.

---

### Interface mockability — ye kyun possible hai?

Go mein interfaces **structural** hain (duck typing with compile-time checking):

```go
// service package mein define hai:
type MonitorService interface {
    CreateMonitor(ctx, userID, url, name string, interval, timeout int) (*Monitor, error)
    ListMonitors(ctx, userID, page, pageSize int) ([]Monitor, error)
    // ... 6 more methods
}

// test file mein:
type mockMonitorSvc struct {
    createMonitorFn func(...) (*domain.Monitor, error)
}

func (m *mockMonitorSvc) CreateMonitor(...) (*domain.Monitor, error) {
    return m.createMonitorFn(...)
}
// agar sab 8 methods implement karo → automatically MonitorService ban jaata hai
// koi "implements" keyword nahi — Go check karta hai compile time pe

var _ service.MonitorService = (*mockMonitorSvc)(nil) // compile-time guarantee
```

**Python mein:** `unittest.mock.MagicMock(spec=MonitorService)` ya `Protocol` class.
**Node.js mein:** `const mock = { createMonitor: jest.fn() }` — pure object, koi class nahi.

---

### Table-driven tests pattern

```go
// EK function, BAAR BAAR cases
tests := []struct {
    name      string
    url       string
    wantErrIs error
}{
    {"happy path", "https://example.com", nil},
    {"empty url", "",                     domain.ErrValidation},
    {"interval < 5", "https://x.com",    domain.ErrValidation},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {  // har case apna sub-test
        // arrange → act → assert
    })
}
```

**Naya case add karna = ek line** table mein. Koi duplicate code nahi.

**Python:** `@pytest.mark.parametrize("url,wantErr", [...])`
**Node.js:** `test.each([...])`("test %s", ...)

---

### httptest flow

```go
// 1. In-memory ResponseWriter banao — koi socket nahi
w := httptest.NewRecorder()

// 2. Fake request banao — koi TCP nahi
r := httptest.NewRequest(http.MethodPost, "/api/monitors",
    strings.NewReader(`{"url":"https://example.com",...}`))

// 3. Auth inject karo (real JWT bypass)
r = r.WithContext(middleware.WithUserID(r.Context(), 1))

// 4. Handler directly call karo
h.Create(w, r)

// 5. Recorder se assert karo
if w.Code != 201 { t.Errorf(...) }
```

**Python/FastAPI:** `client = TestClient(app); resp = client.post("/monitors", json={...})`
**Node.js:** `const resp = await request(app).post("/monitors").send({...})`
**Go:** handler function directly call — koi server start nahi hota.

---

### -race flag — kyun critical hai?

```bash
go test ./... -race -cover
```

`-race` flag **runtime par** memory access ko instrument karta hai:
- Har goroutine ke har read/write ko track karta hai
- Agar do goroutines ek hi variable ko bina sync ke access karein → **race report**

**Pulse mein kyun zaroori:**
Worker pool goroutines + channels use karta hai. RecordOutcome production mein goroutines se call hota hai. Race condition silently data corrupt kar sakta hai — test normally pass bhi ho sakta hai.

```
WARNING: DATA RACE
Write at 0x00c0001a4010 by goroutine 7:
Read at 0x00c0001a4010 by goroutine 8:
```

**NOTE:** `-race` ke liye `gcc` (CGO) chahiye. Is machine pe GCC install nahi hai — isliye `go test ./... -cover` use kiya. Production Linux servers pe `sudo apt install gcc` karke `-race` run kar sakte hain.

**Python:** `threading` module mein explicit locks lagane padte hain — no automatic detection.
**Node.js:** single-threaded — data races usually possible nahi (Worker Threads alag baat hai).

---

### stubTransactor — transaction testing trick

```go
type stubTransactor struct{}

func (stubTransactor) RunInTx(ctx context.Context, fn repository.TxFn) error {
    return fn(ctx)  // ← critical: fn ko call karo, nil return mat karo
}
```

**Kyon `fn(ctx)` call karna zaroori hai:**

Service mein:
```go
s.transactor.RunInTx(ctx, func(ctx context.Context) error {
    s.incidents.Create(ctx, incident)      // side-effect A
    s.monitors.SetStatus(ctx, id, "down")  // side-effect B
    return nil
})
```

Agar stub `fn` call na kare aur seedha `nil` return kare:
- Side-effect A aur B kabhi nahi chalenge
- Test verify nahi kar sakta ki incident create hua ya status set hua

`fn(ctx)` call karne se closure execute hota hai → stubs observe kar sakte hain.

---

### Test results

```
ok  github.com/nishantks908/pulse/internal/handler   coverage: 33.8%
ok  github.com/nishantks908/pulse/internal/monitor   coverage: 93.8%
ok  github.com/nishantks908/pulse/internal/service   coverage: 25.0%
```

Pool tests: 93.8% coverage (high — worker pool har path cover karta hai).
Service tests: 25% — because `RecordOutcome` ke andar bahut branches hain (UP→DOWN, DOWN→UP, already DOWN, save check, update next check — sab ke test hain lekin overall function count zyada hai).
Handler tests: 33.8% — Create + List cover hain; Get/Delete/Pause/Resume/Checks baaki hain.

---

### Self-check questions — Stage 12

1. **`fakeChecker` pool ko deterministically test karne deta hai — matlab kya? Real HTTP checker kyun use nahi kar sakte tests mein?**

2. **`var _ service.MonitorService = (*mockMonitorSvc)(nil)` line kya karti hai? Runtime ya compile time?**

3. **`stubTransactor` mein `fn(ctx)` call karna zaroori kyun hai? Sirf `nil` return karne se kya problem?**

4. **`httptest.NewRecorder()` aur real `http.ResponseWriter` mein kya fark hai?**

5. **`-race` flag ke liye CGO kyun chahiye? Ye flag normally kaun si bugs pakadta hai?**

6. **Table-driven tests ka sabse bada advantage kya hai? Ek naya edge case add karne ke liye kitni lines chahiye?**

---

### What's next — Stage 13

- `GET /api/monitors/{id}/incidents` endpoint
- Prometheus metrics (`/metrics` endpoint with request count, latency histograms)
- Rate limiting middleware
