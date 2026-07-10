// Package platform bootstraps external infrastructure connections:
//   - PostgreSQL via GORM (database/sql connection pool)
//   - Redis via go-redis (cache connection)
//   - Zerolog logger (structured JSON logging)
//
// These are "plumbing" — they are created once in main.go and injected
// down into repositories and services.
//
// Built in Stage 2.
package platform
