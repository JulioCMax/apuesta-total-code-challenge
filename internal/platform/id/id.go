// Package id provides concurrency-safe unique identifier generation. The
// production implementation issues ULIDs (D9): lexicographically sortable
// by creation time, so the DynamoDB bet SK (BET#<ulid>) orders a user's
// history chronologically for free, with no extra index or application-side
// sort.
package id

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ULIDGenerator issues fresh ULIDs. It satisfies any consumer-owned
// "NewID() string" port (e.g. application/betslip.IDGenerator) structurally
// — this package never imports application code (D2).
type ULIDGenerator struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

// NewULIDGenerator builds a generator whose entropy source is wrapped in
// ulid.Monotonic, so IDs minted within the same millisecond still sort in
// generation order.
func NewULIDGenerator() *ULIDGenerator {
	return &ULIDGenerator{entropy: ulid.Monotonic(rand.Reader, 0)}
}

// NewID returns a fresh ULID string. Safe for concurrent use: entropy
// reads are serialized behind a mutex, matching ulid.MonotonicEntropy's
// own single-goroutine-at-a-time contract.
func (g *ULIDGenerator) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), g.entropy).String()
}
