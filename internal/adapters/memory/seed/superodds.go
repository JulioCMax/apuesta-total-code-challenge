package seed

// SuperCuotaOdds is a curated, hand-authored set of Super Cuota (boosted
// odds) selections: real selection IDs from data.json mapped to an
// improved odds value.
//
// This is NOT sourced from data.json — the source dataset carries no
// boost/promotion signal at all — it is authored promotional content,
// mirroring GroupByTeam's own provenance note above. Every value here is a
// deliberate, small improvement over the selection's real TrueOdds in
// data.json, spanning 5 distinct events so the boost never concentrates on
// one match:
//
//	selection                          original -> boosted
//	Bélgica vs Egipto, Bélgica (home)      1.56  ->  1.70
//	EE.UU. vs Paraguay, EE.UU. (home)      1.91  ->  2.05
//	Brasil vs Marruecos, Brasil (home)     1.61  ->  1.75
//	México vs Corea del Sur, México (home) 1.73  ->  1.85
//	Países Bajos vs Japón, Países Bajos    2.00  ->  2.15
//
// The México vs Sudáfrica 1X2 market is deliberately left uncurated: its
// "México" selection is the fixture other tests in this package (see
// adapters/memory/eventrepo_test.go) already pin to a specific unboosted
// odds value, and this map must never collide with an unrelated test's own
// fixture data.
//
// loader.go applies this after decode: it overwrites the selection's Odds
// with the boosted value and records the pre-boost value in OriginalOdds,
// so the UI can say "was X, now Y" honestly (design: Super Cuota). Two
// guard tests in adapters/memory/superodds_test.go, mirroring
// TestGroupSeed_CoversEveryEvent, keep this map honest: every ID here must
// resolve to a real seeded selection, and every boost must be strictly
// greater than the original — a "boost" that lowers odds is a bug, not a
// promotion.
//
// odds must always be a decimal STRING (never a float literal), matching
// every other money/odds value in this codebase (ADR-0005).
var SuperCuotaOdds = map[string]string{
	// Bélgica vs Egipto — Bélgica to win (was 1.56).
	"0ML784926070830059520H": "1.70",
	// EE.UU. vs Paraguay — EE.UU. to win (was 1.91).
	"0ML784926073862533120H": "2.05",
	// Brasil vs Marruecos — Brasil to win (was 1.61).
	"0ML784926072373547008H": "1.75",
	// México vs Corea del Sur — México to win (was 1.73).
	"0ML802017756223635456H": "1.85",
	// Países Bajos vs Japón — Países Bajos to win (was 2.00).
	"0ML784926071857676392H": "2.15",
}
