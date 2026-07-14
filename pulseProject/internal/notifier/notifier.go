package notifier

// notifier.go — Stage 9.
//
// Notifier is the alerting interface. When a monitor flips UP→DOWN, the service
// calls Notify() to send an alert. TODAY we log it; LATER you swap in email,
// Slack, PagerDuty — the service code never changes.
//
// WHY AN INTERFACE?
//   The service layer depends on the INTERFACE, not the implementation.
//   This is the same principle used for repositories (Stage 5):
//     - In tests:      inject a FakeNotifier that records calls, never sends real alerts.
//     - In production: inject EmailNotifier, SlackNotifier, or a multi-sender.
//     - In Stage 9:    inject LogNotifier — just structured logging.
//
//   "Program to interfaces, not implementations" — GoF Design Patterns;
//   also the core of Go's duck-typed interface system.
//
// TIMING — why notify AFTER the transaction commits?
//
//   If we notified INSIDE the transaction:
//     1. BEGIN
//     2. CREATE incident        ← in tx
//     3. UPDATE status="down"   ← in tx
//     4. CALL notify (email/Slack/Pagerduty) ← side-effect
//     5. ROLLBACK ← (step 4 error, or step 2/3 error)
//
//   After rollback, the incident row was erased — but the alert was ALREADY SENT.
//   The on-call engineer would investigate a phantom outage that never persisted.
//
//   Rule: side-effects (HTTP calls, emails, queue messages) that cannot be
//   rolled back must happen AFTER the transaction commits. This guarantees:
//   "if we notified, the incident row exists; if we didn't notify, it doesn't."
//
//   Python/Django:
//     with transaction.atomic():
//         Incident.objects.create(...)
//         Monitor.objects.filter(id=id).update(status="down")
//     # transaction committed — now safe to send email
//     send_alert(monitor)
//
//   Node.js/Knex:
//     await knex.transaction(async trx => { ... })
//     // after await returns without throwing: committed
//     await sendSlackAlert(monitor)

import (
	"context" // context.Context — alerts are cancellable (deadline, HTTP ctx)
	"time"    // time.Time — downSince = when the outage started

	"github.com/rs/zerolog" // zerolog.Logger — structured logging

	"github.com/nishantks908/pulse/internal/domain" // domain.Monitor — passed to Notify
)

// ─────────────────────────────────────────────────────────────────────────────
// Notifier — the interface
// ─────────────────────────────────────────────────────────────────────────────

// Notifier sends an alert when a monitor transitions UP→DOWN.
// Implementations can be: LogNotifier, EmailNotifier, SlackNotifier, etc.
//
// The service holds a Notifier interface and calls it after the incident
// transaction commits. Adding a new channel (Slack, PagerDuty) means writing
// a new struct that satisfies this interface — zero changes to the service.
//
// Python (Protocol/ABC):
//   class Notifier(Protocol):
//       def notify(self, monitor: Monitor, down_since: datetime) -> None: ...
//
// Node.js/TypeScript:
//   interface Notifier {
//     notify(monitor: Monitor, downSince: Date): Promise<void>
//   }
type Notifier interface {
	// Notify is called once per UP→DOWN transition, after the incident
	// transaction has committed and the data is durable.
	//
	// m         — the affected monitor (name, URL, ID for the alert message)
	// downSince — the timestamp of the first failing check (= Incident.StartedAt)
	//
	// Returning an error logs the failure but does NOT affect the monitor's
	// incident state — notification failure is non-fatal.
	Notify(ctx context.Context, m domain.Monitor, downSince time.Time) error
}

// ─────────────────────────────────────────────────────────────────────────────
// LogNotifier — development/default implementation
// ─────────────────────────────────────────────────────────────────────────────

// LogNotifier implements Notifier by writing a structured zerolog error event.
// This is the simplest possible real implementation:
//   - Zero external dependencies (no SMTP, no Slack SDK).
//   - Visible in local dev without any configuration.
//   - Easy to test (check log output).
//
// In production you'd swap this for an EmailNotifier, SlackNotifier,
// or a FanOutNotifier that calls multiple notifiers in parallel.
// The service code never changes — just a different struct injected in main.go.
//
// Python: class LogNotifier: def notify(self, m, down_since): logging.error(...)
// Node.js: class LogNotifier implements Notifier { notify(m, d) { console.error(...) } }
type LogNotifier struct {
	log zerolog.Logger
}

// NewLogNotifier constructs a LogNotifier with the given zerolog.Logger.
// Called in main.go: notifier.NewLogNotifier(logger)
//
// Python: LogNotifier(logger)
// Node.js: new LogNotifier(logger)
func NewLogNotifier(log zerolog.Logger) *LogNotifier {
	return &LogNotifier{log: log}
}

// Notify logs a structured ERROR event with all fields needed to investigate the alert:
//   monitor_id   — look up the DB row
//   monitor_name — human-readable label shown in dashboards
//   url          — the endpoint that failed
//   down_since   — when the first failing check was recorded
//
// zerolog builder pattern: each .Str()/.Uint()/.Time() call adds one JSON field.
// .Msg("...") is the terminal call that flushes the event — nothing is written
// until .Msg() or .Send() is called.
//
// Python logging equivalent:
//   logger.error("🚨 ALERT: monitor is DOWN",
//       extra={"monitor_id": m.id, "url": m.url, "down_since": str(down_since)})
//
// Node.js/pino equivalent:
//   logger.error({ monitorId: m.id, url: m.url, downSince }, "🚨 ALERT: monitor is DOWN")
func (n *LogNotifier) Notify(_ context.Context, m domain.Monitor, downSince time.Time) error {
	n.log.Error().
		Uint("monitor_id", m.ID).
		Str("monitor_name", m.Name).
		Str("url", m.URL).
		Time("down_since", downSince).
		Msg("ALERT: monitor is DOWN")
	return nil // LogNotifier never fails — real notifiers may return SMTP/HTTP errors
}
