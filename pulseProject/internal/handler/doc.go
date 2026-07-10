// Package handler is the HTTP layer of Pulse.
// Handlers parse requests, call service methods, and write JSON responses.
// They know about HTTP but NOTHING about databases or business rules.
//
// Dependency: handler → service (never handler → repository directly)
//
// Built in Stage 4.
package handler
