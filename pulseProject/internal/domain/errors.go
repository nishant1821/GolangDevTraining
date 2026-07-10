// Package domain is the heart of Pulse.
// It contains ONLY pure Go types and errors — no HTTP, no DB, no framework.
//
// Clean-Architecture rule: this package imports NOTHING from outside the
// standard library. Every other package depends on domain, never the reverse.
//
// Python analogy: a models.py that has no Django/SQLAlchemy import — just
//   class NotFoundError(Exception): pass
// Node.js analogy: a plain JS class with no express/sequelize require().
package domain

import "errors" // the ONLY import — standard library

// ─────────────────────────────────────────────────────────────────────────────
// Sentinel Errors
//
// A "sentinel" is a special value you compare against — like io.EOF.
// We define them here (domain layer) so every other layer imports from ONE place.
//
// Why not just return errors.New("not found") inline everywhere?
//   → Because errors.New creates a NEW error value each time it's called.
//     errors.Is() compares by identity, so inline errors would NEVER match.
//
// Python: class ErrNotFound(Exception): pass  — then `except ErrNotFound`
// Node.js: class NotFoundError extends Error {}  — then `catch(e) { if (e instanceof NotFoundError) }`
// Go: errors.Is(err, domain.ErrNotFound)  — works even through wrapped errors
// ─────────────────────────────────────────────────────────────────────────────

var (
	// ErrNotFound: the requested resource does not exist in storage.
	// HTTP layer will map this → 404 Not Found.
	// Example: user asks for monitor ID 99, but ID 99 doesn't exist.
	ErrNotFound = errors.New("not found")

	// ErrValidation: input data broke a business rule.
	// HTTP layer will map this → 422 Unprocessable Entity.
	// Example: monitor URL is blank, or interval_seconds < 1.
	ErrValidation = errors.New("validation failed")

	// ErrConflict: the operation would create a duplicate or invalid state.
	// HTTP layer will map this → 409 Conflict.
	// Example: user tries to register "https://google.com" but already monitors it.
	ErrConflict = errors.New("conflict")

	// ErrUnauthorized: the caller is not allowed to perform this action.
	// HTTP layer will map this → 401 Unauthorized.
	// Example: JWT token is missing, expired, or doesn't own the monitor.
	ErrUnauthorized = errors.New("unauthorized")
)
