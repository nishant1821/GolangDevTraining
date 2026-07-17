package store

import (
	"context"
	"errors"
	"sync"
)

// Not-found ko sentinel error banaya — taaki handler `errors.Is` se
// ise pakad ke 404 map kar sake. Yahi tera errors.Is wala revision point.
var ErrNotFound = errors.New("short code not found")

type InMemoryStore struct {
	mu   sync.Mutex
	data map[string]string // code → longURL
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		data: make(map[string]string), // YAAD: make zaroori, nil map pe write = panic
	}
}

func (s *InMemoryStore) Save(ctx context.Context, code, longURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[code] = longURL
	return nil
}

func (s *InMemoryStore) Find(ctx context.Context, code string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.data[code]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
