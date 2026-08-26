package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/dto"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	appevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/event"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeEventCatalog is a test double for application/event.EventCatalog.
type fakeEventCatalog struct {
	events    []domainevent.Event
	byID      map[string]domainevent.Event
	lastFrom  time.Time
	lastTo    time.Time
	listErr   error
	detailErr error
}

func newFakeEventCatalog(events ...domainevent.Event) *fakeEventCatalog {
	byID := make(map[string]domainevent.Event, len(events))
	for _, e := range events {
		byID[e.ID] = e
	}
	return &fakeEventCatalog{events: events, byID: byID}
}

func (f *fakeEventCatalog) List(_ context.Context, from, to time.Time) ([]domainevent.Event, error) {
	f.lastFrom, f.lastTo = from, to
	if f.listErr != nil {
		return nil, f.listErr
	}
	// Mirrors the real memory.EventRepository.List's inverted-range guard
	// (spec: events-catalog/List Events by Date Range) so handler tests can
	// exercise that error path without depending on the real adapter.
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return nil, domainevent.ErrInvalidDateRange
	}
	var out []domainevent.Event
	for _, e := range f.events {
		if !from.IsZero() && e.StartsAt.Before(from) {
			continue
		}
		if !to.IsZero() && e.StartsAt.After(to) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeEventCatalog) Detail(_ context.Context, id string) (domainevent.Event, error) {
	if f.detailErr != nil {
		return domainevent.Event{}, f.detailErr
	}
	e, ok := f.byID[id]
	if !ok {
		return domainevent.Event{}, domainevent.ErrEventNotFound
	}
	return e, nil
}

func newEventsRouter(catalog *fakeEventCatalog) *gin.Engine {
	h := handler.NewEvents(appevent.NewList(catalog), appevent.NewDetail(catalog))
	r := gin.New()
	r.GET("/events", h.List)
	r.GET("/events/:id", h.Detail)
	return r
}

// TestEventsList_ReturnsEventsWithinRange proves GET /events filters by the
// from/to query parameters and returns 200 with the matching events (spec:
// events-catalog/List Events by Date Range).
func TestEventsList_ReturnsEventsWithinRange(t *testing.T) {
	e1 := domainevent.Event{ID: "e1", Name: "A vs B", StartsAt: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)}
	e2 := domainevent.Event{ID: "e2", Name: "C vs D", StartsAt: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)}
	r := newEventsRouter(newFakeEventCatalog(e1, e2))

	req := httptest.NewRequest(http.MethodGet, "/events?from=2026-06-01&to=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got []dto.EventSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 1)
	require.Equal(t, "e1", got[0].ID)
}

// TestEventsList_DateOnlyToIncludesWholeDay proves a date-only "to" query
// parameter (no time component) includes every event on that calendar day,
// not just events at or before midnight (design.md: "to 23:59:59.999Z
// inclusive").
func TestEventsList_DateOnlyToIncludesWholeDay(t *testing.T) {
	lateEvent := domainevent.Event{ID: "e1", StartsAt: time.Date(2026, 6, 15, 22, 30, 0, 0, time.UTC)}
	r := newEventsRouter(newFakeEventCatalog(lateEvent))

	req := httptest.NewRequest(http.MethodGet, "/events?from=2026-06-12&to=2026-06-15", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got []dto.EventSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 1, "a date-only 'to' must include the whole day, not just up to 00:00:00")
}

// TestEventsList_EmptyRangeReturnsEmptyList proves an empty result is 200
// with an empty list, never an error (spec: events-catalog/List Events by
// Date Range, "Empty range returns empty list, not an error").
func TestEventsList_EmptyRangeReturnsEmptyList(t *testing.T) {
	r := newEventsRouter(newFakeEventCatalog())

	req := httptest.NewRequest(http.MethodGet, "/events?from=2020-01-01&to=2020-01-02", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `[]`, w.Body.String())
}

// TestEventsList_InvertedRangeReturns400Envelope proves the typed
// event.ErrInvalidDateRange domain error surfaces as the standard envelope
// at 400 (spec: api-platform/Consistent Error Envelope).
func TestEventsList_InvertedRangeReturns400Envelope(t *testing.T) {
	r := newEventsRouter(newFakeEventCatalog())

	req := httptest.NewRequest(http.MethodGet, "/events?from=2026-06-20&to=2026-06-10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	require.Equal(t, "INVALID_DATE_RANGE", errObj["code"])
}

// TestEventDetail_ExposesMarketTypeIdAndEventLevelSettings proves the
// detail response carries MarketType.id on every market and the UI
// metadata flags on the event itself (spec: events-catalog/Market and
// Event Metadata Exposure).
func TestEventDetail_ExposesMarketTypeIdAndEventLevelSettings(t *testing.T) {
	e := domainevent.Event{
		ID: "e1", HasStatistics: true, IsBetBuilderEnabled: true,
		Markets: []domainevent.Market{
			{ID: "m1", TypeID: domainevent.MarketTypeMoneyline, Name: "1X2"},
			{ID: "m2", TypeID: domainevent.MarketTypeTotalGoals, Name: "Total Goals"},
		},
	}
	r := newEventsRouter(newFakeEventCatalog(e))

	req := httptest.NewRequest(http.MethodGet, "/events/e1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got dto.EventDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.True(t, got.Settings.HasStatistics)
	require.True(t, got.Settings.IsBetBuilderEnabled)
	require.Len(t, got.Markets, 2)
	require.Equal(t, "ML0", got.Markets[0].MarketType.ID)
	require.Equal(t, "OU200", got.Markets[1].MarketType.ID)
}

// TestEventDetail_UnknownIDReturns404Envelope proves an unknown event ID
// surfaces as the standard envelope at 404 (spec: events-catalog/Event
// Detail With Ordered Default Markets, "Unknown event ID").
func TestEventDetail_UnknownIDReturns404Envelope(t *testing.T) {
	r := newEventsRouter(newFakeEventCatalog())

	req := httptest.NewRequest(http.MethodGet, "/events/does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	require.Equal(t, "EVENT_NOT_FOUND", errObj["code"])
}
