package transform

import (
	"context"
	"fmt"
	"maps"
	"sync"
)

type stateCtxKey struct{}

// State is a per-request store coordinating transformers across phases —
// e.g. reversible anonymization records placeholder → original mappings on
// input that the deanonymize transformer restores on output. Values live in
// memory only for the request's duration and must never be logged or
// audited.
type State struct {
	mu       sync.Mutex
	mappings map[string]string // placeholder → original
	counters map[string]int    // entity type → next placeholder number
}

// WithState returns a context carrying a fresh State. The processor calls
// this once per request, before running either transformer chain.
func WithState(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateCtxKey{}, &State{
		mappings: make(map[string]string),
		counters: make(map[string]int),
	})
}

// StateFrom extracts the request's State, if any.
func StateFrom(ctx context.Context) (*State, bool) {
	s, ok := ctx.Value(stateCtxKey{}).(*State)
	return s, ok
}

// AllocatePlaceholder returns the placeholder for an original value,
// creating a numbered one (<PERSON_1>) if this value hasn't been seen.
// Numbering is request-global, so multiple transformers can't collide.
func (s *State) AllocatePlaceholder(entityType, original string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for p, o := range s.mappings {
		if o == original {
			return p
		}
	}
	s.counters[entityType]++
	placeholder := fmt.Sprintf("<%s_%d>", entityType, s.counters[entityType])
	s.mappings[placeholder] = original
	return placeholder
}

// Mappings returns a copy of all recorded mappings.
func (s *State) Mappings() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.mappings))
	maps.Copy(out, s.mappings)
	return out
}

// Len returns the number of recorded mappings.
func (s *State) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.mappings)
}
