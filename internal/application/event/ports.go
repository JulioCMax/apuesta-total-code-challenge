// Package event holds the List and Detail use cases for the events
// catalog. Both are thin orchestration over a caller-supplied EventCatalog
// port, keeping the HTTP adapter decoupled from the concrete in-memory
// repository (D2 — consumer-owned interfaces, no ports/ folder).
package event

import (
	"context"
	"time"

	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// EventCatalog is the read-only event catalog consumed by List and Detail.
// internal/adapters/memory.EventRepository satisfies it structurally.
type EventCatalog interface {
	// List returns every event whose StartsAt falls within [from, to],
	// both bounds inclusive; a zero from/to leaves that bound open.
	List(ctx context.Context, from, to time.Time) ([]domainevent.Event, error)
	// Detail returns the full event for id, or a typed "not found" error.
	Detail(ctx context.Context, id string) (domainevent.Event, error)
}
