package repository

// check_repo.go — CheckRepository interface + GORM implementation.
//
// Checks are append-only (never updated after creation).
// Every HTTP probe produces one Check row — over time a monitor accumulates
// thousands of rows. Queries here are always filtered by monitorID + LIMIT.

import (
	"context" // context.Context
	"fmt"     // fmt.Errorf

	"gorm.io/gorm" // *gorm.DB

	"github.com/nishantks908/pulse/internal/domain" // domain.Check
)

// ─────────────────────────────────────────────────────────────────────────────
// CheckRepository — the interface
// ─────────────────────────────────────────────────────────────────────────────

// CheckRepository defines DB operations for Check rows.
// The result handler (Stage 6) will call Save after every probe.
// The API handler will call History when a user requests check logs.
type CheckRepository interface {
	// Save inserts a new check row.
	// Check rows are never updated — this is the only write operation.
	//
	// Python: session.add(check); session.commit()
	// Node.js: await Check.create(data)
	Save(ctx context.Context, c *domain.Check) error

	// History returns a page of checks for a monitor, ordered newest-first.
	// offset = (page-1)*limit   e.g., page=2, limit=20 → offset=20
	//
	// SQL: SELECT * FROM checks WHERE monitor_id=$1
	//        ORDER BY created_at DESC LIMIT $2 OFFSET $3
	History(ctx context.Context, monitorID uint, limit, offset int) ([]domain.Check, error)

	// LatestByMonitor returns the single most recent check for a monitor.
	// Used to determine current up/down status without loading all history.
	// Returns domain.ErrNotFound if the monitor has never been checked.
	//
	// SQL: SELECT * FROM checks WHERE monitor_id=$1 ORDER BY created_at DESC LIMIT 1
	LatestByMonitor(ctx context.Context, monitorID uint) (*domain.Check, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// gormCheckRepo — the GORM implementation
// ─────────────────────────────────────────────────────────────────────────────

type gormCheckRepo struct {
	db *gorm.DB
}

// NewCheckRepository constructs a CheckRepository backed by *gorm.DB.
func NewCheckRepository(db *gorm.DB) CheckRepository {
	return &gormCheckRepo{db: db}
}

// Save inserts a new check row.
// GORM fills c.ID and c.CreatedAt from the DB response.
//
// We never update checks — the table is effectively an append-only log.
// This is intentional: historical probe data must be immutable for audit trails.
func (r *gormCheckRepo) Save(ctx context.Context, c *domain.Check) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("save check for monitor %d: %w", c.MonitorID, translateError(err))
	}
	return nil
}

// History returns the last `limit` checks for a monitor, newest first.
// Order("created_at DESC") ensures we get the most recent results.
//
// Python: session.query(Check).filter_by(monitor_id=mid).order_by(Check.created_at.desc()).limit(n).all()
// Node.js: Check.findAll({ where: { monitorId }, order: [['createdAt','DESC']], limit: n })
func (r *gormCheckRepo) History(ctx context.Context, monitorID uint, limit, offset int) ([]domain.Check, error) {
	var checks []domain.Check
	err := r.db.WithContext(ctx).
		Where("monitor_id = ?", monitorID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&checks).Error
	if err != nil {
		return nil, fmt.Errorf("history for monitor %d: %w", monitorID, translateError(err))
	}
	return checks, nil
}

// LatestByMonitor fetches the single most recent check.
// First() + Order DESC gives us the latest row without scanning the whole table.
// Returns domain.ErrNotFound (via translateError) if no checks exist yet.
func (r *gormCheckRepo) LatestByMonitor(ctx context.Context, monitorID uint) (*domain.Check, error) {
	var c domain.Check
	err := r.db.WithContext(ctx).
		Where("monitor_id = ?", monitorID).
		Order("created_at DESC").
		First(&c).Error
	if err != nil {
		return nil, fmt.Errorf("latest check for monitor %d: %w", monitorID, translateError(err))
	}
	return &c, nil
}
