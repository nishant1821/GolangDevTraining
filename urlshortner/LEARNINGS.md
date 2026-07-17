# URL Shortener — Project Learnings & Interview Prep

Project structure recap: `store` (data) → `service` (business logic) → `handler` (HTTP) → `main.go` (wiring/router).

## Core Learnings

1. **Layered architecture** — Store → Service → Handler → Router. Har layer sirf apne neeche wali layer ko jaanta hai.
2. **Dependency Injection via interfaces** — `service.go` mein `Store` interface hai, concrete `*InMemoryStore` nahi. Kal agar Postgres store banao, `service.go` ek line bhi change nahi hoga.
3. **Concurrency safety** — map ko `sync.Mutex` se guard kiya (`store.go`), counter ko `sync/atomic` se (`service.go`) — dono alag reasons se.
4. **Sentinel errors + `errors.Is`** — `ErrNotFound` aur `ErrInvalidURL` ko error-comparison ke liye use kiya, string-matching nahi.
5. **Base62 encoding** — counter number ko chhote alphanumeric code mein convert karna (jaise YouTube/bit.ly IDs).
6. **context.Context propagation** — `r.Context()` request se store tak pass hota hai (abhi unused hai kyunki in-memory hai, lekin DB call hoti toh cancellation kaam karta).

---

## Interview Q&A

### 1. Concurrency

**Q: Map ko mutex se protect kyun kiya, aur counter ko atomic se — dono alag kyun?**
A: Go map **not thread-safe** hai — concurrent read+write se `fatal error: concurrent map writes` (crash, recover bhi nahi hota, panic se bhi bura). Isliye `store.go` mein `sync.Mutex` poore map operation (`Save`/`Find`) ko lock karta hai. Counter (`service.go`) sirf ek `uint64` hai — us pe poora mutex lagana overkill hai, `atomic.AddUint64` ek CPU instruction level increment deta hai, bina lock ke, isliye faster. Python mein GIL ki wajah se yeh dono cheezein "safe by default" lagti hain (illusion), Node.js single-threaded hone ki wajah se issue hi nahi aata — Go truly concurrent hai isliye explicit protection chahiye.

**Q: Race condition kaise detect karoge?**
A: `go test -race ./...` — Go ka built-in race detector. Agar mutex hata do aur yeh command chalao, `data race` report aayega.

### 2. Interfaces & DI

**Q: `service.go` mein `Store` interface kyun define kiya jab sirf ek hi implementation (`InMemoryStore`) hai?**
A: **Testability aur decoupling** ke liye. Interface ki wajah se test mein ek fake/mock store inject kar sakte ho bina real map/DB touch kiye. Yeh Go ka "accept interfaces, return structs" principle hai. Python mein duck-typing se ye implicitly milta hai; Go mein explicit interface declare karna padta hai lekin implementation `implements` keyword ke bina automatically satisfy ho jaati hai (structural typing) — `InMemoryStore` ne kahin nahi likha "I implement Store", bas methods match ho gaye.

**Q: Agar `InMemoryStore` mein `Find` method ka signature thoda alag ho (jaise extra param), toh kya hoga?**
A: Compile error — `service.New(st)` fail karega kyunki `st` ab `Store` interface satisfy nahi karta. Yeh compile-time safety hai, jo Python/JS mein runtime tak nahi pakdi jaati.

### 3. Error handling

**Q: `errors.New("short code not found")` ko sentinel error kyun banaya, plain string kyun return nahi kiya?**
A: Taaki caller (`handler.go`) `errors.Is(err, store.ErrNotFound)` se reliably check kar sake ki yeh *specifically* not-found error hai, na ki koi aur error jiska message coincidentally match ho gaya. String comparison fragile hoti hai (typo, wrapping ke baad message change ho sakta hai); `errors.Is` wrapped errors (`fmt.Errorf("...: %w", err)`) ke through bhi dekh sakta hai.

**Q: `fmt.Errorf("save code: %w", err)` mein `%w` vs `%v` ka fark?**
A: `%w` error ko **wrap** karta hai — original error chain mein rehta hai, `errors.Is`/`errors.Unwrap` se access ho sakta hai. `%v` sirf string format karta hai, original error identity kho jaati hai.

### 4. HTTP semantics

**Q: Redirect ke liye `302 Found` use kiya, `301 Moved Permanently` kyun nahi?**
A: `301` browsers/CDNs **cache kar lete hain permanently** — agar kal short code ka target URL update karna ho, purana cached redirect hi chalta rahega. `302` har baar server se fresh check karwata hai. URL shorteners is wajah se generally 302 use karte hain (kabhi-kabhi analytics tracking ke liye bhi, taaki har click server tak aaye).

**Q: `POST /api/shorten` ke response mein `201 Created` kyun, `200 OK` kyun nahi?**
A: REST convention — naya resource (short code) create hua hai, `201` semantically correct hai aur usually `Location` header ke saath aata hai (yahan missing hai — improvement point: `w.Header().Set("Location", "/"+code)` add kar sakte ho).

### 5. Go language specifics

**Q: `make(map[string]string)` zaroori kyun hai, `var data map[string]string` kyun nahi chalega?**
A: `var data map[string]string` se **nil map** banta hai. Nil map se **read** karna safe hai (zero value milta hai), lekin **write** karna panic deta hai: `assignment to entry in nil map`. `make()` actual underlying hash table allocate karta hai. Python ka `{}` ya JS ka `{}` hamesha ready-to-use hota hai isliye yeh gotcha Go-specific hai.

**Q: Base62 mein `n == 0` ka special case kyun handle kiya?**
A: Loop `for n > 0` hai — agar `n` already 0 hai toh loop kabhi chalega hi nahi, aur empty string return hogi. Isliye pehle hi explicit check laga ke `alphabet[0]` (yaani `"0"`) return karte hain.

**Q: Base62 encode karte waqt digits reverse order mein kyun nikalte hain, aur fix kaise karte ho?**
A: `n % base` **sabse chhota (least significant) digit** deta hai pehle — jaise base10 mein `123 % 10 = 3` sabse pehle milta hai. Toh buffer mein digits ulte order mein bharte jaate hain, end mein do-pointer swap se palat dete hain — same technique jo integer-to-string reversal mein use hoti hai.

### 6. Design/architecture depth (senior-level questions)

**Q: Yeh in-memory store production mein use nahi kar sakte — kyun, aur Redis/Postgres mein switch karne ke liye kya change karna padega?**
A: (a) Server restart hote hi saara data gayab — no persistence. (b) Multiple server instances (horizontal scaling) mein har instance ka apna alag map hoga — inconsistent state. Switch karne ke liye sirf naya struct banega jo `Store` interface (Save+Find) satisfy kare — `service.go` aur `handler.go` **bilkul nahi badlenge**. Yehi interface-based design ka fayda hai.

**Q: Counter-based encoding (`atomic.AddUint64`) mein kya security/scaling issue hai?**
A: Sequential counter se **short codes predictable/enumerable** hain — koi bhi `/api/shorten` ke baad agla code guess kar sakta hai (`code+1`), jisse dusron ke URLs enumerate ho sakte hain. Fix: random ID generation (crypto-random base62) ya hash-based approach. Scaling issue: multiple server instances mein alag-alag counters honge → collision. Fix: distributed ID generator (Snowflake, DB sequence, UUID).

**Q: Agar do requests same longURL ke liye simultaneously `/shorten` call karein, kya hoga?**
A: Dono ko **alag-alag short codes** milenge (duplicate entries, same URL do baar store) — kyunki koi dedup check nahi hai. Improvement: pehle `longURL → code` reverse-lookup map maintain karo, ya hash(longURL) ko code base banao taaki same input → same output ho.

---

## Gaps / Open Improvements

- Saari test files abhi stub hain (`store_test.go`, `service_test.go`, `handler_test.go`, `base62_test.go`) — zero test coverage.
- Koi `Decode` function nahi hai base62 mein (sirf `Encode`) — asymmetric, agar future mein code se number wapas nikalna ho toh nahi kar paoge.
- Graceful shutdown nahi hai `main.go` mein (`http.ListenAndServe` seedha, no `context` based shutdown).
- `201 Created` response mein `Location` header missing.
- No dedup check for repeated `longURL` submissions.
- Sequential counter makes short codes predictable/enumerable.
