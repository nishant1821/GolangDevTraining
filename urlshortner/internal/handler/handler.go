// internal/handler/handler.go
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nishantks908/urlshortener/internal/service"
	"github.com/nishantks908/urlshortener/internal/store"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// request/response ke shapes — JSON tags se field naam control hota hai
type shortenRequest struct {
	URL string `json:"url"`
}
type shortenResponse struct {
	Code string `json:"code"`
}

func (h *Handler) Shorten(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}

	// r.Context() pass kar raha hai — client disconnect hua toh
	// context cancel ho jaayega, ye production context-propagation point hai
	code, err := h.svc.Shorten(r.Context(), req.URL)
	fmt.Println(code)
	if err != nil {
		if errors.Is(err, service.ErrInvalidURL) {
			writeError(w, http.StatusBadRequest, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "server error")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(shortenResponse{Code: code})
}

func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code") // /{code} se nikala

	longURL, err := h.svc.Resolve(r.Context(), code)
	fmt.Println(longURL)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, "server error")
		}
		return
	}

	// asli redirect — 302 use kar rahe hain, kyun neeche
	http.Redirect(w, r, longURL, http.StatusFound) // 302
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
