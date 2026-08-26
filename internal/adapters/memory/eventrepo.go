// Package memory implements the in-memory event catalog: the 24 World Cup
// 2026 events are loaded once at boot from the embedded seed data (D3, D4)
// and never mutated at runtime. Only mutable state (users, balance, bets)
// touches DynamoDB (internal/adapters/dynamo).
package memory

import (
	"context"
	"time"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/memory/seed"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// EventRepository serves the seeded event catalog: date-range listing and
// detail lookup, both backed by a slice sorted once at load time plus a
// byID index.
type EventRepository struct {
	events []event.Event
	byID   map[string]event.Event
}

// NewEventRepository decodes the embedded seed dataset (seed.Data), applies
// the load-time market filter/sort and group/phase enrichment, and builds
// the lookup index. An error here means the embedded data itself is
// malformed — a boot-time failure, since the data is compiled into the
// binary and never expected to change at runtime.
func NewEventRepository() (*EventRepository, error) {
	events, err := loadEvents(seed.Data)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]event.Event, len(events))
	for _, e := range events {
		byID[e.ID] = e
	}

	return &EventRepository{events: events, byID: byID}, nil
}

// List returns every seeded event whose StartsAt falls within [from, to],
// both bounds inclusive. A zero from/to leaves that bound open. An inverted
// range (from after to, both non-zero) is a typed domain error
// (event.ErrInvalidDateRange). An empty result is an empty, non-nil slice
// with a nil error — never an error (spec: events-catalog/List Events by
// Date Range).
func (r *EventRepository) List(_ context.Context, from, to time.Time) ([]event.Event, error) {
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return nil, event.ErrInvalidDateRange
	}

	results := make([]event.Event, 0, len(r.events))
	for _, e := range r.events {
		if !from.IsZero() && e.StartsAt.Before(from) {
			continue
		}
		if !to.IsZero() && e.StartsAt.After(to) {
			continue
		}
		results = append(results, e)
	}
	return results, nil
}

// Detail returns the full event for id, markets already in default order
// (D5). An unknown id returns the typed event.ErrEventNotFound sentinel.
func (r *EventRepository) Detail(_ context.Context, id string) (event.Event, error) {
	e, ok := r.byID[id]
	if !ok {
		return event.Event{}, event.ErrEventNotFound
	}
	return e, nil
}
