package repository

// tx.go — Stage 9.
//
// Transactor wraps db.Transaction so the service layer can run multiple repo
// operations atomically WITHOUT importing *gorm.DB.
//
// WHY A TRANSACTION HERE (the key concept for Stage 9)?
//
//   Consider the UP→DOWN transition: two separate writes must happen together:
//     1. INSERT INTO incidents (monitor_id, started_at, ...)   ← new outage record
//     2. UPDATE monitors SET status='down' WHERE id=?          ← mark monitor as down
//
//   Without a transaction, if step 1 succeeds but step 2 crashes:
//     - The incident says the monitor is down.
//     - The monitor's status field still says "up" or "unknown".
//     - Data is INCONSISTENT — the API shows "up" while an incident is open.
//
//   With a transaction:
//     - Both writes share one atomic boundary.
//     - EITHER both commit (consistent) OR neither commits (consistent).
//     - No half-written state can exist.
//
// GORM TRANSACTION MECHANICS:
//   db.Transaction(func(tx *gorm.DB) error {
//       tx.Create(...)   ← uses the transaction connection
//       tx.Update(...)   ← same transaction
//       return nil       ← COMMIT (both writes land)
//   })
//   // return err → ROLLBACK (neither write lands)
//
// HOW WE HIDE *gorm.DB FROM THE SERVICE LAYER:
//   The service must not import gorm (it would couple business logic to the DB driver).
//   Instead:
//     1. Transactor.RunInTx(ctx, fn) — the service calls this with a plain function.
//     2. RunInTx stores the transaction *gorm.DB in the context (context.WithValue).
//     3. Repo methods call txDB(ctx, r.db) to retrieve the tx DB if one is present.
//     4. The service function just calls s.incidents.Create(ctx, ...) as normal —
//        the context carries the transaction transparently.
//
// Python/SQLAlchemy:
//   with session.begin():
//       session.add(incident)
//       session.query(Monitor).filter_by(id=id).update({"status": "down"})
//   # commit on __exit__; rollback if exception
//
// Node.js/Knex:
//   await knex.transaction(async trx => {
//       await trx('incidents').insert(incident)
//       await trx('monitors').where({ id }).update({ status: 'down' })
//   })

import (
	"context" // context.Context, context.WithValue — tx travels through context
	"fmt"     // fmt.Errorf — wrap transaction errors

	"gorm.io/gorm" // *gorm.DB — transaction handle; never exposed to the service
)

// ─────────────────────────────────────────────────────────────────────────────
// txKey — context key for the transaction
// ─────────────────────────────────────────────────────────────────────────────

// txKey is an unexported zero-size struct used as the context key for storing
// the GORM transaction handle.
//
// WHY an unexported zero-size struct and not a string like "tx"?
//   context.WithValue uses interface{} equality for key matching.
//   If we used a string "tx", any other package that also stores something at
//   context key "tx" would collide — we'd accidentally read the wrong value.
//   An unexported type (only this package can refer to txKey{}) guarantees
//   the key is unique across the entire program.
//
// Python: no direct analogy — Python uses thread-local storage for sessions.
// Node.js: Symbol('tx') achieves a similar uniqueness guarantee.
type txKey struct{}

// ─────────────────────────────────────────────────────────────────────────────
// TxFn — the function type passed to RunInTx
// ─────────────────────────────────────────────────────────────────────────────

// TxFn is the function run inside a transaction.
//
// Return nil  → the calling RunInTx will COMMIT.
// Return err  → the calling RunInTx will ROLLBACK.
//
// The ctx passed to TxFn is a child context that carries the *gorm.DB
// transaction handle. Repo methods automatically detect it via txDB().
//
// Python: Callable[[Session], None]
// Node.js: (trx: Knex.Transaction) => Promise<void>
type TxFn func(ctx context.Context) error

// ─────────────────────────────────────────────────────────────────────────────
// Transactor — the interface
// ─────────────────────────────────────────────────────────────────────────────

// Transactor knows how to run a TxFn inside a database transaction.
// It is an interface so:
//   1. The service layer doesn't import *gorm.DB (business logic stays clean).
//   2. Tests can inject a no-op FakeTransactor that just calls fn(ctx)
//      without a real DB (useful for unit-testing the rollback path).
//
// Python (Protocol):
//   class Transactor(Protocol):
//       def run_in_tx(self, fn: Callable): ...
//
// Node.js/TypeScript:
//   interface Transactor { runInTx(fn: TxFn): Promise<void> }
type Transactor interface {
	// RunInTx begins a DB transaction, calls fn(txCtx), and:
	//   - COMMITs if fn returns nil.
	//   - ROLLBACKs if fn returns any error.
	// The returned error is either fn's error or a DB-level error (begin/commit/rollback).
	RunInTx(ctx context.Context, fn TxFn) error
}

// ─────────────────────────────────────────────────────────────────────────────
// gormTransactor — the GORM implementation
// ─────────────────────────────────────────────────────────────────────────────

type gormTransactor struct{ db *gorm.DB }

// NewTransactor constructs a Transactor backed by *gorm.DB.
// Called in main.go after the DB is connected:
//   tx := repository.NewTransactor(db)
//   svc := service.NewMonitorService(..., tx, notifier)
func NewTransactor(db *gorm.DB) Transactor {
	return &gormTransactor{db: db}
}

// RunInTx wraps db.Transaction to:
//   1. Inject the *gorm.DB transaction into ctx so repo methods can use it.
//   2. Commit on nil return from fn.
//   3. Rollback on non-nil return from fn (or on panic).
//
// db.Transaction does all the BEGIN/COMMIT/ROLLBACK SQL automatically.
//
// CRITICAL: repo methods MUST call txDB(ctx, r.db) (not r.db directly)
// to participate in the transaction. Methods that ignore txDB() will use
// the outer DB and run OUTSIDE the transaction — their writes won't roll back.
//
// Python/SQLAlchemy context manager:
//   with session.begin():           ← BEGIN
//       session.add(incident)       ← inside tx
//       session.execute(upd)        ← inside tx
//   # COMMIT on __exit__(None), ROLLBACK on __exit__(exc)
//
// Node.js/Knex:
//   await knex.transaction(async trx => {  ← BEGIN
//       await trx('incidents').insert(...)  ← inside tx
//       await trx('monitors').update(...)   ← inside tx
//   })                                      ← COMMIT or ROLLBACK
func (t *gormTransactor) RunInTx(ctx context.Context, fn TxFn) error {
	return t.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Store the transaction DB in the context.
		// Any function that calls txDB(txCtx, ...) will get this tx handle.
		txCtx := context.WithValue(ctx, txKey{}, tx)
		if err := fn(txCtx); err != nil {
			// Returning a non-nil error tells GORM to ROLLBACK.
			// GORM then returns this exact error to our caller.
			return fmt.Errorf("transaction: %w", err)
		}
		// Returning nil tells GORM to COMMIT.
		return nil
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// txDB — extract the transaction DB from context
// ─────────────────────────────────────────────────────────────────────────────

// txDB returns the *gorm.DB transaction stored in ctx by RunInTx.
// Falls back to `fallback` (the regular DB) if no transaction is present.
//
// Repo methods use this INSTEAD of r.db to transparently participate
// in whatever transaction the caller has set up — or not.
//
// Pattern:
//   func (r *gormIncidentRepo) Create(ctx context.Context, i *domain.Incident) error {
//       db := txDB(ctx, r.db)       ← picks tx if inside RunInTx
//       db.WithContext(ctx).Create(i)
//   }
//
// This is the "unit of work" pattern: the context carries the work boundary.
// Repo methods don't need to know whether they're inside a transaction — they
// just work correctly in both cases.
//
// Python: SQLAlchemy sessions are shared via scoped_session (thread-local).
// Node.js: Knex passes `trx` explicitly; this hides that via context.
func txDB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return fallback
}
