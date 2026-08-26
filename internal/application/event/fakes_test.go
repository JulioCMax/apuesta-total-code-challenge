package event_test

import (
	"context"
	"time"

	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// fakeEventCatalog is a test double for event.EventCatalog, shared by
// list_test.go and detail_test.go. It records its last call's arguments so
// tests can assert delegation, and returns caller-supplied canned results.
type fakeEventCatalog struct {
	listResult []domainevent.Event
	listErr    error

	detailResult domainevent.Event
	detailErr    error

	lastFrom, lastTo time.Time
	lastDetailID     string
}

func (f *fakeEventCatalog) List(_ context.Context, from, to time.Time) ([]domainevent.Event, error) {
	f.lastFrom, f.lastTo = from, to
	return f.listResult, f.listErr
}

func (f *fakeEventCatalog) Detail(_ context.Context, id string) (domainevent.Event, error) {
	f.lastDetailID = id
	return f.detailResult, f.detailErr
}
