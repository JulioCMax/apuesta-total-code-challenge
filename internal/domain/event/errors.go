package event

import "errors"

var (
	// ErrInvalidGroup is returned when a Group value is non-empty and not
	// one of the 12 World Cup group letters (A-L).
	ErrInvalidGroup = errors.New("event: group must be a letter A-L or empty")

	// ErrEventNotFound is returned when a requested event ID does not exist
	// in the catalog.
	ErrEventNotFound = errors.New("event: not found")

	// ErrInvalidDateRange is returned when a list query's "from" date is
	// after its "to" date.
	ErrInvalidDateRange = errors.New("event: from must not be after to")
)
