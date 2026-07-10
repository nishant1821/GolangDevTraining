// Package service contains the business logic of Pulse.
// Services orchestrate repositories and enforce domain rules.
// They know NOTHING about HTTP (no http.Request) or SQL (*gorm.DB).
//
// A service method looks like:
//   func (s *MonitorService) Create(ctx context.Context, m *domain.Monitor) error
//
// This is easy to unit-test: inject fake repositories, call the method,
// assert on the domain objects and returned errors.
//
//   Python analogy: a service.py class that calls repository methods.
//   Node.js analogy: a service class injected with a repository via constructor.
//
// Built in Stage 4.
package service
