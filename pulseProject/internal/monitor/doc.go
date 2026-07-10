// Package monitor contains two components:
//
//  1. Scheduler — a goroutine that ticks on an interval, queries the DB for
//     monitors whose next-check time has passed, and sends them to a jobs channel.
//
//  2. Pool — a bounded set of worker goroutines that reads from the jobs channel,
//     calls checker.Check(), persists the result, and handles Up→Down transitions
//     (opening incidents, sending notifications).
//
// Built in Stage 6.
package monitor
