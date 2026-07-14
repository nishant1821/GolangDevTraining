package handler_test

// monitor_handler_test.go — Stage 12.
//
// Tests the HTTP handler layer in ISOLATION — no real service, no DB, no Redis.
// We use net/http/httptest to fake the HTTP transport and a mockMonitorSvc to
// control what the service returns.
//
// WHAT IS HTTPTEST?
//   httptest.NewRecorder() → an in-memory http.ResponseWriter that captures:
//     - Status code (w.Code)
//     - Headers (w.Header())
//     - Body (w.Body.String())
//   httptest.NewRequest(method, path, body) → an *http.Request with no socket.
//
//   You call the handler FUNCTION DIRECTLY — no listening port, no TCP, no curl.
//   The handler writes to the recorder; you assert on the recorder.
//
//   Python: client = TestClient(app); resp = client.post("/monitors", json={...})
//   Node.js: const resp = await request(app).post("/monitors").send({...})
//   Go:      h.Create(w, r)  ← call the method directly, w is a recorder
//
// WHY CAN WE MOCK THE SERVICE?
//   MonitorHandler stores a service.MonitorService INTERFACE — not the concrete struct.
//   Any Go value that has the same methods satisfies the interface.
//   mockMonitorSvc has all 8 methods → it IS a MonitorService.
//   handler.NewMonitorHandler(mock) compiles → tests run with zero infrastructure.
//
// Python: unittest.mock.MagicMock(spec=MonitorService)
// Node.js: const svc = { createMonitor: jest.fn().mockResolvedValue(monitor) }

import (
	"bytes"          // bytes.NewBufferString — create request body from string
	"context"        // context.Background
	"encoding/json"  // json.NewDecoder — decode response body
	"net/http"       // http.StatusCreated etc.
	"net/http/httptest" // httptest.NewRecorder, httptest.NewRequest
	"strings"        // strings.NewReader — build JSON body
	"testing"        // *testing.T, t.Run, t.Error, t.Fatal

	"github.com/nishantks908/pulse/internal/domain"     // domain.Monitor, domain.ErrValidation
	"github.com/nishantks908/pulse/internal/handler"    // handler.NewMonitorHandler
	"github.com/nishantks908/pulse/internal/middleware" // middleware.WithUserID
	monpkg "github.com/nishantks908/pulse/internal/monitor" // monitor.Outcome (for RecordOutcome stub)
	"github.com/nishantks908/pulse/internal/service"    // service.MonitorService (interface)
)

// ─────────────────────────────────────────────────────────────────────────────
// mockMonitorSvc — satisfies service.MonitorService with configurable behaviour
// ─────────────────────────────────────────────────────────────────────────────
//
// DESIGN: same "nil-safe function field" pattern as the service test stubs.
//   Each method has a corresponding Fn field.
//   If set → call it; if nil → return a safe zero value.
//
// This way each test only configures the methods it exercises.
// A handler test for Create only needs createFn — the rest are silent.
type mockMonitorSvc struct {
	recordOutcomeFn    func(ctx context.Context, o monpkg.Outcome) error
	createMonitorFn    func(ctx context.Context, userID uint, url, name string, intervalSecs, timeoutSecs int) (*domain.Monitor, error)
	getMonitorFn       func(ctx context.Context, id, userID uint) (*domain.Monitor, error)
	listMonitorsFn     func(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error)
	pauseMonitorFn     func(ctx context.Context, id, userID uint) error
	resumeMonitorFn    func(ctx context.Context, id, userID uint) error
	deleteMonitorFn    func(ctx context.Context, id, userID uint) error
	getCheckHistoryFn  func(ctx context.Context, monitorID, userID uint, page, pageSize int) ([]domain.Check, error)
}

func (m *mockMonitorSvc) RecordOutcome(ctx context.Context, o monpkg.Outcome) error {
	if m.recordOutcomeFn != nil {
		return m.recordOutcomeFn(ctx, o)
	}
	return nil
}
func (m *mockMonitorSvc) CreateMonitor(ctx context.Context, userID uint, url, name string, intervalSecs, timeoutSecs int) (*domain.Monitor, error) {
	if m.createMonitorFn != nil {
		return m.createMonitorFn(ctx, userID, url, name, intervalSecs, timeoutSecs)
	}
	return &domain.Monitor{ID: 1, URL: url, Name: name, IntervalSeconds: intervalSecs}, nil
}
func (m *mockMonitorSvc) GetMonitor(ctx context.Context, id, userID uint) (*domain.Monitor, error) {
	if m.getMonitorFn != nil {
		return m.getMonitorFn(ctx, id, userID)
	}
	return &domain.Monitor{ID: id}, nil
}
func (m *mockMonitorSvc) ListMonitors(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error) {
	if m.listMonitorsFn != nil {
		return m.listMonitorsFn(ctx, userID, page, pageSize)
	}
	return []domain.Monitor{}, nil
}
func (m *mockMonitorSvc) PauseMonitor(ctx context.Context, id, userID uint) error {
	if m.pauseMonitorFn != nil {
		return m.pauseMonitorFn(ctx, id, userID)
	}
	return nil
}
func (m *mockMonitorSvc) ResumeMonitor(ctx context.Context, id, userID uint) error {
	if m.resumeMonitorFn != nil {
		return m.resumeMonitorFn(ctx, id, userID)
	}
	return nil
}
func (m *mockMonitorSvc) DeleteMonitor(ctx context.Context, id, userID uint) error {
	if m.deleteMonitorFn != nil {
		return m.deleteMonitorFn(ctx, id, userID)
	}
	return nil
}
func (m *mockMonitorSvc) GetCheckHistory(ctx context.Context, monitorID, userID uint, page, pageSize int) ([]domain.Check, error) {
	if m.getCheckHistoryFn != nil {
		return m.getCheckHistoryFn(ctx, monitorID, userID, page, pageSize)
	}
	return []domain.Check{}, nil
}

// compile-time interface check
var _ service.MonitorService = (*mockMonitorSvc)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// TestCreate — table-driven tests for POST /api/monitors
// ─────────────────────────────────────────────────────────────────────────────
//
// Covers: 201 happy path, 422 validation failure, 422 bad JSON, 401 no auth.
//
// HTTPTEST FLOW for each case:
//   1. Create recorder w = httptest.NewRecorder()
//   2. Create request r = httptest.NewRequest("POST", "/", body)
//   3. Optionally inject userID into r's context (middleware.WithUserID)
//   4. Call h.Create(w, r) directly — no HTTP server
//   5. Assert w.Code and w.Body
func TestCreate(t *testing.T) {
	// ── Table of test cases ─────────────────────────────────────────────────
	tests := []struct {
		name       string
		body       string  // raw JSON body string
		injectAuth bool    // whether to inject a userID via middleware.WithUserID
		svcFn      func(*mockMonitorSvc) // per-test mock configuration; nil = use defaults
		wantCode   int
		wantBodyContains string // substring to assert in the response body
	}{
		{
			name: "201 happy path",
			body: `{"url":"https://example.com","name":"My Site","interval_seconds":60,"timeout_seconds":10}`,
			injectAuth: true,
			svcFn: func(m *mockMonitorSvc) {
				// Return a realistic monitor — handler should write it as JSON with 201.
				m.createMonitorFn = func(_ context.Context, userID uint, url, name string, interval, timeout int) (*domain.Monitor, error) {
					return &domain.Monitor{
						ID:              42,
						UserID:          userID,
						URL:             url,
						Name:            name,
						IntervalSeconds: interval,
						TimeoutSeconds:  timeout,
						Active:          true,
					}, nil
				}
			},
			wantCode:         http.StatusCreated,
			wantBodyContains: `"id":42`,
		},
		{
			name: "422 missing url",
			body: `{"name":"My Site","interval_seconds":60,"timeout_seconds":10}`,
			injectAuth: true,
			wantCode:         http.StatusUnprocessableEntity,
			wantBodyContains: "error",
		},
		{
			name: "422 interval too small",
			body: `{"url":"https://example.com","name":"My Site","interval_seconds":3,"timeout_seconds":10}`,
			injectAuth: true,
			wantCode:         http.StatusUnprocessableEntity,
			wantBodyContains: "error",
		},
		{
			name:             "422 bad JSON",
			body:             `{this is not json}`,
			injectAuth:       true,
			wantCode:         http.StatusUnprocessableEntity, // handler treats JSON decode error as ErrValidation
			wantBodyContains: "error",
		},
		{
			name:             "401 no auth context",
			body:             `{"url":"https://example.com","name":"My Site","interval_seconds":60,"timeout_seconds":10}`,
			injectAuth:       false, // no middleware.WithUserID → UserIDFromCtx returns false
			wantCode:         http.StatusUnauthorized,
			wantBodyContains: "unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the mock service.
			mock := &mockMonitorSvc{}
			if tt.svcFn != nil {
				tt.svcFn(mock)
			}
			h := handler.NewMonitorHandler(mock)

			// Build the request.
			r := httptest.NewRequest(http.MethodPost, "/api/monitors",
				strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")

			// Inject auth if the test wants it.
			if tt.injectAuth {
				// middleware.WithUserID stores userID=1 under the same ctxKey that
				// the real Auth middleware uses. The handler calls UserIDFromCtx
				// and finds 1 — no real JWT processing needed.
				r = r.WithContext(middleware.WithUserID(r.Context(), 1))
			}

			w := httptest.NewRecorder()
			h.Create(w, r)

			// Assert status code.
			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d\nbody: %s", w.Code, tt.wantCode, w.Body.String())
			}

			// Assert body contains expected substring.
			if tt.wantBodyContains != "" && !strings.Contains(w.Body.String(), tt.wantBodyContains) {
				t.Errorf("body %q does not contain %q", w.Body.String(), tt.wantBodyContains)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestList — GET /api/monitors returns 200 with array
// ─────────────────────────────────────────────────────────────────────────────
//
// Tests that the List handler:
//   - Requires auth (401 without context user ID)
//   - Returns 200 with a JSON array when authed
//   - Always returns [] not null for an empty result
func TestList(t *testing.T) {
	t.Run("200 with monitors", func(t *testing.T) {
		mock := &mockMonitorSvc{}
		mock.listMonitorsFn = func(_ context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error) {
			return []domain.Monitor{
				{ID: 1, Name: "Alpha", URL: "https://alpha.com", UserID: userID},
				{ID: 2, Name: "Beta", URL: "https://beta.com", UserID: userID},
			}, nil
		}

		h := handler.NewMonitorHandler(mock)
		r := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
		r = r.WithContext(middleware.WithUserID(r.Context(), 99))
		w := httptest.NewRecorder()

		h.List(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200\nbody: %s", w.Code, w.Body.String())
		}

		// Decode the response body — handler wraps payload in {"success":true,"data":[...]}.
		// The JSON helper always uses this envelope shape (see respond.go).
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
			t.Fatalf("could not decode response body: %v\nbody: %s", err, w.Body.String())
		}
		if len(env.Data) != 2 {
			t.Errorf("got %d monitors, want 2", len(env.Data))
		}
	})

	t.Run("200 empty list — not null", func(t *testing.T) {
		// The handler must return [] not null/nil in JSON.
		// Many JavaScript clients break when they receive null instead of [].
		mock := &mockMonitorSvc{}
		mock.listMonitorsFn = func(_ context.Context, _ uint, _, _ int) ([]domain.Monitor, error) {
			return nil, nil // simulate DB returning empty slice as nil
		}

		h := handler.NewMonitorHandler(mock)
		r := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
		r = r.WithContext(middleware.WithUserID(r.Context(), 1))
		w := httptest.NewRecorder()

		h.List(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		// Envelope data field should be [] not null.
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
			t.Fatalf("decode: %v\nbody: %s", err, w.Body.String())
		}
		// env.Data will be nil if JSON sent null, or [] if sent [].
		// We want a non-nil empty slice (serialised as []).
		body := w.Body.String()
		if strings.Contains(body, `"data":null`) {
			t.Errorf("body %q: data should be [] not null", body)
		}
	})

	t.Run("401 no auth", func(t *testing.T) {
		h := handler.NewMonitorHandler(&mockMonitorSvc{})
		r := httptest.NewRequest(http.MethodGet, "/api/monitors", nil)
		// No middleware.WithUserID → UserIDFromCtx returns (0, false) → 401.
		w := httptest.NewRecorder()

		h.List(w, r)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCreate_serviceError — service returning ErrConflict → 409
// ─────────────────────────────────────────────────────────────────────────────
//
// Verifies that the handler's error-mapping (Err helper) correctly converts
// domain sentinel errors to the right HTTP status codes.
// This exercises a codepath that can't be reached through validation alone.
func TestCreate_serviceError(t *testing.T) {
	mock := &mockMonitorSvc{}
	mock.createMonitorFn = func(_ context.Context, _ uint, _, _ string, _, _ int) (*domain.Monitor, error) {
		// Simulate the repo returning a conflict (e.g. duplicate URL for user).
		return nil, domain.ErrConflict
	}

	h := handler.NewMonitorHandler(mock)
	body := `{"url":"https://example.com","name":"Dup","interval_seconds":60,"timeout_seconds":10}`
	r := httptest.NewRequest(http.MethodPost, "/api/monitors", bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(middleware.WithUserID(r.Context(), 1))
	w := httptest.NewRecorder()

	h.Create(w, r)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 Conflict\nbody: %s", w.Code, w.Body.String())
	}
}
