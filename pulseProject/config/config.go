// Package config loads all application settings from environment variables.
//
// Why environment variables instead of a config file?
// The 12-factor app methodology says: "Store config in the environment."
// This lets you run the SAME binary in dev (with .env) and prod (with real secrets)
// without any code change.
//
// Python equivalent: pydantic BaseSettings / python-decouple
//   class Settings(BaseSettings):
//       port: str = "8080"
//       database_url: str = "postgres://..."
//
// Node.js equivalent: dotenv + process.env
//   const port = process.env.PORT || '8080'
package config

import (
	"os"       // os.Getenv — reads environment variables
	"strconv" // strconv.Atoi — parses string → int (env vars are always strings)
	"time"    // time.Duration, time.Second, time.Hour
)

// Config is a plain struct holding all settings the application needs.
// Having one central Config struct (instead of reading os.Getenv everywhere)
// means:
//   1. Easy to test — just pass a &Config{...} in tests, no env setup needed.
//   2. One place to see ALL configuration options.
//   3. Type-safe — values are already parsed to int, time.Duration, etc.
//
// Python analogy: dataclass or pydantic model with all settings as fields.
// Node.js analogy: a config object: { port: parseInt(process.env.PORT) || 8080 }
type Config struct {
	// ── Server ──────────────────────────────────────────────────────────────

	// Port the HTTP server listens on. Default: 8080.
	// Kubernetes / Docker expose this port to the outside world.
	Port string

	// ── Database ─────────────────────────────────────────────────────────────

	// DatabaseURL is a PostgreSQL connection string.
	// Format: postgres://user:password@host:port/dbname?sslmode=disable
	// GORM uses this to open a *sql.DB connection pool.
	DatabaseURL string

	// ── Cache ────────────────────────────────────────────────────────────────

	// RedisURL is the Redis connection string.
	// We'll use Redis to cache the "latest check result" per monitor —
	// so the API can return current status instantly without a DB query.
	RedisURL string

	// ── Auth ─────────────────────────────────────────────────────────────────

	// JWTSecret is the HMAC-SHA256 signing key for JWT tokens.
	// CRITICAL: in production this MUST be a long random string from a secret manager.
	// Never hard-code it in source code.
	JWTSecret string

	// JWTExpiry is how long a JWT token stays valid after it's issued.
	// time.Duration is just an int64 counting nanoseconds, but multiplying
	// by time.Hour gives a human-readable number of hours.
	JWTExpiry time.Duration

	// ── Checker / Worker Pool ─────────────────────────────────────────────────

	// WorkerCount is how many goroutines run HTTP checks concurrently.
	// Think of it like the size of a thread pool.
	// Python: ThreadPoolExecutor(max_workers=10)
	// Node.js: concurrency limit in p-limit or bottleneck
	WorkerCount int

	// CheckTimeout is the per-request HTTP deadline.
	// If a site doesn't respond within this duration, the check fails.
	// Passed as context.WithTimeout to each HTTP request.
	CheckTimeout time.Duration

	// ScheduleInterval is how often the scheduler wakes up and looks for
	// monitors whose next check time has passed.
	ScheduleInterval time.Duration
}

// Load reads environment variables and returns a populated *Config.
// Callers should call this ONCE at startup (in main.go) and pass the result
// down through the dependency chain.
//
// Unset env vars fall back to safe development defaults so that
// `go run ./cmd/server` just works without any setup.
func Load() *Config {
	return &Config{
		// getEnv(key, default) — returns the env var or the default string.
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://pulse:pulse@localhost:5432/pulse?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:   getEnv("JWT_SECRET", "change-me-in-production-use-a-long-random-string"),

		// time.Duration(n) converts an integer to nanoseconds-based Duration.
		// Multiplying by time.Hour / time.Second then scales it correctly.
		//
		// e.g., getIntEnv("JWT_EXPIRY_HOURS", 24) returns 24 (int)
		//       time.Duration(24) * time.Hour = 24 * 3,600,000,000,000 ns = 24h
		JWTExpiry: time.Duration(getIntEnv("JWT_EXPIRY_HOURS", 24)) * time.Hour,

		WorkerCount:      getIntEnv("WORKER_COUNT", 10),
		CheckTimeout:     time.Duration(getIntEnv("CHECK_TIMEOUT_SECONDS", 10)) * time.Second,
		ScheduleInterval: time.Duration(getIntEnv("SCHEDULE_INTERVAL_SECONDS", 30)) * time.Second,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper functions (unexported — lowercase, package-private)
//
// "Unexported" in Go = lowercase first letter = only usable within this package.
// Python equivalent: _get_env() (convention, not enforced by language)
// Node.js equivalent: no real equivalent — JS has no package-private scope
// ─────────────────────────────────────────────────────────────────────────────

// getEnv returns the value of the environment variable named by key,
// or defaultVal if the variable is not set or is empty.
//
// os.Getenv returns "" for both "unset" and "set to empty string".
// For config purposes, treating both the same is fine.
func getEnv(key, defaultVal string) string {
	// os.Getenv is Go's os.environ.get(key, "")
	if val := os.Getenv(key); val != "" {
		return val // env var is set — use it
	}
	return defaultVal // env var missing — use the safe default
}

// getIntEnv parses an env var as an integer.
// Falls back to defaultVal if the variable is unset OR if it can't be parsed.
//
// strconv.Atoi is Go's int(os.environ.get(key)) — but it returns (int, error).
// In Go you ALWAYS handle errors — there are no implicit exceptions.
func getIntEnv(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal // not set → use default
	}

	// strconv.Atoi converts string "10" → int 10.
	// It returns (int, error). We must check the error.
	// If someone sets WORKER_COUNT="banana", we fall back gracefully.
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal // invalid integer → use default
	}
	return n
}
