package service_test

// monitor_service_test.go — Stage 12.
//
// Tests the service layer WITHOUT a real DB, network, or Redis.
// Every external dependency is replaced with a stub that satisfies the same
// interface the service depends on.
//
// HOW INTERFACES MAKE THIS POSSIBLE:
//   service.NewMonitorService accepts repository.MonitorRepository (an interface),
//   NOT *gorm.DB directly. Any Go value whose type has the right methods satisfies
//   the interface automatically — no "implements" declaration, no registration.
//   That's Go's structural typing ("duck typing" with compile-time checking).
//
//   stubMonitorRepo has all 8 required methods → it IS a MonitorRepository.
//   NewMonitorService accepts it → test passes with zero DB infrastructure.
//   Tests run in microseconds: no Postgres, no Redis, no internet required.
//
// WHY -race MATTERS HERE:
//   RecordOutcome launches nothing concurrent by itself, but in production it's
//   called from a goroutine that reads the outcomes channel.
//   The -race flag instruments memory reads/writes at runtime. If two goroutines
//   touch the same variable without synchronisation → race report → test fails.
//   Running with -race catches races that -count=1 tests would miss.
//
// TEST PATTERNS USED:
//   Table-driven tests  — one Test function, N cases in a slice (idiomatic Go).
//   Stub vs mock        — stubs return canned values; our stubs also record args.
//   Transactor stub     — calls fn(ctx) so the closure's two writes actually run.
//
// Python: unittest.mock.MagicMock() or a class satisfying a Protocol.
// Node.js: a plain object { createMonitor: jest.fn() } passed to the constructor.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog" // zerolog.Nop() — discards all log output in tests

	"github.com/nishantks908/pulse/internal/domain"     // sentinel errors + Monitor type
	"github.com/nishantks908/pulse/internal/monitor"    // monitor.Outcome, monitor.Job
	pnotifier "github.com/nishantks908/pulse/internal/notifier" // notifier.Notifier interface
	"github.com/nishantks908/pulse/internal/repository" // repository.TxFn (for Transactor stub)
	"github.com/nishantks908/pulse/internal/service"    // the package under test
)

// ─────────────────────────────────────────────────────────────────────────────
// Stubs — test doubles satisfying the repository / notifier interfaces
// ─────────────────────────────────────────────────────────────────────────────
//
// STUB DESIGN PATTERN:
//   Each method checks whether its function field is set.
//   If set  → call the function (test controls the behaviour).
//   If nil  → return the zero value (safe default, no panic).
//
// This means each test only sets the function fields it cares about.
// Unrelated methods silently succeed — no boilerplate for unused methods.
//
// Python: class StubMonitorRepo: def create(self, m): return None  (empty impl)
// Node.js: const stub = { create: jest.fn().mockResolvedValue(null) }

// ── MonitorRepository stub ───────────────────────────────────────────────────

type stubMonitorRepo struct {
	createFn       func(ctx context.Context, m *domain.Monitor) error
	byIDFn         func(ctx context.Context, id uint) (*domain.Monitor, error)
	listDueFn      func(ctx context.Context, now time.Time) ([]domain.Monitor, error)
	listByUserFn   func(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error)
	updateStatusFn func(ctx context.Context, id uint, active bool) error
	updateNextFn   func(ctx context.Context, id uint, next time.Time) error
	deleteFn       func(ctx context.Context, id uint) error
	setStatusFn    func(ctx context.Context, id uint, status string) error
}

func (s *stubMonitorRepo) Create(ctx context.Context, m *domain.Monitor) error {
	if s.createFn != nil {
		return s.createFn(ctx, m)
	}
	return nil
}
func (s *stubMonitorRepo) ByID(ctx context.Context, id uint) (*domain.Monitor, error) {
	if s.byIDFn != nil {
		return s.byIDFn(ctx, id)
	}
	// Default: return a minimal monitor so notifier doesn't crash.
	return &domain.Monitor{ID: id, Name: "stub", URL: "https://stub.example.com"}, nil
}
func (s *stubMonitorRepo) ListDue(ctx context.Context, now time.Time) ([]domain.Monitor, error) {
	if s.listDueFn != nil {
		return s.listDueFn(ctx, now)
	}
	return nil, nil
}
func (s *stubMonitorRepo) ListByUser(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error) {
	if s.listByUserFn != nil {
		return s.listByUserFn(ctx, userID, page, pageSize)
	}
	return nil, nil
}
func (s *stubMonitorRepo) UpdateStatus(ctx context.Context, id uint, active bool) error {
	if s.updateStatusFn != nil {
		return s.updateStatusFn(ctx, id, active)
	}
	return nil
}
func (s *stubMonitorRepo) UpdateNextCheck(ctx context.Context, id uint, next time.Time) error {
	if s.updateNextFn != nil {
		return s.updateNextFn(ctx, id, next)
	}
	return nil
}
func (s *stubMonitorRepo) Delete(ctx context.Context, id uint) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}
func (s *stubMonitorRepo) SetStatus(ctx context.Context, id uint, status string) error {
	if s.setStatusFn != nil {
		return s.setStatusFn(ctx, id, status)
	}
	return nil
}

// ── CheckRepository stub ─────────────────────────────────────────────────────

type stubCheckRepo struct {
	saveFn            func(ctx context.Context, c *domain.Check) error
	historyFn         func(ctx context.Context, monitorID uint, limit, offset int) ([]domain.Check, error)
	latestByMonitorFn func(ctx context.Context, monitorID uint) (*domain.Check, error)
}

func (s *stubCheckRepo) Save(ctx context.Context, c *domain.Check) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, c)
	}
	return nil
}
func (s *stubCheckRepo) History(ctx context.Context, monitorID uint, limit, offset int) ([]domain.Check, error) {
	if s.historyFn != nil {
		return s.historyFn(ctx, monitorID, limit, offset)
	}
	return nil, nil
}
func (s *stubCheckRepo) LatestByMonitor(ctx context.Context, monitorID uint) (*domain.Check, error) {
	if s.latestByMonitorFn != nil {
		return s.latestByMonitorFn(ctx, monitorID)
	}
	return nil, domain.ErrNotFound
}

// ── IncidentRepository stub ──────────────────────────────────────────────────

type stubIncidentRepo struct {
	createFn        func(ctx context.Context, i *domain.Incident) error
	openByMonitorFn func(ctx context.Context, monitorID uint) (*domain.Incident, error)
	resolveFn       func(ctx context.Context, id uint, resolvedAt time.Time) error
	listByMonitorFn func(ctx context.Context, monitorID uint, limit int) ([]domain.Incident, error)
}

func (s *stubIncidentRepo) Create(ctx context.Context, i *domain.Incident) error {
	if s.createFn != nil {
		return s.createFn(ctx, i)
	}
	return nil
}
func (s *stubIncidentRepo) OpenByMonitor(ctx context.Context, monitorID uint) (*domain.Incident, error) {
	if s.openByMonitorFn != nil {
		return s.openByMonitorFn(ctx, monitorID)
	}
	// Default: no open incident (site was up or never checked).
	return nil, domain.ErrNotFound
}
func (s *stubIncidentRepo) Resolve(ctx context.Context, id uint, resolvedAt time.Time) error {
	if s.resolveFn != nil {
		return s.resolveFn(ctx, id, resolvedAt)
	}
	return nil
}
func (s *stubIncidentRepo) ListByMonitor(ctx context.Context, monitorID uint, limit int) ([]domain.Incident, error) {
	if s.listByMonitorFn != nil {
		return s.listByMonitorFn(ctx, monitorID, limit)
	}
	return nil, nil
}

// ── Transactor stub ──────────────────────────────────────────────────────────

// stubTransactor calls fn(ctx) directly — no real DB transaction.
//
// WHY call fn instead of returning nil immediately?
//   The service's RecordOutcome does:
//     s.transactor.RunInTx(ctx, func(ctx) error {
//         s.incidents.Create(ctx, incident)    ← side-effect A
//         s.monitors.SetStatus(ctx, id, "down") ← side-effect B
//         return nil
//     })
//
//   If the stub returned nil WITHOUT calling fn, side-effects A and B would
//   never happen. Tests couldn't verify that Create or SetStatus were called.
//   Calling fn(ctx) executes the closure — stubs observe the calls.
//
// Python: the with-block body still runs whether you mock the context manager or not.
// Node.js: the callback passed to knex.transaction() would not run if you mocked wrong.
type stubTransactor struct{}

func (stubTransactor) RunInTx(ctx context.Context, fn repository.TxFn) error {
	// Execute the transaction function — no BEGIN/COMMIT/ROLLBACK needed in tests.
	return fn(ctx)
}

// ── Notifier stub ────────────────────────────────────────────────────────────

// stubNotifier records whether Notify was called and with what arguments.
// Unlike stubs that just return values, this one also OBSERVES calls — making
// it closer to a "mock" in the mock-vs-stub terminology.
type stubNotifier struct {
	called        bool
	lastMonitor   domain.Monitor
	lastDownSince time.Time
}

func (s *stubNotifier) Notify(_ context.Context, m domain.Monitor, downSince time.Time) error {
	s.called = true
	s.lastMonitor = m
	s.lastDownSince = downSince
	return nil
}

// ── compile-time interface checks ────────────────────────────────────────────
// These blank assignments fail at compile time if a stub drifts from the interface.
// "If the interface changes, every stub breaks immediately — not at test runtime."
//
// Python: no direct equivalent (checked at runtime via Protocol).
// Go: static guarantee, zero runtime cost.
var (
	_ repository.MonitorRepository  = (*stubMonitorRepo)(nil)
	_ repository.CheckRepository    = (*stubCheckRepo)(nil)
	_ repository.IncidentRepository = (*stubIncidentRepo)(nil)
	_ repository.Transactor         = stubTransactor{}
	_ pnotifier.Notifier            = (*stubNotifier)(nil)
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helper — build a service wired with controllable stubs
// ─────────────────────────────────────────────────────────────────────────────

// deps groups all stubs so tests can configure specific behaviours.
type deps struct {
	monitors  *stubMonitorRepo
	checks    *stubCheckRepo
	incidents *stubIncidentRepo
	notifier  *stubNotifier
}

// newTestSvc returns a MonitorService backed entirely by stubs.
// Pass the returned deps to configure specific stub behaviours per test.
func newTestSvc(t *testing.T) (service.MonitorService, *deps) {
	t.Helper()
	d := &deps{
		monitors:  &stubMonitorRepo{},
		checks:    &stubCheckRepo{},
		incidents: &stubIncidentRepo{},
		notifier:  &stubNotifier{},
	}
	svc := service.NewMonitorService(
		d.monitors,
		d.checks,
		d.incidents,
		nil,            // no Redis cache — all cache guards check `if s.cache != nil`
		stubTransactor{}, // executes TxFn directly (no real DB tx needed)
		d.notifier,
		zerolog.Nop(), // discard all log output — no noise during go test
	)
	return svc, d
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCreateMonitor — table-driven validation + happy path
// ─────────────────────────────────────────────────────────────────────────────

// TestCreateMonitor verifies that business-rule validation in the service layer
// rejects invalid inputs BEFORE calling the repository.
//
// TABLE-DRIVEN PATTERN:
//   Define all cases up-front in a slice, then range over them.
//   Each case runs as its own sub-test via t.Run — failures are isolated.
//   Adding a new edge case = add one struct literal to the table.
//
//   Python: @pytest.mark.parametrize("url,interval,want_err", [("", 60, ErrValidation), ...])
//   Node.js: test.each([...])("create monitor: %s", (url, interval, wantErr) => {...})
//   Go:      tests := []struct{...}{{ ... }, { ... }}; for _, tt := range tests { t.Run(tt.name, ...) }
func TestCreateMonitor(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		monName   string
		interval  int
		timeout   int
		wantErrIs error // nil means expect success; non-nil = expected sentinel
	}{
		{
			name:      "happy path",
			url:       "https://example.com",
			monName:   "My Site",
			interval:  60,
			timeout:   10,
			wantErrIs: nil,
		},
		{
			name:      "empty URL",
			url:       "",
			monName:   "My Site",
			interval:  60,
			timeout:   10,
			wantErrIs: domain.ErrValidation,
		},
		{
			name:      "whitespace-only URL",
			url:       "   ",
			monName:   "My Site",
			interval:  60,
			timeout:   10,
			wantErrIs: domain.ErrValidation,
		},
		{
			name:      "empty name",
			url:       "https://example.com",
			monName:   "",
			interval:  60,
			timeout:   10,
			wantErrIs: domain.ErrValidation,
		},
		{
			name:      "interval too small (below 5s minimum)",
			url:       "https://example.com",
			monName:   "My Site",
			interval:  4,
			timeout:   10,
			wantErrIs: domain.ErrValidation,
		},
		{
			name:      "timeout zero",
			url:       "https://example.com",
			monName:   "My Site",
			interval:  60,
			timeout:   0,
			wantErrIs: domain.ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestSvc(t)

			got, err := svc.CreateMonitor(context.Background(), 1,
				tt.url, tt.monName, tt.interval, tt.timeout)

			if tt.wantErrIs != nil {
				// errors.Is traverses the error chain — works even through fmt.Errorf wrapping.
				// The service wraps domain.ErrValidation: fmt.Errorf("create monitor: %w: ...", domain.ErrValidation)
				// errors.Is(wrappedErr, domain.ErrValidation) → true, because %w preserves the chain.
				//
				// Python: pytest.raises(ValidationError)
				// Node.js: expect(err).toBeInstanceOf(ValidationError)
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("want error wrapping %v, got: %v", tt.wantErrIs, err)
				}
				if got != nil {
					t.Error("expected nil monitor on validation error")
				}
				return
			}

			// Happy path: no error, monitor returned with correct fields.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected non-nil *domain.Monitor")
			}
			if got.URL != tt.url {
				t.Errorf("monitor.URL = %q, want %q", got.URL, tt.url)
			}
			if got.IntervalSeconds != tt.interval {
				t.Errorf("monitor.IntervalSeconds = %d, want %d", got.IntervalSeconds, tt.interval)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRecordOutcome_UPtoDOWN — first failing check opens incident in a transaction
// ─────────────────────────────────────────────────────────────────────────────

// TestRecordOutcome_UPtoDOWN verifies the UP→DOWN transition:
//   1. No open incident (ErrNotFound from OpenByMonitor).
//   2. stubTransactor.RunInTx calls fn(ctx) → incident created + status set "down".
//   3. Notifier called AFTER the transaction.
//
// This test answers: "why does the transaction + notifier ordering matter?"
// The stub lets us verify both WHAT happened and WHEN.
func TestRecordOutcome_UPtoDOWN(t *testing.T) {
	const monitorID = uint(7)
	svc, d := newTestSvc(t)

	// ── Configure stubs ──────────────────────────────────────────────────────

	// No open incident → first failing check → UP→DOWN transition.
	d.incidents.openByMonitorFn = func(_ context.Context, id uint) (*domain.Incident, error) {
		return nil, domain.ErrNotFound // "no open incident"
	}

	// Capture the incident that was created inside the transaction.
	var createdIncident *domain.Incident
	d.incidents.createFn = func(_ context.Context, i *domain.Incident) error {
		createdIncident = i
		return nil
	}

	// Capture the status that was set inside the transaction.
	var statusSet string
	d.monitors.setStatusFn = func(_ context.Context, _ uint, status string) error {
		statusSet = status
		return nil
	}

	// Return a real monitor for the notifier fetch (after tx commits).
	d.monitors.byIDFn = func(_ context.Context, id uint) (*domain.Monitor, error) {
		return &domain.Monitor{
			ID:   id,
			Name: "Test Site",
			URL:  "https://example.com",
		}, nil
	}

	// ── Act ──────────────────────────────────────────────────────────────────

	// Build a failing Outcome — Up defaults to false (zero value of bool).
	// Zero Result = {StatusCode:0, LatencyMs:0, Up:false, Err:nil}
	// which represents "request failed, no HTTP response received."
	o := monitor.Outcome{
		Job: monitor.Job{
			MonitorID:       monitorID,
			URL:             "https://example.com",
			IntervalSeconds: 60,
		},
		// Result.Up = false (zero value) → site is DOWN
	}

	if err := svc.RecordOutcome(context.Background(), o); err != nil {
		t.Fatalf("RecordOutcome returned unexpected error: %v", err)
	}

	// ── Assert ───────────────────────────────────────────────────────────────

	// Verify incident was created with correct MonitorID and a non-zero StartedAt.
	if createdIncident == nil {
		t.Fatal("expected incidents.Create to be called, but it was not")
	}
	if createdIncident.MonitorID != monitorID {
		t.Errorf("incident.MonitorID = %d, want %d", createdIncident.MonitorID, monitorID)
	}
	if createdIncident.StartedAt.IsZero() {
		t.Error("incident.StartedAt is zero — should be set to time.Now() at check time")
	}

	// Verify the transaction set the monitor status to "down".
	if statusSet != "down" {
		t.Errorf("monitors.SetStatus called with %q, want %q", statusSet, "down")
	}

	// Verify the notifier was called (after the transaction, not inside it).
	if !d.notifier.called {
		t.Fatal("expected notifier.Notify to be called after the transaction committed")
	}
	if d.notifier.lastMonitor.ID != monitorID {
		t.Errorf("notifier called with monitor ID %d, want %d",
			d.notifier.lastMonitor.ID, monitorID)
	}
	if d.notifier.lastDownSince.IsZero() {
		t.Error("notifier received zero downSince timestamp")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRecordOutcome_DOWNtoUP — existing incident resolved in transaction
// ─────────────────────────────────────────────────────────────────────────────

// TestRecordOutcome_DOWNtoUP verifies the DOWN→UP recovery path:
//   1. An open incident exists (site was down).
//   2. Outcome.Up = true → site is back up.
//   3. stubTransactor calls fn(ctx) → incident resolved + status set "up".
//   4. Notifier should NOT be called (only alerts on going down, not recovery).
func TestRecordOutcome_DOWNtoUP(t *testing.T) {
	const monitorID = uint(3)
	svc, d := newTestSvc(t)

	// An open incident exists — site was DOWN, now coming back UP.
	openIncident := &domain.Incident{
		ID:        42,
		MonitorID: monitorID,
		StartedAt: time.Now().Add(-5 * time.Minute),
	}
	d.incidents.openByMonitorFn = func(_ context.Context, id uint) (*domain.Incident, error) {
		return openIncident, nil
	}

	// Capture that Resolve was called with the correct incident ID.
	var resolvedID uint
	d.incidents.resolveFn = func(_ context.Context, id uint, _ time.Time) error {
		resolvedID = id
		return nil
	}

	// Capture the status set.
	var statusSet string
	d.monitors.setStatusFn = func(_ context.Context, _ uint, status string) error {
		statusSet = status
		return nil
	}

	// Successful outcome — site is back UP.
	o := monitor.Outcome{
		Job: monitor.Job{MonitorID: monitorID, URL: "https://example.com", IntervalSeconds: 60},
	}
	o.Result.Up = true
	o.Result.StatusCode = 200

	if err := svc.RecordOutcome(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Incident should be resolved with the correct ID.
	if resolvedID != openIncident.ID {
		t.Errorf("incidents.Resolve called with ID %d, want %d", resolvedID, openIncident.ID)
	}

	// Status should be set to "up".
	if statusSet != "up" {
		t.Errorf("monitors.SetStatus called with %q, want %q", statusSet, "up")
	}

	// Notifier should NOT be called on recovery.
	if d.notifier.called {
		t.Error("notifier.Notify should NOT be called on DOWN→UP recovery")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRecordOutcome_alreadyDown — no new incident when monitor was already down
// ─────────────────────────────────────────────────────────────────────────────

// TestRecordOutcome_alreadyDown ensures the service doesn't create duplicate
// incidents when consecutive failing checks arrive for the same monitor.
// "One outage = one incident" is a business rule enforced here.
func TestRecordOutcome_alreadyDown(t *testing.T) {
	const monitorID = uint(5)
	svc, d := newTestSvc(t)

	// An open incident already exists → monitor is STILL down (not a new outage).
	d.incidents.openByMonitorFn = func(_ context.Context, id uint) (*domain.Incident, error) {
		return &domain.Incident{ID: 1, MonitorID: id}, nil
	}

	// If Create is called, the test should fail — no duplicate incidents allowed.
	d.incidents.createFn = func(_ context.Context, i *domain.Incident) error {
		t.Error("incidents.Create should NOT be called when monitor is already down")
		return nil
	}

	o := monitor.Outcome{
		Job: monitor.Job{MonitorID: monitorID, URL: "https://example.com", IntervalSeconds: 60},
	}
	// Result.Up = false (zero value) → still DOWN.

	if err := svc.RecordOutcome(context.Background(), o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.notifier.called {
		t.Error("notifier should NOT be called when monitor is already down")
	}
}
