package user

import (
	"encoding/json"
	// "errors"
	"net/http"

	// "gorm.io/gorm"

	"notes-api/internal/auth"
	"notes-api/internal/response"
)

// Handler — HTTP layer. Store ko andar leta hai (wahi urlshortner wala injection pattern).
type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// request body shapes — Node mein tu inline req.body.email use kar leta tha,
// Go mein strongly-typed rehne ke liye struct banate hain jisme JSON decode hoga.
type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register — POST /auth/register
// Node: app.post("/auth/register", async (req, res) => { const hash = await bcrypt.hash(...); ... })
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "could not process password", http.StatusInternalServerError)
		return
	}

	u := &User{Email: req.Email, Password: hashed}
	if err := h.store.Create(u); err != nil {
		// email already exists → unique constraint violation
		// Node mein tu Sequelize ka err.name === "SequelizeUniqueConstraintError" check karta tha,
		// yahan hum bas generic bata rahe hain (real project mein driver-specific error check karte hain)
		http.Error(w, "could not create user", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(u) // Password field json:"-" hai isliye leak nahi hoga
}

// YOUR TURN: Login likh — POST /auth/login
// Node: app.post("/auth/login", async (req,res) => {
//         const user = await User.findOne({ where: { email } })
//         const ok = await bcrypt.compare(password, user.password)
//         const token = jwt.sign({ userId: user.id }, SECRET)
//       })
//
// steps:
//  1. body decode kar (loginRequest mein)
//  2. h.store.FindByEmail(req.Email) call kar
//     - error aaye (gorm.ErrRecordNotFound ho ya kuch aur) → http.StatusUnauthorized, "invalid credentials"
//       (IMPORTANT self-check: email galat hai ya password galat hai — response SAME hona chahiye dono cases mein.
//        Alag-alag message dene se attacker ko pata chal jaata hai konsa email exist karta hai — "user enumeration")
//  3. auth.ComparePassword(user.Password, req.Password) call kar
//     - error aaye → wahi generic StatusUnauthorized "invalid credentials"
//  4. auth.GenerateToken(user.ID) call kar
//  5. response mein JSON bhej: {"token": "<token>"}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Step 2: user dhoondo. NA MILE → generic 401 (enumeration se bachne ke liye)
	user, err := h.store.FindByEmail(req.Email)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Step 3: password check. GALAT → wahi generic 401
	if err := auth.ComparePassword(user.Password, req.Password); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Step 4: token banao
	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		http.Error(w, "could not generate token", http.StatusInternalServerError)
		return
	}

	// Step 5: JSON bhejo
	response.JSON(w, http.StatusOK, map[string]string{"token": token})
}
