package event

import (
	"context"

	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// Detail is the "event detail" use case (spec: events-catalog/Event Detail
// With Ordered Default Markets).
type Detail struct {
	catalog EventCatalog
}

// NewDetail builds a Detail use case backed by catalog.
func NewDetail(catalog EventCatalog) *Detail {
	return &Detail{catalog: catalog}
}

// Execute returns the full event for id, or a typed "not found" error.
func (d *Detail) Execute(ctx context.Context, id string) (domainevent.Event, error) {
	return d.catalog.Detail(ctx, id)
}
