package event_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/application/event"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// TestDetail_ReturnsCatalogResult proves the Detail use case delegates the
// id and returns the catalog's event unchanged.
func TestDetail_ReturnsCatalogResult(t *testing.T) {
	want := domainevent.Event{ID: "evt-1", Name: "Test Event"}
	catalog := &fakeEventCatalog{detailResult: want}
	uc := event.NewDetail(catalog)

	got, err := uc.Execute(context.Background(), "evt-1")

	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, "evt-1", catalog.lastDetailID)
}

// TestDetail_PropagatesUnknownIDError proves an unknown event ID surfaces
// the typed event.ErrEventNotFound sentinel unchanged (spec: events-
// catalog/Event Detail With Ordered Default Markets, "Unknown event ID").
func TestDetail_PropagatesUnknownIDError(t *testing.T) {
	catalog := &fakeEventCatalog{detailErr: domainevent.ErrEventNotFound}
	uc := event.NewDetail(catalog)

	_, err := uc.Execute(context.Background(), "unknown-id")

	require.ErrorIs(t, err, domainevent.ErrEventNotFound)
}
