package repository

// incident_repo.go — IncidentRepository interface + GORM implementation.
//
// An Incident is opened when a monitor flips Up→Down and resolved when
// it flips Down→Up. The result handler (Stage 6) drives this lifecycle.
//
// Incident rows are never soft-deleted — outage history must be permanent.

import (
	"context" // context.Context
	"fmt"     // fmt.Errorf
	"time"    // time.Time — ResolvedAt

	"gorm.io/gorm" // *gorm.DB

	"github.com/nishantks908/pulse/internal/domain" // domain.Incident
)

// ─────────────────────────────────────────────────────────────────────────────
// IncidentRepository — the interface
// ─────────────────────────────────────────────────────────────────────────────

// IncidentRepository defines DB operations for Incident rows.
type IncidentRepository interface {
	// Create inserts a new (open) incident when a monitor goes down.
	// i.ResolvedAt is nil — the incident is ongoing.
	Create(ctx context.Context, i *domain.Incident) error

	// OpenByMonitor returns the currently open incident for a monitor.
	// "Open" means resolved_at IS NULL.
	// Returns domain.ErrNotFound if the monitor has no open incident (it's up).
	//
	// SQL: SELECT * FROM incidents WHERE monitor_id=$1 AND resolved_at IS NULL LIMIT 1
	OpenByMonitor(ctx context.Context, monitorID uint) (*domain.Incident, error)

	// Resolve closes an incident by setting resolved_at.
	// Called when a monitor that was down comes back up.
	//
	// SQL: UPDATE incidents SET resolved_at=$1 WHERE id=$2
	Resolve(ctx context.Context, id uint, resolvedAt time.Time) error

	// ListByMonitor returns recent incidents for a monitor, newest first.
	// Used by GET /monitors/:id/incidents.
	//
	// SQL: SELECT * FROM incidents WHERE monitor_id=$1 ORDER BY created_at DESC LIMIT $2
	ListByMonitor(ctx context.Context, monitorID uint, limit int) ([]domain.Incident, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// gormIncidentRepo — the GORM implementation
// ─────────────────────────────────────────────────────────────────────────────

type gormIncidentRepo struct {
	db *gorm.DB
}

// NewIncidentRepository constructs an IncidentRepository backed by *gorm.DB.
func NewIncidentRepository(db *gorm.DB) IncidentRepository {
	return &gormIncidentRepo{db: db}
}

// Create inserts a new open incident.
func (r *gormIncidentRepo) Create(ctx context.Context, i *domain.Incident) error {
	if err := r.db.WithContext(ctx).Create(i).Error; err != nil {
		return fmt.Errorf("incident create for monitor %d: %w", i.MonitorID, translateError(err))
	}
	return nil
}

// OpenByMonitor finds the current open incident for a monitor.
// "resolved_at IS NULL" is the open condition — GORM handles the nil pointer.
//
// Python: session.query(Incident).filter_by(monitor_id=mid, resolved_at=None).one_or_none()
// Node.js: Incident.findOne({ where: { monitorId, resolvedAt: null } })
func (r *gormIncidentRepo) OpenByMonitor(ctx context.Context, monitorID uint) (*domain.Incident, error) {
	var i domain.Incident
	err := r.db.WithContext(ctx).
		Where("monitor_id = ? AND resolved_at IS NULL", monitorID).
		First(&i).Error
	if err != nil {
		return nil, fmt.Errorf("open incident for monitor %d: %w", monitorID, translateError(err))
	}
	return &i, nil
}

// Resolve sets resolved_at on an incident.
// We use Updates(map) to update only resolved_at, not the whole struct.
//
// WHY map[string]any and not Updates(struct)?
// GORM skips ZERO-VALUE fields when you pass a struct to Updates().
// A time.Time zero value would be SKIPPED — resolved_at would not be set!
// Using a map guarantees the value is always written, even if it's a zero.
//
// Python: session.query(Incident).filter_by(id=id).update({"resolved_at": ts})
// Node.js: await Incident.update({ resolvedAt: ts }, { where: { id } })
func (r *gormIncidentRepo) Resolve(ctx context.Context, id uint, resolvedAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Incident{}).
		Where("id = ?", id).
		Updates(map[string]any{"resolved_at": resolvedAt})
	if result.Error != nil {
		return fmt.Errorf("resolve incident %d: %w", id, translateError(result.Error))
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("resolve incident %d: %w", id, domain.ErrNotFound)
	}
	return nil
}

// ListByMonitor returns recent incidents ordered newest-first.
func (r *gormIncidentRepo) ListByMonitor(ctx context.Context, monitorID uint, limit int) ([]domain.Incident, error) {
	var incidents []domain.Incident
	err := r.db.WithContext(ctx).
		Where("monitor_id = ?", monitorID).
		Order("created_at DESC").
		Limit(limit).
		Find(&incidents).Error
	if err != nil {
		return nil, fmt.Errorf("incidents for monitor %d: %w", monitorID, translateError(err))
	}
	return incidents, nil
}
