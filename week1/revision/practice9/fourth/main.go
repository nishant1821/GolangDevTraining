package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5" // sirf v5 import karo — dono (chi aur chi/v5) ek saath import karna galat hai,
	// Go compiler "chi" naam ka conflict dega. Python mein jaise `import requests` sirf ek baar karte ho,
	// waise hi yahan sirf ek chi package chahiye.
)

// User struct — JSON tags batate hain field ka naam JSON mein kya hoga.
// Python: @dataclass ke fields, ya Pydantic model (class User(BaseModel): id: int; name: str; email: str)
// Node.js: { id: Number, name: String, email: String } jaisa object shape / TypeScript interface
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// In-memory store — map + Mutex (concurrent-safe banane ke liye)
// Python: global dict `users = {}` — lekin Python mein GIL ki wajah se dict ops kaafi had tak safe hote hain single process mein
// Node.js: ek object `let users = {}` — Node single-threaded hai (event loop), isliye lock ki zarurat hi nahi padti
// Go mein multiple goroutines (threads jaisa) same map ko ek saath likh sakte hain, isliye Mutex zaroori hai
var (
	users  = map[int]User{} // id -> User mapping. Python: dict[int, User]. JS: Map<number, User> ya plain object
	nextID = 1              // auto-increment ID counter, jaise SQL ka AUTO_INCREMENT
	mu     sync.Mutex       // map ko race condition se bachane ke liye lock
)

func main() {
	r := chi.NewRouter() // Express jaisa router. Node: const app = express(); Python: app = FastAPI() / Flask()

	// Route registration — Express ke app.get/app.post jaisa hi pattern
	r.Get("/users", listUsers)       // GET /users        -> Express: app.get('/users', listUsers)
	r.Get("/users/{id}", getUser)    // GET /users/:id     -> Express: app.get('/users/:id', getUser)
	r.Post("/users", createUser)     // POST /users        -> Express: app.post('/users', createUser)
	r.Put("/users/{id}", updateUser) // PUT /users/:id     -> Express: app.put('/users/:id', updateUser)
	r.Delete("/users/{id}", deleteUser) // DELETE /users/:id -> Express: app.delete('/users/:id', deleteUser)

	http.ListenAndServe(":8080", r) // server start. Node: app.listen(8080). Python (Flask): app.run(port=8080)
}

// listUsers — sab users ki list bhejta hai (GET /users)
func listUsers(w http.ResponseWriter, r *http.Request) {
	mu.Lock()         // lock lagao — jab tak hum map padh rahe hain koi aur isko modify na kare
	defer mu.Unlock() // function khatam hote hi lock apne aap khul jayega (defer = "finally" jaisa)

	// map se ek slice banao — map ka order guaranteed nahi hota Go mein (Python dict 3.7+ me insertion order guaranteed hota hai)
	list := []User{} // Python: list(users.values()). Node: Object.values(users) / Array.from(users.values())
	for _, u := range users {
		list = append(list, u) // append = Python list.append() / JS array.push(), lekin Go mein naya slice return hota hai
	}

	// response bhejne se pehle header set karna zaroori hai (WriteHeader se pehle)
	// "Content-Type" hi sahi header key hai — "Content-json" jaisa koi standard header nahi hota
	w.Header().Set("Content-Type", "application/json") // Node: res.setHeader('Content-Type', 'application/json') ya res.json() khud kar deta hai
	json.NewEncoder(w).Encode(list)                     // Python: return jsonify(list) / json.dumps(list). Node: res.json(list)
}

// getUser — ek single user by id bhejta hai (GET /users/{id})
func getUser(w http.ResponseWriter, r *http.Request) {
	// chi.URLParam(r, "id") — URL se {id} placeholder ka actual value nikalta hai (string ke roop mein)
	// Express: req.params.id | FastAPI: id: int path param | Flask: request.view_args['id']
	idStr := chi.URLParam(r, "id")

	// URL params hamesha string aate hain, isliye int mein convert karna padta hai
	// Python: int(id_str) — ValueError agar invalid. Node: Number(idStr) ya parseInt(idStr, 10)
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest) // 400 Bad Request. Node: res.status(400).send('invalid id')
		return                                             // yahan return zaroori hai warna neeche ka code bhi chal jayega
	}

	mu.Lock()
	user, ok := users[id] // comma-ok idiom — map mein key hai ya nahi, bina panic kiye check karta hai
	mu.Unlock()           // yahan defer nahi use kiya kyunki hume abhi unlock karke aage error-check bhi karna hai

	// Python: user = users.get(id); if user is None: abort(404)
	// Node: const user = users[id]; if (!user) return res.status(404).send('not found')
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound) // WriteHeader nahi — http.Error status + body dono set karta hai
		return                                                // return zaroori — warna neeche encode() zero-value User bhej dega
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user) // single user object encode — Node: res.json(user)
}

// createUser — naya user banata hai (POST /users)
func createUser(w http.ResponseWriter, r *http.Request) {
	var u User
	// request body ko User struct mein decode karo
	// Python (FastAPI): body auto-parse hota hai pydantic model se. Flask: u = request.get_json()
	// Node: express.json() middleware ke baad req.body already parsed object hota hai
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	mu.Lock()
	u.ID = nextID   // client se aaya hua ID ignore karo, server khud assign karega (jaise DB auto-increment)
	nextID++        // agla ID ke liye counter badhao
	users[u.ID] = u // map mein insert. Python: users[u.id] = u. Node: users[u.id] = u
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated) // 201 Created — resource successfully bana. Node: res.status(201)
	json.NewEncoder(w).Encode(u)      // naya bana hua user wapas bhejo (id samet), taaki client ko pata chale
}

// updateUser — existing user ko update karta hai (PUT /users/{id})
func updateUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	mu.Lock()
	if _, ok := users[id]; !ok {
		mu.Unlock()
		http.Error(w, "user not found", http.StatusNotFound) // pehle check karo user exist karta hai, warna silently create ho jayega
		return
	}
	u.ID = id       // URL ka id hi authoritative hai — body mein aaya id ignore/override karo
	users[id] = u   // purana overwrite. Python: users[id] = u. Node: users[id] = u
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u) // updated user wapas bhejo
}

// deleteUser — user ko delete karta hai (DELETE /users/{id})
func deleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	mu.Lock()
	if _, ok := users[id]; !ok {
		mu.Unlock()
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	delete(users, id) // Go ka built-in delete function. Python: del users[id]. Node: delete users[id] (object) / users.delete(id) (Map)
	mu.Unlock()

	w.WriteHeader(http.StatusNoContent) // 204 No Content — successfully deleted, body khali hai. Node: res.sendStatus(204)
}
