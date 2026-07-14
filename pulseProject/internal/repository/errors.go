package repository

// errors.go — shared error translation for all repository implementations.
//
// THE CORE RULE OF THIS FILE:
//   Infrastructure errors (gorm.ErrRecordNotFound, postgres constraint codes)
//   must NEVER leak into the service or handler layers.
//   They are translated here, at the boundary, into domain errors.
//
// WHY translate errors?
//
//   Without translation:
//     service/handler layer imports "gorm.io/gorm" just to check errors.
//     Now gorm leaks into every layer → you can never swap the DB.
//
//   With translation:
//     service does:  errors.Is(err, domain.ErrNotFound)
//     It doesn't know or care that the DB layer uses GORM.
//     Swap GORM for raw SQL tomorrow → zero changes outside repository/.
//
//   Python analogy:
//     SQLAlchemy raises NoResultFound → your service layer catches AppNotFoundError.
//     The translation happens in the repository method, not the service.
//
//   Node.js analogy:
//     Sequelize raises EmptyResultError → your service catches NotFoundError.
//     The repo catches Sequelize errors and re-throws domain errors.

import (
	"errors" // errors.Is — unwrap and check error chain

	"gorm.io/gorm"                              // gorm.ErrRecordNotFound — the infra error
	"github.com/nishantks908/pulse/internal/domain" // domain.ErrNotFound — the domain error
)

// translateError converts GORM/Postgres errors into domain-layer errors.
// All repository methods call this before returning an error to the caller.
//
// Current translations:
//   gorm.ErrRecordNotFound  → domain.ErrNotFound
//   everything else         → unchanged (passed through as-is)
//
// Add more translations here as needed (e.g., unique-constraint → domain.ErrConflict).
func translateError(err error) error {
	if err == nil {
		return nil // fast path: no error — nothing to translate
	}

	// errors.Is unwraps the error chain — works even if GORM wrapped the error
	// with additional context using fmt.Errorf("...: %w", gorm.ErrRecordNotFound).
	//
	// Python: except NoResultFound → raise NotFoundError(...)
	// Node.js: if (e instanceof EmptyResultError) throw new NotFoundError()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}

	// All other errors pass through unchanged.
	// The caller (service/handler) will see them as opaque infrastructure errors.
	// They'll typically become HTTP 500 at the handler level.
	return err
}
