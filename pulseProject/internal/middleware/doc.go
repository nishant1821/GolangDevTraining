// Package middleware contains HTTP middleware for chi.
// Middleware wraps handlers to add cross-cutting concerns:
//   - JWT authentication (verify token, inject user into context)
//   - Request ID (trace individual requests through logs)
//   - Rate limiting
//
// Built in Stage 5.
package middleware
