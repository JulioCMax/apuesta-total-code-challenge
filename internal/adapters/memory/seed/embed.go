// Package seed embeds the World Cup 2026 reference dataset and the curated
// team-to-group map used to enrich it. The dataset is embedded (D4) so boot
// can never fail on a missing file and no runtime file path is required.
package seed

import _ "embed"

// Data is the raw, embedded contents of data.json — the canonical World Cup
// 2026 events/markets/selections reference data, relocated here from
// docs/data.json so go:embed can reach it (D4).
//
//go:embed data.json
var Data []byte
