// Package checker performs a single HTTP health-check against one URL.
// It returns a domain.Check (status code, response time, up/down, error message).
// Each check runs with a per-request context timeout so a slow server
// cannot hold a worker goroutine indefinitely.
//
// Built in Stage 3.
package checker
