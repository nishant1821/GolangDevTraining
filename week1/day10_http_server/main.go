package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

// ─── Model ───────────────────────────────────────────────────────────────────

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ─── In-memory store ──────────────────────────────────────────────────────────
//
// Ek server pe kai requests ek saath aati hain — har request apni goroutine mein.
// Sab wahi users map ko padhte/likhte hain → bina Mutex race condition hogi.
// Isiliye har map access ke pehle mu.Lock() aur kaam ke baad mu.Unlock().

var (
	mu     sync.Mutex
	users  = map[int]User{}
	nextID = 1
)

// ─── Helper ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// GET /users → saari values slice mein daalo → 200
func listUsers(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	list := make([]User, 0, len(users))
	for _, u := range users {
		list = append(list, u)
	}
	mu.Unlock()

	writeJSON(w, http.StatusOK, list)
}

// GET /users/{id} → mila to 200, nahi to 404
func getUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	mu.Lock()
	u, ok := users[id]
	mu.Unlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// POST /users → body decode (galat JSON → 400) → nextID assign → store → 201
func createUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	mu.Lock()
	u := User{ID: nextID, Name: input.Name, Email: input.Email}
	users[nextID] = u
	nextID++
	mu.Unlock()

	writeJSON(w, http.StatusCreated, u)
}

// PUT /users/{id} → exist karta? → haan to update + 200, nahi to 404
func updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var input struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	mu.Lock()
	u, ok := users[id]
	if ok {
		u.Name = input.Name
		u.Email = input.Email
		users[id] = u
	}
	mu.Unlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// DELETE /users/{id} → tha to delete + 204, nahi to 404
func deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	mu.Lock()
	_, ok := users[id]
	if ok {
		delete(users, id)
	}
	mu.Unlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204 — body nahi hoti
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	// Go 1.22+ mux: method prefix ("GET ") aur path params ("{id}") built-in support karta hai
	mux := http.NewServeMux()

	mux.HandleFunc("GET /users", listUsers)
	mux.HandleFunc("GET /users/{id}", getUser)
	mux.HandleFunc("POST /users", createUser)
	mux.HandleFunc("PUT /users/{id}", updateUser)
	mux.HandleFunc("DELETE /users/{id}", deleteUser)

	addr := ":4000"
	fmt.Println("Server chal raha hai →  http://localhost" + addr)
	fmt.Println()
	fmt.Println("Routes:")
	fmt.Println("  GET    /users        → list all users")
	fmt.Println("  GET    /users/{id}   → get user by id")
	fmt.Println("  POST   /users        → create user  (body: {\"name\":\"...\",\"email\":\"...\"})")
	fmt.Println("  PUT    /users/{id}   → update user  (body: {\"name\":\"...\",\"email\":\"...\"})")
	fmt.Println("  DELETE /users/{id}   → delete user")

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Println("server error:", err)
	}
}
