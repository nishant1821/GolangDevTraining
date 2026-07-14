package service

// monitor_service.go — Stage 6.
//
// MonitorService is the BUSINESS LOGIC layer.
//
// It knows:
//   - domain rules: what a valid monitor looks like, what an incident is
//   - repository interfaces: how to read/write monitors, checks, incidents
//   - monitor.Outcome: what the pool produces after a check
//
// It knows NOTHING about:
//   - HTTP (no http.Request, no http.ResponseWriter, no status codes)
//   - SQL (*gorm.DB, SQL strings, GORM methods)
//   - How the check was performed (HTTPChecker detail — it just sees the result)
//
// WHY must service NOT import handler?
//   handler depends on service (handler calls service methods).
//   If service also imports handler → CIRCULAR IMPORT → Go compiler error.
//   Go forbids circular imports by design — it enforces the dependency arrow
//   always points downward: handler → service → repository → domain.
//
// WHY must service NOT import gorm?
//   If service uses *gorm.DB directly:
//     1. Can't test without a real Postgres DB running.
//     2. Can never swap the DB layer (e.g., move to raw SQL, SQLite, etc.)
//     3. GORM error types leak into business logic.
//   By depending only on repository INTERFACES, the service doesn't care
//   what's behind them — GORM, fake, raw SQL, all look the same.
//
// DEPENDENCY INJECTION BY HAND:
//   Go has no DI framework (no Spring, no FastAPI Depends).
//   You create dependencies explicitly and pass them into constructors.
//   "By hand" means: main.go creates everything and wires it.
//     repo   := repository.NewMonitorRepository(db)
//     svc    := service.NewMonitorService(repo, ...)
//   No magic, no annotations, no container — just function calls.
//
// Python analogy: a service.py class that __init__ accepts a repository.
// Node.js analogy: a class constructor that receives injected dependencies.

import (
	"context"       // context.Context — every method is cancellable
	"encoding/json" // json.Marshal / json.Unmarshal — cache serialisation
	"errors"        // errors.Is — check error chain
	"fmt"           // fmt.Errorf, fmt.Sprintf — wrap errors + build cache keys
	"strings"       // strings.TrimSpace — input sanitisation
	"time"          // time.Now, time.Duration — timestamps + intervals

	"github.com/rs/zerolog"                                  // zerolog.Logger — structured logging

	"github.com/nishantks908/pulse/internal/domain"          // domain types + sentinel errors
	"github.com/nishantks908/pulse/internal/monitor"         // monitor.Outcome — result from pool
	"github.com/nishantks908/pulse/internal/notifier"        // notifier.Notifier — alerting interface
	"github.com/nishantks908/pulse/internal/platform/cache"  // cache.Cache — Redis wrapper
	"github.com/nishantks908/pulse/internal/repository"      // repository interfaces (NOT *gorm.DB)
)

// ─────────────────────────────────────────────────────────────────────────────
// MonitorService — the struct (dependencies injected via constructor)
// ─────────────────────────────────────────────────────────────────────────────

// monitorService holds all dependencies as INTERFACES — not concrete types.
// Lowercase = unexported. Callers use NewMonitorService() and work with
// the returned concrete type directly (Stage 7 will add a service interface
// when handlers need it for mocking).
//
// Python:
//   class MonitorService:
//       def __init__(self, monitors, checks, incidents):
//           self.monitors = monitors
//           self.checks   = checks
//           self.incidents = incidents
//
// Node.js:
//   class MonitorService {
//     constructor(monitors, checks, incidents) { ... }
//   }
// monitorCacheTTL is how long a monitor's JSON lives in Redis.
// Short TTL (30s) means stale data is corrected quickly; long enough to absorb
// repeated GET /api/monitors/{id} calls within one page load.
//
// Python: MONITOR_CACHE_TTL = timedelta(seconds=30)
// Node.js: const MONITOR_CACHE_TTL = 30_000  // ms
const monitorCacheTTL = 30 * time.Second

type monitorService struct {
	monitors   repository.MonitorRepository  // read/write monitors table
	checks     repository.CheckRepository    // write checks table (append-only log)
	incidents  repository.IncidentRepository // open/close incidents
	cache      *cache.Cache                  // optional Redis cache; nil = no caching
	transactor repository.Transactor         // runs two DB writes inside one transaction (Stage 9)
	notifier   notifier.Notifier             // sends alert after UP→DOWN transaction commits (Stage 9)
	log        zerolog.Logger                // structured logger — non-fatal warnings in RecordOutcome
}

// MonitorService is the public interface that handler depends on.
// Defining the interface HERE (in the service package, where it is implemented)
// is idiomatic Go — callers use the interface, not the struct.
//
// Stage 7 handlers depend on this interface, not the concrete *monitorService,
// so you can pass a mock implementation in handler tests without touching the DB.
//
// Python: Abstract base class (ABC) with @abstractmethod — similar concept.
// Node.js/TypeScript: interface MonitorService { createMonitor(...): Promise<Monitor> }
type MonitorService interface {
	RecordOutcome(ctx context.Context, o monitor.Outcome) error
	CreateMonitor(ctx context.Context, userID uint, url, name string, intervalSecs, timeoutSecs int) (*domain.Monitor, error)
	GetMonitor(ctx context.Context, id, userID uint) (*domain.Monitor, error)
	ListMonitors(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error)
	PauseMonitor(ctx context.Context, id, userID uint) error
	ResumeMonitor(ctx context.Context, id, userID uint) error
	DeleteMonitor(ctx context.Context, id, userID uint) error
	// GetCheckHistory returns a paginated list of probe results for one monitor.
	// Enforces ownership — users can only read checks for their own monitors.
	GetCheckHistory(ctx context.Context, monitorID, userID uint, page, pageSize int) ([]domain.Check, error)
}

// NewMonitorService is the constructor — wired in main.go.
// Returns the MonitorService interface so callers don't depend on the concrete struct.
//
// Python: def new_monitor_service(monitors, checks, incidents) -> MonitorService
// Node.js: export function newMonitorService(monitors, checks, incidents): MonitorService
// NewMonitorService wires up the monitor service with its dependencies.
//
// c is the Redis cache. Pass nil to run without caching — the service falls
// back to DB-only reads. This lets main.go degrade gracefully when Redis is
// unavailable instead of crashing the whole binary.
//
// Python: MonitorService(monitors, checks, incidents, cache=None)
// Node.js: new MonitorService(monitors, checks, incidents, cache ?? null)
func NewMonitorService(
	monitors repository.MonitorRepository,
	checks repository.CheckRepository,
	incidents repository.IncidentRepository,
	c *cache.Cache,
	tx repository.Transactor,
	n notifier.Notifier,
	log zerolog.Logger,
) MonitorService {
	return &monitorService{
		monitors:   monitors,
		checks:     checks,
		incidents:  incidents,
		cache:      c,   // nil is fine — all cache methods guard with `if s.cache == nil`
		transactor: tx,  // runs incident+status writes atomically (Stage 9)
		notifier:   n,   // logs/emails/Slacks after transaction commits
		log:        log, // structured logger for non-fatal warnings
	}
}

// monitorCacheKey returns the Redis key for a single monitor.
// Namespaced with "pulse:" so keys from different services don't collide
// if they share a Redis instance.
//
// Python: f"pulse:monitor:{id}"
// Node.js: `pulse:monitor:${id}`
func monitorCacheKey(id uint) string {
	return fmt.Sprintf("pulse:monitor:%d", id)
}

// ─────────────────────────────────────────────────────────────────────────────
// RecordOutcome — called after every HTTP probe
// ─────────────────────────────────────────────────────────────────────────────

// RecordOutcome processes one Outcome from the worker pool.
// It is the "result handler" goroutine in main.go calls it in a loop:
//
//	for o := range outcomes {
//	    svc.RecordOutcome(ctx, o)
//	}
//
// Three things happen in order:
//  1. Save the Check row (the probe result as a permanent log entry).
//  2. Advance the monitor's NextCheckAt (so the scheduler knows when to check again).
//  3. Open or close an Incident (Up→Down opens one, Down→Up resolves one).
//
// Business rules live HERE, not in the repository:
//   - "Don't open a second incident if one is already open."
//   - "Only resolve an incident if one exists."
//   - "NextCheckAt = now + interval."
// Repository methods are simple CRUD — they don't know these rules.
func (s *monitorService) RecordOutcome(ctx context.Context, o monitor.Outcome) error {
	// ── Step 1: Save the Check row ────────────────────────────────────────────
	// Build a domain.Check from the Outcome and persist it.
	// This is the permanent record: "at time T, monitor M had status S."
	//
	// Checks are append-only — we never update them.
	c := &domain.Check{
		MonitorID:      o.Job.MonitorID,
		StatusCode:     o.Result.StatusCode,
		ResponseTimeMs: o.Result.LatencyMs,
		Up:             o.Result.Up,
	}
	if o.Result.Err != nil {
		// Err is a Go error interface — convert to string for DB storage.
		// We store the message so the API can show "connection refused" to the user.
		c.Error = o.Result.Err.Error()
	}
	if err := s.checks.Save(ctx, c); err != nil {
		return fmt.Errorf("record outcome: save check: %w", err)
	}

	// ── Step 2: Advance NextCheckAt ───────────────────────────────────────────
	// Tell the scheduler when to check this monitor next.
	// next = NOW + interval (e.g., now + 60 seconds)
	//
	// If IntervalSeconds is 0 (shouldn't happen, but defensive):
	// default to 60 seconds so we don't spam the scheduler.
	interval := o.Job.IntervalSeconds
	if interval <= 0 {
		interval = 60
	}
	next := time.Now().Add(time.Duration(interval) * time.Second)
	if err := s.monitors.UpdateNextCheck(ctx, o.Job.MonitorID, next); err != nil {
		// Non-fatal — the check row was saved. Log and continue.
		// If UpdateNextCheck fails, the scheduler will re-check on the next tick
		// (NextCheckAt is still the old value, so it will be "due" again soon).
		// Returning here would skip incident logic, which is worse.
		//
		// s.log.Warn() → zerolog structured warning.
		// .Err(err) adds the "error" JSON field.
		// .Uint("monitor_id", ...) adds a typed field — no string formatting needed.
		// .Msg() is the terminal call that flushes the event.
		//
		// Python: logger.warning("record outcome: update next check", extra={"monitor_id": ..., "error": ...})
		// Node.js/pino: log.warn({ monitorId: ..., err }, "record outcome: update next check")
		s.log.Warn().
			Err(err).
			Uint("monitor_id", o.Job.MonitorID).
			Msg("record outcome: update next check failed (non-fatal)")
	}

	// ── Step 3: Incident management (Stage 9 — transactions + notifications) ──
	//
	// Business rules:
	//   Up = false → site DOWN  → open an Incident (if none open)
	//   Up = true  → site UP    → resolve open Incident (if one exists)
	//
	// WHY two operations must be atomic:
	//   UP→DOWN: incident INSERT + status UPDATE must both succeed or both fail.
	//   If incident creates but status stays "up": API shows "up" while incident row
	//   says "down" → inconsistent data confuses on-call.
	//   If status is set "down" but incident fails: alerts would look for an incident
	//   that doesn't exist.
	//   TRANSACTION → either both land or neither does.
	//
	// WHY notify AFTER the transaction:
	//   Inside the transaction, the commit hasn't happened yet.
	//   If notify fires before commit and then the transaction rolls back,
	//   the engineer gets paged for an outage that was never recorded.
	//   "notify after commit" = "if we alerted, the data exists."
	//
	// GORM rollback trigger:
	//   s.transactor.RunInTx(ctx, func(ctx) error { ... return err })
	//   Returning a non-nil error from the callback → GORM calls ROLLBACK.
	//   Returning nil → GORM calls COMMIT.
	//
	// Python:
	//   with session.begin():
	//       session.add(incident)
	//       monitor.status = "down"
	//   send_alert(monitor)  # after commit
	//
	// Node.js:
	//   await knex.transaction(async trx => {
	//       await trx('incidents').insert(incident)
	//       await trx('monitors').where({ id }).update({ status: 'down' })
	//   })
	//   await sendAlert(monitor)  // after transaction resolved

	if !o.Result.Up {
		// ── Site is DOWN ──────────────────────────────────────────────────────
		_, err := s.incidents.OpenByMonitor(ctx, o.Job.MonitorID)
		switch {
		case err == nil:
			// An open incident already exists → monitor is still down.
			// Nothing to do — don't open a second incident for the same outage.

		case errors.Is(err, domain.ErrNotFound):
			// ── UP→DOWN transition: first failing check ────────────────────────
			// Run two writes atomically:
			//   1. INSERT into incidents (new outage record)
			//   2. UPDATE monitors SET status='down'
			// If either fails, GORM rolls back both.
			//
			// downSince is captured outside the transaction and passed into it.
			// We also pass it to Notify() after commit so the alert shows the
			// exact time the outage started — same value stored in Incident.StartedAt.
			downSince := time.Now()
			if txErr := s.transactor.RunInTx(ctx, func(ctx context.Context) error {
				incident := &domain.Incident{
					MonitorID: o.Job.MonitorID,
					StartedAt: downSince,
				}
				// Return non-nil → GORM rolls back everything in this function.
				if err := s.incidents.Create(ctx, incident); err != nil {
					return err
				}
				// Return non-nil here → incident INSERT is also rolled back.
				return s.monitors.SetStatus(ctx, o.Job.MonitorID, "down")
			}); txErr != nil {
				return fmt.Errorf("record outcome: open incident transaction: %w", txErr)
			}

			// Transaction committed — notify AFTER so the incident row is durable.
			// Notification failure is non-fatal: we don't want a failed Slack/email
			// call to mark the whole outcome as failed and stop the pipeline.
			// In production you'd log + alert-on-alert-failure separately.
			//
			// We fetch the monitor here (for name + URL) because the Job only
			// carries MonitorID, URL, interval — not the full struct.
			if m, fetchErr := s.monitors.ByID(ctx, o.Job.MonitorID); fetchErr == nil {
				_ = s.notifier.Notify(ctx, *m, downSince)
			}

		default:
			return fmt.Errorf("record outcome: check open incident: %w", err)
		}

	} else {
		// ── Site is UP ────────────────────────────────────────────────────────
		incident, err := s.incidents.OpenByMonitor(ctx, o.Job.MonitorID)
		switch {
		case err == nil:
			// ── DOWN→UP transition: site recovered ────────────────────────────
			// Run two writes atomically:
			//   1. UPDATE incidents SET resolved_at=NOW() WHERE id=?
			//   2. UPDATE monitors SET status='up'
			// If either fails, both roll back — incident stays open, status unchanged.
			if txErr := s.transactor.RunInTx(ctx, func(ctx context.Context) error {
				if err := s.incidents.Resolve(ctx, incident.ID, time.Now()); err != nil {
					return err
				}
				return s.monitors.SetStatus(ctx, o.Job.MonitorID, "up")
			}); txErr != nil {
				return fmt.Errorf("record outcome: resolve incident transaction: %w", txErr)
			}

		case errors.Is(err, domain.ErrNotFound):
			// No open incident — monitor was already up or this is its first check.
			// Update status to "up" to move it from "unknown" on first success.
			// Single write → no transaction needed.
			// Non-fatal: if this fails, status stays "unknown" until the next check.
			if setErr := s.monitors.SetStatus(ctx, o.Job.MonitorID, "up"); setErr != nil {
				s.log.Warn().
					Err(setErr).
					Uint("monitor_id", o.Job.MonitorID).
					Msg("record outcome: set status up failed (non-fatal)")
			}

		default:
			return fmt.Errorf("record outcome: check open incident: %w", err)
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Monitor CRUD — business logic methods
// ─────────────────────────────────────────────────────────────────────────────

// CreateMonitor validates inputs and inserts a new monitor for a user.
//
// Business rules enforced here (NOT in repository, NOT in handler):
//   - URL must not be empty.
//   - Name must not be empty.
//   - IntervalSeconds must be >= 5 (avoid hammering sites).
//   - TimeoutSeconds must be >= 1.
//   - NextCheckAt = time.Time{} (zero) → scheduler picks it up on first tick.
//
// Python: def create_monitor(self, user_id, url, name, interval, timeout) -> Monitor
// Node.js: async createMonitor(userId, url, name, interval, timeout): Monitor
func (s *monitorService) CreateMonitor(
	ctx context.Context,
	userID uint,
	url, name string,
	intervalSecs, timeoutSecs int,
) (*domain.Monitor, error) {
	// ── Validate ──────────────────────────────────────────────────────────────
	// TrimSpace: "  " (spaces only) should fail, not be stored as a URL.
	url = strings.TrimSpace(url)
	name = strings.TrimSpace(name)

	if url == "" {
		return nil, fmt.Errorf("create monitor: %w: url is required", domain.ErrValidation)
	}
	if name == "" {
		return nil, fmt.Errorf("create monitor: %w: name is required", domain.ErrValidation)
	}
	if intervalSecs < 5 {
		return nil, fmt.Errorf("create monitor: %w: interval_seconds must be >= 5", domain.ErrValidation)
	}
	if timeoutSecs < 1 {
		return nil, fmt.Errorf("create monitor: %w: timeout_seconds must be >= 1", domain.ErrValidation)
	}

	m := &domain.Monitor{
		UserID:          userID,
		URL:             url,
		Name:            name,
		IntervalSeconds: intervalSecs,
		TimeoutSeconds:  timeoutSecs,
		Active:          true,
		// NextCheckAt zero → scheduler treats it as "due immediately"
	}

	if err := s.monitors.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("create monitor: %w", err)
	}
	return m, nil
}

// GetMonitor fetches a monitor by ID, enforcing ownership.
// Uses cache-aside: check Redis first, fall back to DB on a miss.
//
// CACHE-ASIDE FLOW:
//
//   Hit (fast path):
//     Redis GET "pulse:monitor:42" → "{...json...}"
//     json.Unmarshal → *domain.Monitor
//     ownership check
//     return (DB never touched)
//
//   Miss (slow path):
//     Redis GET → cache.ErrCacheMiss
//     DB: monitors.ByID(ctx, 42)
//     json.Marshal monitor
//     Redis SET "pulse:monitor:42" json TTL=30s
//     ownership check
//     return
//
//   Redis error (fail-open):
//     Redis GET → real network error (not just a miss)
//     log the error, proceed as if miss → go to DB
//     WHY fail-open? Cache is an optimisation, not a source of truth.
//       Returning an error to the user because Redis hiccupped would be wrong.
//
// WHY ownership check AFTER cache lookup?
//   We cache by monitor ID, not by (id, userID) pair.
//   A cached monitor could belong to a different user if IDs were reused,
//   but since IDs are DB primary keys and never reused (auto-increment),
//   ownership check is still safe to do after retrieval.
//   Caching per-user would multiply cache entries for shared resources.
func (s *monitorService) GetMonitor(ctx context.Context, id, userID uint) (*domain.Monitor, error) {
	// ── Cache lookup ──────────────────────────────────────────────────────────
	if s.cache != nil {
		if m, ok := s.cacheGetMonitor(ctx, id); ok {
			// Cache hit — just do ownership check, no DB.
			if m.UserID != userID {
				return nil, fmt.Errorf("get monitor: %w", domain.ErrNotFound)
			}
			return m, nil
		}
		// Miss or Redis error — fall through to DB (fail-open).
	}

	// ── DB fetch ──────────────────────────────────────────────────────────────
	m, err := s.monitors.ByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get monitor: %w", err)
	}

	// Ownership check — same sentinel as "not found" so callers can't tell
	// whether the monitor exists but belongs to someone else.
	if m.UserID != userID {
		return nil, fmt.Errorf("get monitor: %w", domain.ErrNotFound)
	}

	// ── Populate cache ────────────────────────────────────────────────────────
	// Store AFTER ownership check so we only cache monitors the requester owns.
	// (Technically any monitor is safe to cache by ID — ownership is checked on
	// retrieval — but this keeps the intent clearer.)
	if s.cache != nil {
		s.cacheSetMonitor(ctx, m)
	}

	return m, nil
}

// cacheGetMonitor is a helper that reads a monitor from Redis and unmarshals it.
// Returns (monitor, true) on hit, (nil, false) on miss OR Redis error (fail-open).
func (s *monitorService) cacheGetMonitor(ctx context.Context, id uint) (*domain.Monitor, bool) {
	raw, err := s.cache.Get(ctx, monitorCacheKey(id))
	if err != nil {
		// Both miss and real errors → false (fall through to DB).
		// Real errors are intentionally silent here — the service layer
		// doesn't have a logger injected. main.go logs Redis startup errors.
		return nil, false
	}
	var m domain.Monitor
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, false // corrupt cache entry — treat as miss
	}
	return &m, true
}

// cacheSetMonitor marshals a monitor to JSON and stores it in Redis with TTL.
// Errors are silently swallowed — cache population is best-effort; the DB
// result is still returned to the caller even if Redis is unavailable.
func (s *monitorService) cacheSetMonitor(ctx context.Context, m *domain.Monitor) {
	b, err := json.Marshal(m)
	if err != nil {
		return // struct should always marshal; defensive guard
	}
	_ = s.cache.Set(ctx, monitorCacheKey(m.ID), string(b), monitorCacheTTL)
}

// cacheDelMonitor removes a monitor's cached entry.
// Called on any write that changes the monitor (pause, resume, delete).
// Uses context.Background() so the invalidation isn't bound to the
// request context — if the request's context is already cancelled, we still
// want to invalidate to avoid stale cache.
func (s *monitorService) cacheDelMonitor(id uint) {
	if s.cache == nil {
		return
	}
	_ = s.cache.Del(context.Background(), monitorCacheKey(id))
}

// ListMonitors returns paginated monitors for a user.
func (s *monitorService) ListMonitors(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error) {
	if pageSize <= 0 {
		pageSize = 20 // sensible default
	}
	monitors, err := s.monitors.ListByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list monitors: %w", err)
	}
	return monitors, nil
}

// PauseMonitor sets active=false so the scheduler skips it.
// Enforces ownership — only the owner can pause.
// Invalidates the cache so the next GET reflects the paused state.
func (s *monitorService) PauseMonitor(ctx context.Context, id, userID uint) error {
	if _, err := s.GetMonitor(ctx, id, userID); err != nil {
		return fmt.Errorf("pause monitor: %w", err)
	}
	if err := s.monitors.UpdateStatus(ctx, id, false); err != nil {
		return fmt.Errorf("pause monitor: %w", err)
	}
	// Invalidate AFTER successful DB write.
	// If we invalidated before the write and the write failed,
	// the next GET would repopulate the cache with the old (wrong) value,
	// then the write error returns — user sees "success" but DB has old data.
	// Invalidating after the write is always safe.
	s.cacheDelMonitor(id)
	return nil
}

// ResumeMonitor sets active=true and resets NextCheckAt to now so it's
// picked up on the very next scheduler tick.
// Invalidates the cache so the next GET reflects the resumed state.
func (s *monitorService) ResumeMonitor(ctx context.Context, id, userID uint) error {
	if _, err := s.GetMonitor(ctx, id, userID); err != nil {
		return fmt.Errorf("resume monitor: %w", err)
	}
	if err := s.monitors.UpdateStatus(ctx, id, true); err != nil {
		return fmt.Errorf("resume monitor: %w", err)
	}
	if err := s.monitors.UpdateNextCheck(ctx, id, time.Time{}); err != nil {
		return fmt.Errorf("resume monitor: reset next check: %w", err)
	}
	s.cacheDelMonitor(id) // invalidate after all DB writes succeed
	return nil
}

// DeleteMonitor soft-deletes a monitor (sets deleted_at).
// Enforces ownership. Invalidates the cache.
func (s *monitorService) DeleteMonitor(ctx context.Context, id, userID uint) error {
	if _, err := s.GetMonitor(ctx, id, userID); err != nil {
		return fmt.Errorf("delete monitor: %w", err)
	}
	if err := s.monitors.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete monitor: %w", err)
	}
	s.cacheDelMonitor(id) // remove from cache so next GET returns 404
	return nil
}

// GetCheckHistory returns a paginated list of probe results for one monitor.
//
// Ownership is enforced first — GetMonitor returns ErrNotFound if the monitor
// belongs to a different user, so the caller can never read another user's checks.
//
// Pagination:
//   page=1, pageSize=20 → offset=0,  limit=20  (first 20 rows)
//   page=2, pageSize=20 → offset=20, limit=20  (next 20 rows)
//
// SQL produced by the repo:
//   SELECT * FROM checks WHERE monitor_id=$1
//     ORDER BY created_at DESC LIMIT $2 OFFSET $3
//
// Python: checks = Check.query.filter_by(monitor_id=id).order_by(...).offset(...).limit(...).all()
// Node.js: Check.findAll({ where: {monitorId}, order:[['createdAt','DESC']], limit, offset })
func (s *monitorService) GetCheckHistory(
	ctx context.Context,
	monitorID, userID uint,
	page, pageSize int,
) ([]domain.Check, error) {
	// Ownership check — same sentinel error as GetMonitor (ErrNotFound), so
	// callers can't tell whether the monitor exists but belongs to someone else.
	if _, err := s.GetMonitor(ctx, monitorID, userID); err != nil {
		return nil, fmt.Errorf("get check history: %w", err)
	}

	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	checks, err := s.checks.History(ctx, monitorID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("get check history: %w", err)
	}
	return checks, nil
}
