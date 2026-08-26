package event

import (
	"context"
	"time"

	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// List is the "list events by date range" use case (spec: events-catalog/
// List Events by Date Range).
type List struct {
	catalog EventCatalog
}

// NewList builds a List use case backed by catalog.
func NewList(catalog EventCatalog) *List {
	return &List{catalog: catalog}
}

// Execute returns every event within [from, to], both bounds inclusive.
func (l *List) Execute(ctx context.Context, from, to time.Time) ([]domainevent.Event, error) {
	return l.catalog.List(ctx, from, to)
}
