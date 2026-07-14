package handler

// monitor_handler.go — Stage 7.
//
// MonitorHandler exposes REST endpoints under /api/monitors:
//
//   POST   /api/monitors              — create a monitor (validated)
//   GET    /api/monitors              — list monitors (paginated)
//   GET    /api/monitors/{id}         — get one monitor
//   DELETE /api/monitors/{id}         — soft-delete
//   PATCH  /api/monitors/{id}/pause   — pause
//   PATCH  /api/monitors/{id}/resume  — resume
//   GET    /api/monitors/{id}/checks  — paginated check history
//
// Validation: go-playground/validator validates the create body
// so the service never receives blank URLs or out-of-range intervals.
//
// All responses use the consistent envelope from respond.go:
//   Success: {"success":true,"data":…,"request_id":"…"}
//   Failure: {"success":false,"error":"…","message":"…","request_id":"…"}
//
// Python/FastAPI:
//   @router.post("/monitors", response_model=Envelope[MonitorOut])
//   async def create(body: CreateMonitorIn, user=Depends(current_user)):
//       return await svc.create_monitor(...)
//
// Node.js/Express:
//   router.post("/monitors", auth, validate(schema), async (req, res) => { ... })

import (
	"encoding/json" // json.NewDecoder
	"errors"        // errors.As — unwrap validator.ValidationErrors
	"fmt"           // fmt.Sprintf — build validation message
	"net/http"      // http.Request, http.ResponseWriter
	"strconv"       // strconv.ParseUint, strconv.Atoi
	"strings"       // strings.Join — join field error messages

	"github.com/go-chi/chi/v5"                    // chi.URLParam
	"github.com/go-playground/validator/v10"      // validator.New, ValidationErrors

	"github.com/nishantks908/pulse/internal/domain"     // domain.Monitor, domain.ErrValidation
	"github.com/nishantks908/pulse/internal/middleware" // middleware.UserIDFromCtx
	"github.com/nishantks908/pulse/internal/service"    // service.MonitorService (interface)
)

// ─────────────────────────────────────────────────────────────────────────────
// Package-level validator — created once, reused per request (thread-safe)
// ─────────────────────────────────────────────────────────────────────────────

// validate is a singleton validator instance.
// validator.New() is expensive (reflection-based setup); creating it once at
// startup and reusing it across requests is the recommended pattern.
//
// Python: from pydantic import BaseModel — Pydantic compiles validators once per class.
// Node.js/Joi: const schema = Joi.object({...}) — schema compiled once, .validate() many times.
var validate = validator.New()

// ─────────────────────────────────────────────────────────────────────────────
// MonitorHandler — the handler struct
// ─────────────────────────────────────────────────────────────────────────────

// MonitorHandler holds the MonitorService interface.
// Interface dependency means tests can inject a fake without a real DB.
type MonitorHandler struct {
	svc service.MonitorService
}

// NewMonitorHandler constructs the handler — called in main.go.
func NewMonitorHandler(svc service.MonitorService) *MonitorHandler {
	return &MonitorHandler{svc: svc}
}

// ─────────────────────────────────────────────────────────────────────────────
// Create — POST /api/monitors
// ─────────────────────────────────────────────────────────────────────────────

// createReq is the validated request body for creating a monitor.
//
// validate tags:
//   required     — field must be present and non-zero
//   url          — must be a valid URL (scheme + host)
//   min=5        — IntervalSeconds: minimum 5 seconds (avoid hammering sites)
//   max=86400    — IntervalSeconds: max 24 hours (sensible ceiling)
//   min=1        — TimeoutSeconds: minimum 1 second
//   max=60       — TimeoutSeconds: max 60 seconds
//
// Python/Pydantic:
//   class CreateMonitorIn(BaseModel):
//       url: AnyUrl
//       name: str
//       interval_seconds: int = Field(ge=5, le=86400)
//       timeout_seconds:  int = Field(ge=1, le=60)
//
// Node.js/Joi:
//   const schema = Joi.object({
//     url: Joi.string().uri().required(),
//     interval_seconds: Joi.number().min(5).max(86400).required(),
//   })
type createReq struct {
	URL             string `json:"url"              validate:"required,url"`
	Name            string `json:"name"             validate:"required,min=1,max=255"`
	IntervalSeconds int    `json:"interval_seconds" validate:"required,min=5,max=86400"`
	TimeoutSeconds  int    `json:"timeout_seconds"  validate:"required,min=1,max=60"`
}

// Create parses + validates the body, then calls CreateMonitor.
// Returns 201 Created with the new monitor inside the success envelope.
func (h *MonitorHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body createReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// JSON syntax error — not a validation error, return 400 Bad Request.
		Err(w, r, fmt.Errorf("%w: %s", domain.ErrValidation, "invalid JSON body"))
		return
	}

	// validate.Struct runs all `validate:"..."` tags on the struct fields.
	// Returns nil if all pass; ValidationErrors if any fail.
	//
	// Python: body = CreateMonitorIn(**raw)  ← Pydantic raises ValidationError on failure
	// Node.js: const { error } = schema.validate(body)
	if err := validate.Struct(body); err != nil {
		Err(w, r, fmt.Errorf("%w: %s", domain.ErrValidation, validationMsg(err)))
		return
	}

	monitor, err := h.svc.CreateMonitor(r.Context(), userID,
		body.URL, body.Name, body.IntervalSeconds, body.TimeoutSeconds)
	if err != nil {
		Err(w, r, err)
		return
	}

	JSON(w, r, http.StatusCreated, monitor)
}

// ─────────────────────────────────────────────────────────────────────────────
// List — GET /api/monitors?page=1&page_size=20
// ─────────────────────────────────────────────────────────────────────────────

func (h *MonitorHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	page := queryInt(r, "page", 1)
	pageSize := queryInt(r, "page_size", 20)

	monitors, err := h.svc.ListMonitors(r.Context(), userID, page, pageSize)
	if err != nil {
		Err(w, r, err)
		return
	}
	if monitors == nil {
		monitors = []domain.Monitor{} // never return null — always return []
	}

	JSON(w, r, http.StatusOK, monitors)
}

// ─────────────────────────────────────────────────────────────────────────────
// Get — GET /api/monitors/{id}
// ─────────────────────────────────────────────────────────────────────────────

func (h *MonitorHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		Err(w, r, fmt.Errorf("%w: invalid monitor id", domain.ErrValidation))
		return
	}

	monitor, err := h.svc.GetMonitor(r.Context(), id, userID)
	if err != nil {
		Err(w, r, err)
		return
	}

	JSON(w, r, http.StatusOK, monitor)
}

// ─────────────────────────────────────────────────────────────────────────────
// Delete — DELETE /api/monitors/{id}
// ─────────────────────────────────────────────────────────────────────────────

func (h *MonitorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		Err(w, r, fmt.Errorf("%w: invalid monitor id", domain.ErrValidation))
		return
	}

	if err := h.svc.DeleteMonitor(r.Context(), id, userID); err != nil {
		Err(w, r, err)
		return
	}

	// 204 No Content — action succeeded, no body.
	w.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────────────────────────────────────────────────────────
// Pause — PATCH /api/monitors/{id}/pause
// ─────────────────────────────────────────────────────────────────────────────

func (h *MonitorHandler) Pause(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		Err(w, r, fmt.Errorf("%w: invalid monitor id", domain.ErrValidation))
		return
	}

	if err := h.svc.PauseMonitor(r.Context(), id, userID); err != nil {
		Err(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────────────────────────────────────────────────────────
// Resume — PATCH /api/monitors/{id}/resume
// ─────────────────────────────────────────────────────────────────────────────

func (h *MonitorHandler) Resume(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id, err := parseID(r, "id")
	if err != nil {
		Err(w, r, fmt.Errorf("%w: invalid monitor id", domain.ErrValidation))
		return
	}

	if err := h.svc.ResumeMonitor(r.Context(), id, userID); err != nil {
		Err(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ─────────────────────────────────────────────────────────────────────────────
// Checks — GET /api/monitors/{id}/checks?page=1&page_size=50
// ─────────────────────────────────────────────────────────────────────────────

// Checks returns paginated probe history for one monitor.
//
// Query params:
//   page      — 1-indexed page (default 1)
//   page_size — items per page (default 50, max sensible = 200)
//
// The service enforces ownership — users only see their own monitors' checks.
//
// Response example:
//   {
//     "success": true,
//     "data": [
//       {"id":1,"checked_at":"…","status_code":200,"response_time_ms":42,"up":true},
//       …
//     ],
//     "request_id": "…"
//   }
func (h *MonitorHandler) Checks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// {id} is the MONITOR id, not the check id.
	id, err := parseID(r, "id")
	if err != nil {
		Err(w, r, fmt.Errorf("%w: invalid monitor id", domain.ErrValidation))
		return
	}

	page := queryInt(r, "page", 1)
	pageSize := queryInt(r, "page_size", 50)

	checks, err := h.svc.GetCheckHistory(r.Context(), id, userID, page, pageSize)
	if err != nil {
		Err(w, r, err)
		return
	}
	if checks == nil {
		checks = []domain.Check{}
	}

	JSON(w, r, http.StatusOK, checks)
}

// ─────────────────────────────────────────────────────────────────────────────
// Private helpers
// ─────────────────────────────────────────────────────────────────────────────

// parseID reads a chi URL param and converts it to uint.
//
// chi URL params are always strings; we centralise conversion here
// instead of duplicating strconv.ParseUint in every handler method.
func parseID(r *http.Request, key string) (uint, error) {
	n, err := strconv.ParseUint(chi.URLParam(r, key), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(n), nil
}

// queryInt reads an integer query param with a default fallback.
//
// Python: int(request.query_params.get("page", 1))
// Node.js: parseInt(req.query.page) || 1
func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return defaultVal
	}
	return n
}

// validationMsg turns validator.ValidationErrors into a human-readable string.
//
// validator.ValidationErrors is a slice of FieldError; each FieldError has:
//   .Field()  — struct field name (e.g., "URL")
//   .Tag()    — failing rule  (e.g., "required", "url", "min")
//   .Param()  — rule parameter (e.g., "5" for min=5)
//
// Example output: "URL: url; IntervalSeconds: min=5"
//
// Python: str(pydantic.ValidationError) — similar field+message pairs
// Node.js: Joi error.details.map(d => d.message).join("; ")
func validationMsg(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err.Error()
	}
	msgs := make([]string, len(ve))
	for i, fe := range ve {
		msgs[i] = fmt.Sprintf("%s: %s", fe.Field(), fe.Tag())
		if fe.Param() != "" {
			msgs[i] = fmt.Sprintf("%s=%s", msgs[i], fe.Param())
		}
	}
	return strings.Join(msgs, "; ")
}
