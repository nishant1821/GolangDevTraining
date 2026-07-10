// Package repository is the database access layer.
// Each repository implements an interface defined in the service layer,
// wrapping GORM calls so the service never touches *gorm.DB directly.
//
// Why interfaces? So tests can inject a fake/mock repository without needing
// a real PostgreSQL database.
//
//   Python analogy: a class that wraps SQLAlchemy session calls.
//   Node.js analogy: a class that wraps Sequelize / Knex calls.
//
// Built in Stage 2.
package repository
