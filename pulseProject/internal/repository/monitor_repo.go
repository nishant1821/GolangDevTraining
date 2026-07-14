package repository

// monitor_repo.go — MonitorRepository interface + GORM implementation.
//
// INTERFACE LIVES HERE, NOT IN SERVICE — design note:
//   The Go proverb is "accept interfaces, return structs."
//   Ideally an interface lives with its CONSUMER (service layer), so the
//   service defines exactly the methods it needs and doesn't care about the rest.
//   We define it here (alongside the implementation) because the service layer
//   doesn't exist yet. When Stage 6 adds the service, the interface can move.
//   Either location is valid; the key is that *gorm.DB never appears in service/.
//
// Python analogy:
//   class MonitorRepository(Protocol):    # interface
//       def create(self, m: Monitor) -> None: ...
//   class GormMonitorRepository:          # implementation
//       def create(self, m: Monitor) -> None: db.session.add(m); db.session.commit()
//
// Node.js analogy:
//   interface MonitorRepository { create(m: Monitor): Promise<void> }
//   class SequelizeMonitorRepository implements MonitorRepository { ... }

import (
	"context" // context.Context — every DB call is cancellable
	"fmt"     // fmt.Errorf — wrap errors with context
	"time"    // time.Time — for ListDue(now)

	"gorm.io/gorm" // *gorm.DB — used ONLY inside this package, never exposed

	"github.com/nishantks908/pulse/internal/domain" // domain.Monitor, domain.ErrNotFound
)

// ─────────────────────────────────────────────────────────────────────────────
// MonitorRepository — the interface (contract)
// ─────────────────────────────────────────────────────────────────────────────

// MonitorRepository defines every DB operation the rest of the app can do
// on Monitor rows. The service layer holds THIS interface, not *gorm.DB.
//
// Benefit: in tests you inject a fakeMonitorRepo that returns hardcoded data —
// no Postgres container needed to test business logic.
//
// Python Protocol / ABC analogy:
//   class MonitorRepository(Protocol):
//       def create(self, m: Monitor) -> None: ...
//       def by_id(self, id: int) -> Monitor: ...
//       ...
type MonitorRepository interface {
	// Create inserts a new monitor row.
	// The database fills in ID, CreatedAt, UpdatedAt.
	// m is passed as pointer so GORM can write back the generated ID.
	//
	// Python: session.add(monitor); session.commit()
	// Node.js: await Monitor.create(data)
	Create(ctx context.Context, m *domain.Monitor) error

	// ByID fetches a single monitor by primary key.
	// Returns domain.ErrNotFound if no row exists (translated from gorm.ErrRecordNotFound).
	//
	// Python: session.get(Monitor, id)  →  raise NotFoundError if None
	// Node.js: await Monitor.findByPk(id)  →  throw NotFoundError if null
	ByID(ctx context.Context, id uint) (*domain.Monitor, error)

	// ListDue returns active monitors whose NextCheckAt is at or before `now`.
	// The scheduler calls this on every tick:
	//   due := func() []domain.Monitor { ms, _ := repo.ListDue(ctx, time.Now()); return ms }
	//
	// SQL: SELECT * FROM monitors WHERE active=true AND next_check_at <= $1 AND deleted_at IS NULL
	ListDue(ctx context.Context, now time.Time) ([]domain.Monitor, error)

	// ListByUser returns a paginated slice of monitors owned by userID.
	// page is 1-indexed. pageSize is the number of rows per page.
	// Used by GET /monitors (handler layer, Stage 6).
	//
	// SQL: SELECT * FROM monitors WHERE user_id=$1 LIMIT $2 OFFSET $3
	ListByUser(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error)

	// UpdateStatus sets the active flag on a monitor.
	// Used by PATCH /monitors/:id/pause and /resume endpoints.
	//
	// SQL: UPDATE monitors SET active=$1, updated_at=NOW() WHERE id=$2
	UpdateStatus(ctx context.Context, id uint, active bool) error

	// UpdateNextCheck sets NextCheckAt after a probe completes.
	// Called by the result handler: next = time.Now().Add(interval).
	//
	// SQL: UPDATE monitors SET next_check_at=$1, updated_at=NOW() WHERE id=$2
	UpdateNextCheck(ctx context.Context, id uint, next time.Time) error

	// Delete soft-deletes a monitor (sets deleted_at = NOW()).
	// GORM handles the WHERE clause automatically for future queries.
	//
	// SQL: UPDATE monitors SET deleted_at=NOW() WHERE id=$1
	Delete(ctx context.Context, id uint) error
}

// ─────────────────────────────────────────────────────────────────────────────
// gormMonitorRepo — the GORM implementation
// ─────────────────────────────────────────────────────────────────────────────

// gormMonitorRepo satisfies MonitorRepository using a *gorm.DB.
// Unexported (lowercase g) — callers get a MonitorRepository interface, never
// a concrete *gormMonitorRepo. This enforces the boundary.
//
// Python: class _GormMonitorRepository(MonitorRepository): ...  (single underscore = private)
// Node.js: no enforced access control — convention only
type gormMonitorRepo struct {
	db *gorm.DB // unexported — nobody outside this package touches it
}

// NewMonitorRepository is the constructor.
// Returns the INTERFACE, not the concrete struct.
// This is the "return structs, accept interfaces" half of the Go proverb —
// we return the interface so callers are automatically decoupled.
//
// Python: def new_monitor_repository(db) -> MonitorRepository: return _GormMonitorRepository(db)
// Node.js: export function newMonitorRepository(db): MonitorRepository { return new GormMonitorRepository(db) }
func NewMonitorRepository(db *gorm.DB) MonitorRepository {
	return &gormMonitorRepo{db: db}
}

// ── Method implementations ────────────────────────────────────────────────────

// Create inserts m into the monitors table.
// GORM fills m.ID, m.CreatedAt, m.UpdatedAt from the DB response.
//
// db.WithContext(ctx) ties the SQL query to the request context:
//   - if ctx has a deadline, the query is cancelled when it expires
//   - if ctx is cancelled (SIGTERM), the query is aborted mid-flight
// ALWAYS use WithContext — never call db.Create without it.
//
// Python: session.add(m); session.flush()  ← flush to get the ID back
// Node.js: const row = await Monitor.create(data)  ← row.id is set after
func (r *gormMonitorRepo) Create(ctx context.Context, m *domain.Monitor) error {
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("monitor create: %w", translateError(err))
	}
	return nil
}

// ByID fetches a monitor by primary key.
// First() finds the first row matching the condition and fills &m.
// If no row exists, GORM returns gorm.ErrRecordNotFound — translateError
// converts it to domain.ErrNotFound.
//
// Python: m = session.get(Monitor, id); if m is None: raise NotFoundError
// Node.js: const m = await Monitor.findByPk(id); if (!m) throw new NotFoundError()
func (r *gormMonitorRepo) ByID(ctx context.Context, id uint) (*domain.Monitor, error) {
	var m domain.Monitor
	// First(&m, id) = SELECT * FROM monitors WHERE id=? AND deleted_at IS NULL LIMIT 1
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, fmt.Errorf("monitor by id %d: %w", id, translateError(err))
	}
	return &m, nil
}

// ListDue returns monitors due for a check at or before `now`.
// WHERE active=true filters out paused monitors.
// GORM automatically adds WHERE deleted_at IS NULL for soft-deleted rows.
//
// Python: session.query(Monitor).filter(Monitor.active==True, Monitor.next_check_at<=now).all()
// Node.js: Monitor.findAll({ where: { active: true, nextCheckAt: { [Op.lte]: now } } })
func (r *gormMonitorRepo) ListDue(ctx context.Context, now time.Time) ([]domain.Monitor, error) {
	var monitors []domain.Monitor
	err := r.db.WithContext(ctx).
		Where("active = ? AND next_check_at <= ?", true, now).
		Find(&monitors).Error
	if err != nil {
		return nil, fmt.Errorf("list due monitors: %w", translateError(err))
	}
	return monitors, nil
}

// ListByUser returns a paginated slice of monitors owned by userID.
// OFFSET = (page - 1) * pageSize  →  page 1 starts at row 0.
//
// Python: session.query(Monitor).filter_by(user_id=uid).limit(size).offset((page-1)*size).all()
// Node.js: Monitor.findAll({ where: { userId }, limit: size, offset: (page-1)*size })
func (r *gormMonitorRepo) ListByUser(ctx context.Context, userID uint, page, pageSize int) ([]domain.Monitor, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	var monitors []domain.Monitor
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Limit(pageSize).
		Offset(offset).
		Find(&monitors).Error
	if err != nil {
		return nil, fmt.Errorf("list monitors for user %d: %w", userID, translateError(err))
	}
	return monitors, nil
}

// UpdateStatus sets active=true/false on a monitor.
// Model(&domain.Monitor{}).Where(...) targets the UPDATE without loading the row first.
// Updates() only updates the columns you specify (not all columns).
//
// GOTCHA: Never use Save() to update a single field — it writes ALL fields,
// including zero-values, potentially overwriting data you didn't intend to change.
// Use Updates(map) or Updates(struct with non-zero fields) for partial updates.
//
// Python: session.query(Monitor).filter_by(id=id).update({"active": active})
// Node.js: await Monitor.update({ active }, { where: { id } })
func (r *gormMonitorRepo) UpdateStatus(ctx context.Context, id uint, active bool) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Monitor{}).
		Where("id = ?", id).
		Updates(map[string]any{"active": active})
	if result.Error != nil {
		return fmt.Errorf("update monitor %d status: %w", id, translateError(result.Error))
	}
	if result.RowsAffected == 0 {
		// No rows updated → monitor doesn't exist.
		// RowsAffected check is needed because GORM doesn't return
		// ErrRecordNotFound for UPDATE queries — only for SELECT queries.
		return fmt.Errorf("update monitor %d status: %w", id, domain.ErrNotFound)
	}
	return nil
}

// UpdateNextCheck sets next_check_at after a probe completes.
// Called by the result handler in Stage 6.
func (r *gormMonitorRepo) UpdateNextCheck(ctx context.Context, id uint, next time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Monitor{}).
		Where("id = ?", id).
		Updates(map[string]any{"next_check_at": next})
	if result.Error != nil {
		return fmt.Errorf("update monitor %d next_check: %w", id, translateError(result.Error))
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update monitor %d next_check: %w", id, domain.ErrNotFound)
	}
	return nil
}

// Delete soft-deletes a monitor by setting deleted_at.
// GORM detects the DeletedAt field on domain.Monitor and issues an UPDATE
// instead of a DELETE statement.
//
// Python/django-safedelete: monitor.delete()  → sets deleted_at, doesn't DELETE
// Node.js/Sequelize paranoid: monitor.destroy()  → sets deletedAt, doesn't DELETE
func (r *gormMonitorRepo) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&domain.Monitor{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete monitor %d: %w", id, translateError(result.Error))
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("delete monitor %d: %w", id, domain.ErrNotFound)
	}
	return nil
}
