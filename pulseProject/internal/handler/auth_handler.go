package handler

// auth_handler.go — Stage 7.
//
// AuthHandler exposes two public endpoints (no JWT required):
//
//   POST /auth/register  → 201 with user JSON in success envelope
//   POST /auth/login     → 200 with {"token":"<JWT>"} in success envelope
//
// All responses use the consistent envelope: {success, data/error, request_id}.

import (
	"encoding/json" // json.NewDecoder
	"net/http"      // http.Request, http.ResponseWriter, http.StatusCreated

	"github.com/nishantks908/pulse/internal/service" // service.UserService (interface)
)

// AuthHandler holds the user service interface.
type AuthHandler struct {
	users service.UserService
}

// NewAuthHandler is the constructor — called in main.go.
func NewAuthHandler(users service.UserService) *AuthHandler {
	return &AuthHandler{users: users}
}

// Register — POST /auth/register
//
// Body:   {"email":"…","password":"…"}
// 201:    {"success":true,"data":{"id":1,"email":"…"},"request_id":"…"}
// 409:    email already exists
// 422:    blank email or password too short
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusUnprocessableEntity)
		return
	}

	user, err := h.users.Register(r.Context(), body.Email, body.Password)
	if err != nil {
		Err(w, r, err)
		return
	}

	JSON(w, r, http.StatusCreated, user)
}

// Login — POST /auth/login
//
// Body:   {"email":"…","password":"…"}
// 200:    {"success":true,"data":{"token":"eyJ…"},"request_id":"…"}
// 401:    wrong email or password
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusUnprocessableEntity)
		return
	}

	_, tokenStr, err := h.users.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		Err(w, r, err)
		return
	}

	JSON(w, r, http.StatusOK, map[string]string{"token": tokenStr})
}
