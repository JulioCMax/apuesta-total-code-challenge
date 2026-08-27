package seed

// BetBuilderDisabled is a small, authored set of event IDs whose Bet
// Builder capability is treated as disabled, overriding data.json's own
// Settings.IsBetBuilderEnabled flag for exactly those events.
//
// This is NOT sourced from data.json — every one of the 24 seeded events
// carries IsBetBuilderEnabled true there (the source dataset has no
// disabled event at all), which makes betslip.ErrBetBuilderNotAvailable a
// dead branch no real request could ever reach, leaving the Bet Builder
// gate undemonstrable end-to-end. This is authored demo data, mirroring
// SuperCuotaOdds's own provenance note above, existing purely to make that
// gate reachable against the real catalog.
//
// Both events below are deliberately absent from SuperCuotaOdds's five
// curated events, so the two demos never collide on the same fixture:
//
//	event               id                   reason
//	Catar vs Suiza      784926068556738560   Bet Builder demo (gate proof)
//	Haití vs Escocia    784926067864678400   Bet Builder demo (gate proof)
//
// loader.go applies this in buildEvent: an event ID present here forces
// Event.IsBetBuilderEnabled to false regardless of the source flag, which
// then propagates to every SelectionRef the catalog resolves for that
// event (event.NewSelectionRef), reaching BetSlip.Quote's Bet Builder gate
// exactly as a genuinely disabled event would (spec: bet-slip-calculation/
// Same-Event Combo Rejection, "Bet Builder opt-in on a disabled event").
// Two guard tests in adapters/memory/betbuilder_test.go, mirroring the
// Super Cuota guards, keep this map honest: every ID here must resolve to
// a real seeded event, and that event's flag must actually load false.
var BetBuilderDisabled = map[string]bool{
	"784926068556738560": true, // Catar vs Suiza
	"784926067864678400": true, // Haití vs Escocia
}
