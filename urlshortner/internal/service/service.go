package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/nishantks908/urlshortener/internal/shortener"
)

var ErrInvalidURL = errors.New("invalid or empty URL")

// Chef ko storeroom se sirf itna chahiye. InMemoryStore ye already
// satisfy karta hai (Save + Find dono hain) — implicitly, bina "implements"
// likhe. Yahi Go ka structural interface.
type Store interface {
	Save(ctx context.Context, code, longURL string) error
	Find(ctx context.Context, code string) (string, error)
}

type Service struct {
	store   Store // concrete nahi — INTERFACE. yahi wo ek-line fix tha.
	counter uint64
}

func New(st Store) *Service {
	return &Service{store: st}
}

// Shorten: lambi URL → chhota code
func (s *Service) Shorten(ctx context.Context, longURL string) (string, error) {
	if longURL == "" {
		return "", ErrInvalidURL
	}

	n := atomic.AddUint64(&s.counter, 1)
	code := shortener.Encode(n)

	if err := s.store.Save(ctx, code, longURL); err != nil {
		return "", fmt.Errorf("save code: %w", err)
	}
	return code, nil
}

// Resolve: chhota code → lambi URL (redirect handler isko call karega)
func (s *Service) Resolve(ctx context.Context, code string) (string, error) {
	return s.store.Find(ctx, code)
}
