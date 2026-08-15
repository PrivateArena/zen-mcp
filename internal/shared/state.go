package shared

import (
	"reflect"
	"sync"
)

type Store struct {
	mu     sync.RWMutex
	values map[string]string
	subs   map[string][]func(string)
}

// NewStore is a helper function
func NewStore() *Store {
	return &Store{
		values: make(map[string]string),
		subs:   make(map[string][]func(string)),
	}
}

// Set is a helper function
func (s *Store) Set(key, value string) {
	s.mu.Lock()
	prev, existed := s.values[key]
	s.values[key] = value
	watchers := make([]func(string), 0, len(s.subs[key]))
	if existed && prev != value {
		watchers = append(watchers, s.subs[key]...)
	} else if !existed {
		watchers = append(watchers, s.subs[key]...)
	}
	s.mu.Unlock()
	for _, fn := range watchers {
		fn(value)
	}
}

// Get is a helper function
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[key]
	return v, ok
}

// GetAll is a helper function
func (s *Store) GetAll() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
}

// eqFunc is a helper function
func eqFunc(a, b func(string)) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

// Clear is a helper function
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = make(map[string]string)
}
