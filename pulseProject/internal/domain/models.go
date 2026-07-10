package domain

// models.go defines the four core domain types used throughout Pulse.
// These are plain Go structs — no external imports, just the standard "time" package.
//
// GORM tags (gorm:"...") tell the ORM how to map the struct to a DB column.
// JSON tags  (json:"...") control what the field looks like in API responses.
// Both live on the SAME struct — this is idiomatic Go (no separate DTO layer
// for a service this size).
//
// Python/Django:  class Monitor(models.Model): url = models.URLField()
// Python/SQLAlch: class Monitor(Base): url = Column(String)
// Node.js/Prisma: model Monitor { url String }
// Go/GORM:        type Monitor struct { URL string `gorm:"not null"` }

import "time" // ONLY import — keeps domain free of external packages

// ─────────────────────────────────────────────────────────────────────────────
// Monitor — a website/endpoint the user wants to watch
// ─────────────────────────────────────────────────────────────────────────────

// Monitor is the central entity. A user registers a URL, sets a check interval,
// and Pulse pings it on that schedule.
type Monitor struct {
	// ── Primary Key & Timestamps ────────────────────────────────────────────
	//
	// We define these manually instead of embedding gorm.Model so that
	// domain/models.go stays free of the gorm import.
	// (gorm.Model would give us the same fields but import "gorm.io/gorm")

	// ID: auto-incrementing primary key.
	// gorm:"primarykey" → GORM treats this as PK.
	// json:"id" → serialised as lowercase "id" in JSON (REST convention).
	ID uint `gorm:"primarykey" json:"id"`

	// CreatedAt / UpdatedAt: GORM sets these automatically when you
	// create or update a record (it detects the field names by convention).
	// No tag needed for the auto-behaviour; json tags control the API output.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// DeletedAt *time.Time — soft delete.
	// Instead of running DELETE FROM monitors, GORM sets this timestamp.
	// Subsequent queries automatically add WHERE deleted_at IS NULL.
	// Using *time.Time (pointer) means NULL in the DB when not deleted.
	// json:"-" → NEVER include this field in JSON responses (clients don't
	//            need to know about internal soft-delete mechanics).
	//
	// Python/Django: class Meta: soft_delete = True  (via django-safedelete)
	// Node.js/Sequelize: paranoid: true in model options
	DeletedAt *time.Time `gorm:"index" json:"-"`

	// ── Ownership ────────────────────────────────────────────────────────────

	// UserID is the foreign key — which User owns this Monitor.
	// gorm:"not null;index" → DB-level NOT NULL constraint + index for fast lookup.
	// json:"user_id" → clients see "user_id" in API responses.
	UserID uint `gorm:"not null;index" json:"user_id"`

	// ── Core Fields ──────────────────────────────────────────────────────────

	// URL is the full endpoint to probe. e.g., "https://example.com/api/v1/ping"
	// gorm:"not null;size:2048" → DB column VARCHAR(2048), cannot be null.
	URL string `gorm:"not null;size:2048" json:"url"`

	// Name is a human-readable label set by the user. e.g., "Production API"
	Name string `gorm:"not null;size:255" json:"name"`

	// IntervalSeconds: how many seconds between checks.
	// Default 60 means "ping every minute".
	// gorm:"default:60" → DB default value used when inserting without this field.
	IntervalSeconds int `gorm:"not null;default:60" json:"interval_seconds"`

	// TimeoutSeconds: how long (seconds) to wait for a response before
	// declaring the check a failure. Default 10 → 10-second HTTP timeout.
	TimeoutSeconds int `gorm:"not null;default:10" json:"timeout_seconds"`

	// Active: if false the scheduler skips this monitor.
	// Useful for pausing a monitor without deleting it.
	Active bool `gorm:"not null;default:true" json:"active"`

	// ── Relationships ────────────────────────────────────────────────────────
	//
	// GORM can auto-populate these when you call db.Preload("User") or
	// db.Preload("Checks"). json:"-" hides them from API responses by default
	// (the handler layer decides when to include related data via DTOs).

	// User is the owner of this monitor (belongs-to).
	User User `gorm:"foreignKey:UserID" json:"-"`

	// Checks are the historical probe results for this monitor (has-many).
	Checks []Check `gorm:"foreignKey:MonitorID" json:"-"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Check — a single HTTP probe result
// ─────────────────────────────────────────────────────────────────────────────

// Check is one data-point from a single HTTP ping.
// One Monitor accumulates thousands of Checks over its lifetime.
// Relationship: Monitor (1) → Checks (N)
type Check struct {
	// ID & CreatedAt only — checks are append-only, they are never updated.
	// json tag "checked_at" is more descriptive than "created_at" for this type.
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"checked_at"` // renamed via json tag for clarity

	// MonitorID links this check to its parent monitor.
	// index makes queries like "give me last 100 checks for monitor #5" fast.
	MonitorID uint `gorm:"not null;index" json:"monitor_id"`

	// StatusCode is the HTTP response status (200, 404, 503 …).
	// 0 means the request never got a response (network error, timeout).
	StatusCode int `gorm:"not null" json:"status_code"`

	// ResponseTimeMs is the round-trip time in milliseconds.
	// We store int64 because time.Duration is int64 nanoseconds;
	// dividing by time.Millisecond gives us a clean integer.
	ResponseTimeMs int64 `gorm:"not null" json:"response_time_ms"`

	// Up is the summary: true = this check passed (2xx within timeout).
	// Denormalised from StatusCode for fast "is it up?" queries.
	Up bool `gorm:"not null" json:"up"`

	// Error holds the failure reason when Up==false.
	// e.g., "context deadline exceeded", "connection refused".
	// json:"error,omitempty" → omit this key entirely from JSON when empty string.
	// "omitempty" is like Python's `if value: result["error"] = value`.
	Error string `gorm:"size:1024" json:"error,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// User — an account that owns monitors
// ─────────────────────────────────────────────────────────────────────────────

// User represents a Pulse account. A user registers, logs in (JWT), and
// creates monitors. All monitors belong to exactly one user.
type User struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"-"` // soft-delete

	// Email is the login identifier. uniqueIndex creates a UNIQUE constraint.
	// json:"email" → visible in API responses (e.g., GET /me).
	Email string `gorm:"not null;uniqueIndex;size:255" json:"email"`

	// Password stores the BCRYPT HASH, NEVER the plain-text password.
	// json:"-" → this field is COMPLETELY INVISIBLE in every JSON response.
	// If you forget json:"-" on a password field, you have a security bug.
	//
	// Python: password = make_password(raw_password)  # Django
	// Node.js: bcrypt.hash(password, 10)
	// Go: bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	Password string `gorm:"not null;size:255" json:"-"`

	// Monitors owned by this user (has-many).
	Monitors []Monitor `gorm:"foreignKey:UserID" json:"-"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Incident — an outage window
// ─────────────────────────────────────────────────────────────────────────────

// Incident is opened when a monitor flips from Up → Down.
// It is resolved (ResolvedAt is set) when the monitor flips back Down → Up.
// This lets us track "how long was the site down?" and notify users.
type Incident struct {
	// Incidents are never updated after creation (except ResolvedAt).
	// No soft-delete here — incident history should be permanent.
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	// MonitorID links this incident to the affected monitor.
	MonitorID uint `gorm:"not null;index" json:"monitor_id"`

	// StartedAt is the timestamp of the FIRST failing check that opened this incident.
	// It may differ slightly from CreatedAt if the DB write was delayed.
	StartedAt time.Time `gorm:"not null" json:"started_at"`

	// ResolvedAt is nil/NULL while the incident is still ongoing.
	//
	// *time.Time (pointer to time) → maps to SQL NULL when the pointer is nil.
	// A nil pointer in Go is like None in Python or null in JavaScript/SQL.
	//
	// Python/Django: resolved_at = models.DateTimeField(null=True, blank=True)
	// Node.js/Sequelize: resolvedAt: { type: DataTypes.DATE, allowNull: true }
	// Go/GORM: *time.Time with no extra tag — GORM handles the NULL automatically.
	//
	// json:"resolved_at,omitempty" → key is absent from JSON while nil (ongoing incident).
	// Once resolved it becomes: "resolved_at": "2025-07-09T12:34:56Z"
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	// Monitor is the parent record (belongs-to). Populated via db.Preload("Monitor").
	Monitor Monitor `gorm:"foreignKey:MonitorID" json:"-"`
}
