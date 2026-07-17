package note

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"notes-api/internal/auth"
	"notes-api/internal/response"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

type noteRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// List — GET /notes
// userID context se aata hai (middleware ne daala tha) — request body/params se NAHI,
// warna client khud hi "userId: 999" bhej ke kisi aur ka data maang sakta hai.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r.Context())

	notes, err := h.store.ListByUser(userID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "could not fetch notes")
		return
	}
	response.JSON(w, http.StatusOK, notes)
}

// Create — POST /notes
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r.Context())

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	n := &Note{Title: req.Title, Body: req.Body, UserID: userID}
	if err := h.store.Create(n); err != nil {
		response.Error(w, http.StatusInternalServerError, "could not create note")
		return
	}
	response.JSON(w, http.StatusCreated, n)
}

// YOUR TURN: Get, Update, Delete — teeno ka common skeleton same hai:
//
//  1. userID, _ := auth.GetUserID(r.Context())
//
//  2. URL se {id} nikaalo aur uint mein convert karo:
//     idStr := chi.URLParam(r, "id")
//     id, err := strconv.ParseUint(idStr, 10, 64)
//     if err != nil { response.Error(w, http.StatusBadRequest, "invalid id"); return }
//     (URL params hamesha string aate hain — "abc" bhi aa sakta hai, isliye convert error bhi handle karna)
//
//  3. store method call karo — h.store.GetByID(uint(id), userID) / Update(...) / Delete(...)
//
//  4. error check — DO tarah ke error alag treat karne hain:
//     if errors.Is(err, gorm.ErrRecordNotFound) {
//     response.Error(w, http.StatusNotFound, "note not found")   // NOT 403 — pehle discuss kiya tha
//     return
//     }
//     if err != nil {
//     response.Error(w, http.StatusInternalServerError, "something went wrong")
//     return
//     }
//
//  5. success response:
//     Get    → response.JSON(w, http.StatusOK, note)
//     Update → response.JSON(w, http.StatusOK, <updated note, ya bas {"status":"updated"}>)
//     Delete → w.WriteHeader(http.StatusNoContent)  (204 — body nahi bhejte delete pe)
//
// Update ke liye request body bhi decode karna hoga (noteRequest jaisa Create mein kiya).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	note, err := h.store.GetByID(uint(id), userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(w, http.StatusNotFound, "note not found")
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	response.JSON(w, http.StatusOK, note)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.GetUserID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	// body decode — Create jaisa
	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// store.Update call — WHERE id AND user_id andar hi hai, ownership yahin enforce
	err = h.store.Update(uint(id), userID, req.Title, req.Body)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(w, http.StatusNotFound, "note not found")
		return
	}
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"message": "updated"})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	// TODO
	userId, _ := auth.GetUserID(r.Context())

	idStr := chi.URLParam(r, "id")

	id, err := strconv.ParseInt(idStr, 10, 64)

	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	err = h.store.Delete(uint(id), userId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(w, http.StatusNotFound, "Note not found!")
		return
	}

	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
