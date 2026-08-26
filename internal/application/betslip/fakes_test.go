package betslip_test

import (
	"context"

	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// fakeEventCatalog is a test double for betslip.EventCatalog, shared by
// calculate_test.go, place_test.go and place_race_test.go. It resolves
// selection IDs from an in-memory map, mirroring the real memory adapter's
// eventual SelectionsByIDs behavior: an unresolved ID yields the typed
// domainbetslip.ErrSelectionNotFound, exactly like the real adapter will.
type fakeEventCatalog struct {
	bySelectionID map[string]domainevent.SelectionRef
	err           error
	lastIDs       []string
}

// newFakeCatalog indexes refs by ID for lookup by SelectionsByIDs.
func newFakeCatalog(refs ...domainevent.SelectionRef) *fakeEventCatalog {
	byID := make(map[string]domainevent.SelectionRef, len(refs))
	for _, ref := range refs {
		byID[ref.ID] = ref
	}
	return &fakeEventCatalog{bySelectionID: byID}
}

func (f *fakeEventCatalog) SelectionsByIDs(_ context.Context, ids []string) ([]domainevent.SelectionRef, error) {
	f.lastIDs = ids
	if f.err != nil {
		return nil, f.err
	}

	refs := make([]domainevent.SelectionRef, 0, len(ids))
	for _, id := range ids {
		ref, ok := f.bySelectionID[id]
		if !ok {
			return nil, domainbetslip.ErrSelectionNotFound{SelectionID: id}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}
