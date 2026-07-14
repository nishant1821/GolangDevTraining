package repository

// user_repo.go — UserRepository interface + GORM implementation.
//
// Users are the owners of monitors. Auth (JWT) is built in Stage 7,
// but the repository is created here so the DB schema is complete from day one.
//
// Important: passwords are NEVER stored in plain text.
// The service layer (Stage 6) runs bcrypt BEFORE calling Create.
// This repository only ever sees the bcrypt hash.

import (
	"context" // context.Context
	"fmt"     // fmt.Errorf

	"gorm.io/gorm" // *gorm.DB

	"github.com/nishantks908/pulse/internal/domain" // domain.User
)

// ─────────────────────────────────────────────────────────────────────────────
// UserRepository — the interface
// ─────────────────────────────────────────────────────────────────────────────

// UserRepository defines DB operations for User rows.
// The auth service (Stage 7) will call ByEmail on login and Create on register.
type UserRepository interface {
	// Create inserts a new user.
	// u.Password must already be a bcrypt hash — NEVER plain text.
	// GORM fills u.ID, u.CreatedAt, u.UpdatedAt.
	//
	// A UNIQUE constraint on email means duplicate registration returns an error.
	// The service should translate that to domain.ErrConflict (add to translateError later).
	Create(ctx context.Context, u *domain.User) error

	// ByID fetches a user by primary key.
	// Returns domain.ErrNotFound if no row exists.
	ByID(ctx context.Context, id uint) (*domain.User, error)

	// ByEmail fetches a user by email address (the login identifier).
	// Returns domain.ErrNotFound if no user with that email exists.
	// The auth service calls this on every login attempt.
	//
	// SQL: SELECT * FROM users WHERE email=$1 AND deleted_at IS NULL LIMIT 1
	ByEmail(ctx context.Context, email string) (*domain.User, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// gormUserRepo — the GORM implementation
// ─────────────────────────────────────────────────────────────────────────────

type gormUserRepo struct {
	db *gorm.DB
}

// NewUserRepository constructs a UserRepository backed by *gorm.DB.
func NewUserRepository(db *gorm.DB) UserRepository {
	return &gormUserRepo{db: db}
}

// Create inserts a new user row.
// If email already exists, Postgres raises a unique-constraint violation.
// translateError currently passes that through unchanged (will become domain.ErrConflict in Stage 7).
func (r *gormUserRepo) Create(ctx context.Context, u *domain.User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("user create: %w", translateError(err))
	}
	return nil
}

// ByID fetches a user by primary key.
func (r *gormUserRepo) ByID(ctx context.Context, id uint) (*domain.User, error) {
	var u domain.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, fmt.Errorf("user by id %d: %w", id, translateError(err))
	}
	return &u, nil
}

// ByEmail finds a user by email. Used on every login attempt.
// Where("email = ?", email) generates a parameterised query — SQL injection safe.
// First() returns gorm.ErrRecordNotFound if no row → translateError → domain.ErrNotFound.
//
// SECURITY NOTE: even if the user doesn't exist, the service should take
// the same amount of time to respond as a successful lookup (constant-time
// response). This prevents email enumeration via timing attacks.
// That timing logic lives in the auth service, not here.
//
// Python: session.query(User).filter_by(email=email).one_or_none()
// Node.js: User.findOne({ where: { email } })
func (r *gormUserRepo) ByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).
		Where("email = ?", email).
		First(&u).Error
	if err != nil {
		return nil, fmt.Errorf("user by email: %w", translateError(err))
	}
	return &u, nil
}
