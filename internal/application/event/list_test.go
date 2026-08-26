package event_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/application/event"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// TestList_DelegatesRangeToCatalog proves the List use case passes the
// caller's from/to through unchanged and returns the catalog's result
// (spec: events-catalog/List Events by Date Range).
func TestList_DelegatesRangeToCatalog(t *testing.T) {
	want := []domainevent.Event{{ID: "evt-1"}, {ID: "evt-2"}}
	catalog := &fakeEventCatalog{listResult: want}
	uc := event.NewList(catalog)

	from := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	got, err := uc.Execute(context.Background(), from, to)

	require.NoError(t, err)
	require.Equal(t, want, got)
	require.True(t, catalog.lastFrom.Equal(from))
	require.True(t, catalog.lastTo.Equal(to))
}

// TestList_PropagatesInvalidRangeError proves a typed range error from the
// catalog surfaces unchanged, not wrapped or swallowed.
func TestList_PropagatesInvalidRangeError(t *testing.T) {
	catalog := &fakeEventCatalog{listErr: domainevent.ErrInvalidDateRange}
	uc := event.NewList(catalog)

	_, err := uc.Execute(context.Background(), time.Time{}, time.Time{})

	require.ErrorIs(t, err, domainevent.ErrInvalidDateRange)
}
