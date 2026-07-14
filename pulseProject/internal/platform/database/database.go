// Package database bootstraps the PostgreSQL connection for Pulse.
//
// It wraps GORM's Open() with:
//   - a configured connection pool (max open/idle conns, max lifetime)
//   - AutoMigrate so the DB schema stays in sync with domain structs on every start
//
// Only main.go calls this package. Services and repositories receive *gorm.DB
// through constructor injection — they never import this package directly.
//
// Python/SQLAlchemy: engine = create_engine(url, pool_size=10, max_overflow=5)
// Node.js/Sequelize: new Sequelize(url, { pool: { max: 10, min: 2 } })
package database

import (
	"fmt"      // fmt.Errorf — wrap errors with context
	"time"     // time.Duration — connection pool lifetimes

	"gorm.io/driver/postgres" // GORM's pgx-backed Postgres driver
	"gorm.io/gorm"            // gorm.DB, gorm.Config, gorm.Logger

	"github.com/nishantks908/pulse/internal/domain" // domain structs for AutoMigrate
)

// Connect opens a *gorm.DB backed by PostgreSQL at the given DSN.
//
// DSN format: postgres://user:pass@host:port/dbname?sslmode=disable
// (matches the DATABASE_URL env var set in config.go)
//
// The connection pool is tuned conservatively for a small service:
//   - MaxOpenConns 25   — max simultaneous DB connections (OS file descriptors)
//   - MaxIdleConns 10   — kept open between requests (avoids handshake overhead)
//   - ConnMaxLifetime 5m — recycle connections to survive DB restarts / LB reconnects
//
// Python: create_engine(dsn, pool_size=10, max_overflow=15, pool_recycle=300)
// Node.js: { pool: { max: 25, min: 10, idle: 10000 } }  (Sequelize)
func Connect(dsn string) (*gorm.DB, error) {
	// gorm.Open takes a Dialector (database-specific connector) and options.
	// postgres.Open(dsn) builds the pgx connection from the DSN string.
	// gorm.Config{} — we use defaults (logger, naming conventions).
	//
	// GORM does NOT actually connect here — it builds the config.
	// The first real query (e.g. AutoMigrate) opens the TCP connection.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		// Wrap with context so callers know WHERE the error came from.
		// fmt.Errorf("...: %w", err) preserves the original error for errors.Is().
		//
		// Python: raise RuntimeError(f"connect to postgres: {e}") from e
		// Node.js: throw new Error(`connect to postgres: ${e.message}`)
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	// sqlDB is the underlying *sql.DB from the standard library.
	// GORM wraps it but exposes it for pool configuration.
	// This is the layer that actually manages TCP connections to Postgres.
	//
	// Python/SQLAlchemy: engine.pool — same concept, pool is a property of engine
	// Node.js/pg: pool = new Pool({ max: 25 })
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB from gorm: %w", err)
	}

	// ── Connection pool settings ─────────────────────────────────────────────

	// MaxOpenConns: max number of connections that can be OPEN at once.
	// Each open connection uses one OS file descriptor.
	// PostgreSQL's default max_connections is 100 — stay well below it.
	// Setting 0 = unlimited → can exhaust Postgres under burst traffic.
	sqlDB.SetMaxOpenConns(25)

	// MaxIdleConns: connections kept in pool EVEN WHEN IDLE.
	// An idle connection costs ~4KB of server RAM but saves the TCP+TLS
	// handshake on the next request (~2-5ms).
	// Rule of thumb: idle ≤ open/2
	sqlDB.SetMaxIdleConns(10)

	// ConnMaxLifetime: force-recycle connections older than this.
	// Prevents stale connections that survive Postgres restarts or
	// load-balancer TCP resets from silently failing.
	//
	// 5 minutes is idiomatic for most services.
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// AutoMigrate creates or updates DB tables to match the domain structs.
//
// GORM compares the struct fields + tags against the live schema and issues
// ALTER TABLE / CREATE TABLE as needed. It never drops columns (safe to run
// on a live DB with existing data).
//
// Call once at startup — before the HTTP server starts accepting requests.
//
// Python/Alembic:   alembic upgrade head  (migration files)
// Python/GORM:      Base.metadata.create_all(engine)  (auto — no migration files)
// Node.js/Sequelize: sequelize.sync({ alter: true })
// Go/GORM:           db.AutoMigrate(...)  — same idea as sequelize.sync
//
// Why AutoMigrate and not SQL migration files?
// Migration files are better for production (auditable, reversible).
// AutoMigrate is great for development: no ceremony, instant schema sync.
// In a real product you'd generate SQL from GORM diffs and check them into git.
func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&domain.User{},     // users table
		&domain.Monitor{},  // monitors table (FK → users)
		&domain.Check{},    // checks table   (FK → monitors)
		&domain.Incident{}, // incidents table (FK → monitors)
	)
	if err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
