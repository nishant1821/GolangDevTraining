// Package checker performs a single HTTP health-check against one URL.
//
// Public surface (what other packages use):
//
//	type Checker interface { Check(ctx, url) Result }
//	type Result struct { StatusCode, LatencyMs, Up, Err }
//
// Private implementation: HTTPChecker — uses net/http with context-bound requests.
// Tests inject a fake that satisfies Checker without making real HTTP calls.
package checker
