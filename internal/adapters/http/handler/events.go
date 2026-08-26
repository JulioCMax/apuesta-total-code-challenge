// Package handler holds the Gin handlers for every endpoint (design.md's
// Package Tree: adapters/http/handler). Handlers are thin: bind/validate
// the request DTO, call exactly one application use case, map the result
// (or error) to a response DTO or the shared apperror envelope. No
// business rule lives here.
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/dto"
	appevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/event"
)

// Events serves GET /events and GET /events/:id (spec: events-catalog).
type Events struct {
	list   *appevent.List
	detail *appevent.Detail
}

// NewEvents builds an Events handler backed by list and detail.
func NewEvents(list *appevent.List, detail *appevent.Detail) *Events {
	return &Events{list: list, detail: detail}
}

// List handles GET /events?from=&to=. Each bound accepts either a full
// RFC3339 timestamp or a date-only "YYYY-MM-DD" value; an omitted bound
// leaves that side open. A date-only "to" is extended to the last instant
// of that day so the whole calendar day is included (design.md: "to
// 23:59:59.999Z inclusive"; spec: events-catalog/List Events by Date
// Range).
func (h *Events) List(c *gin.Context) {
	from, err := parseFromParam(c.Query("from"))
	if err != nil {
		apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "El parámetro 'from' no es una fecha válida.")
		return
	}
	to, err := parseToParam(c.Query("to"))
	if err != nil {
		apperror.WriteStatus(c, http.StatusBadRequest, "VALIDATION_ERROR", "El parámetro 'to' no es una fecha válida.")
		return
	}

	events, err := h.list.Execute(c.Request.Context(), from, to)
	if err != nil {
		apperror.Write(c, err)
		return
	}

	c.JSON(http.StatusOK, dto.EventsFromDomain(events))
}

// Detail handles GET /events/:id (spec: events-catalog/Event Detail With
// Ordered Default Markets, /Market and Event Metadata Exposure).
func (h *Events) Detail(c *gin.Context) {
	e, err := h.detail.Execute(c.Request.Context(), c.Param("id"))
	if err != nil {
		apperror.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.EventDetailFromDomain(e))
}

const dateOnlyLayout = "2006-01-02"

// parseFromParam parses an optional lower date-range bound: an empty value
// leaves the bound open (zero time.Time).
func parseFromParam(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(dateOnlyLayout, raw)
}

// parseToParam parses an optional upper date-range bound. A date-only value
// is extended to 23:59:59.999999999 of that day so the entire calendar day
// is included (an explicit RFC3339 timestamp is used exactly as given —
// the caller asked for a precise instant).
func parseToParam(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	d, err := time.Parse(dateOnlyLayout, raw)
	if err != nil {
		return time.Time{}, err
	}
	return d.Add(24*time.Hour - time.Nanosecond), nil
}
