package template

import (
	"sync"
)

func NewSet() *Set {
	return &Set{
		m: make(map[string][]byte),
	}
}

type Set struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func (s *Set) Put(key string, value []byte) {
	s.mu.Lock()
	s.m[key] = value
	s.mu.Unlock()
}

// todo: not save for modification
func (s *Set) Value(key string) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// maps returns a zero value if the key is not present
	return s.m[key]
}

func (s *Set) Delete(key string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	delete(s.m, key)
}

func (s *Set) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.m)
}

func (s *Set) Contains(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.m[key]; ok {
		return true
	}
	return false
}

func (s *Set) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0)
	for key := range s.m {
		keys = append(keys, key)
	}
	return keys
}
