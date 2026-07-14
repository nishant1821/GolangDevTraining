package handler

// respond.go — Stage 7 (revised).
//
// Every HTTP response from Pulse — success or failure — is wrapped in the
// same JSON envelope so clients have a consistent shape to parse.
//
// Envelope shape:
//
//   Success:
//   {
//     "success":    true,
//     "data":       <payload>,
//     "request_id": "550e8400-..."
//   }
//
//   Failure:
//   {
//     "success":    false,
//     "error":      "not_found",          ← machine-readable error code
//     "message":    "monitor not found",  ← human-readable description
//     "request_id": "550e8400-..."
//   }
//
// WHY a consistent envelope?
//   - Clients write ONE error-handling path: check `success`, then read `error`.
//   - `request_id` lets support engineers correlate a client-reported error
//     with the exact server-side log line in seconds.
//   - Machine-readable `error` codes let clients branch on specific failures
//     (e.g., show "email already in use" only when error=="conflict").
//
// Python/FastAPI: JSONResponse(content={"success": True, "data": ...})
// Node.js/Express: res.json({ success: true, data: ..., requestId: ... })

import (
	"encoding/json" // json.NewEncoder — stream-encode to response body
	"errors"        // errors.Is — sentinel error matching
	"net/http"      // http.ResponseWriter, http.StatusOK, etc.

	"github.com/nishantks908/pulse/internal/domain"     // domain sentinel errors
	"github.com/nishantks908/pulse/internal/middleware" // middleware.RequestIDFromCtx
)

// ─────────────────────────────────────────────────────────────────────────────
// Envelope — the wire format
// ─────────────────────────────────────────────────────────────────────────────

// envelope is the JSON wrapper around every response.
// json:"omitempty" means the field is absent from JSON when its zero value.
//   - On success: Error and Message are empty strings → omitted.
//   - On failure: Data is nil → omitted.
//
// Python: dataclass or TypedDict with Optional fields
// Node.js/TypeScript: interface Envelope<T> { success: boolean; data?: T; error?: string; ... }
type envelope struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`      // present on success
	Error     string `json:"error,omitempty"`     // machine-readable code on failure
	Message   string `json:"message,omitempty"`   // human-readable description on failure
	RequestID string `json:"request_id,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON — write a success response
// ─────────────────────────────────────────────────────────────────────────────

// JSON wraps data in an envelope and writes it with the given HTTP status code.
// Reads the request ID from r's context (set by RequestID middleware).
//
// Usage:
//   JSON(w, r, http.StatusCreated, monitor)
//   JSON(w, r, http.StatusOK, monitors)
//
// Python: return JSONResponse({"success": True, "data": data, "request_id": id}, status_code=status)
// Node.js: res.status(status).json({ success: true, data, requestId })
func JSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Success:   true,
		Data:      data,
		RequestID: middleware.RequestIDFromCtx(r.Context()),
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Err — write an error response
// ─────────────────────────────────────────────────────────────────────────────

// Err maps a domain error to an HTTP status code and writes the error envelope.
//
// Mapping:
//   domain.ErrNotFound     → 404  error:"not_found"
//   domain.ErrValidation   → 422  error:"validation_failed"
//   domain.ErrConflict     → 409  error:"conflict"
//   domain.ErrUnauthorized → 401  error:"unauthorized"
//   anything else          → 500  error:"internal_error"  (no raw message exposed)
//
// Note: 500 responses return a generic message — never the raw Go error string.
// Exposing "pq: connection refused" or "gorm: record not found" to clients
// leaks internal details that can aid attackers.
//
// Python: raise HTTPException(status_code=404, detail={"error": "not_found", ...})
// Node.js: res.status(404).json({ success: false, error: "not_found", message: "..." })
func Err(w http.ResponseWriter, r *http.Request, err error) {
	code, errCode := httpCode(err)

	msg := err.Error()
	if code == http.StatusInternalServerError {
		msg = "an unexpected error occurred"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(envelope{
		Success:   false,
		Error:     errCode,
		Message:   msg,
		RequestID: middleware.RequestIDFromCtx(r.Context()),
	})
}

// httpCode maps a domain error to (HTTP status code, error code string).
// Centralising this mapping means every handler uses the same codes — no drift.
func httpCode(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrValidation):
		return http.StatusUnprocessableEntity, "validation_failed"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
