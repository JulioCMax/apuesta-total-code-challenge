package seed

// GroupByTeam maps a World Cup 2026 participant name — spelled exactly as
// it appears in data.json — to its group letter (A-L).
//
// data.json carries no phase/group signal at all, so this content is
// transcribed from the official FIFA World Cup 2026 Final Draw (2025-12-05),
// cross-checked against the challenge PDF screenshots and against the two
// team clusters derivable from data.json itself (Engram #1627). It covers
// all 39 participant names across all 24 seeded events; see
// docs/adr/0008-*.md for the full provenance write-up.
var GroupByTeam = map[string]string{
	// Group A
	"México":        "A",
	"Sudáfrica":     "A",
	"Corea del Sur": "A",

	// Group B
	"Canadá": "B",
	"Catar":  "B",
	"Suiza":  "B",

	// Group C
	"Brasil":    "C",
	"Marruecos": "C",
	"Haití":     "C",
	"Escocia":   "C",

	// Group D
	"EE.UU.":    "D",
	"Paraguay":  "D",
	"Australia": "D",

	// Group E
	"Alemania":        "E",
	"Curazao":         "E",
	"Costa de Marfil": "E",
	"Ecuador":         "E",

	// Group F
	"Países Bajos": "F",
	"Japón":        "F",

	// Group G
	"Bélgica":       "G",
	"Egipto":        "G",
	"Irán":          "G",
	"Nueva Zelanda": "G",

	// Group H
	"España":         "H",
	"Cabo Verde":     "H",
	"Uruguay":        "H",
	"Arabia Saudita": "H",

	// Group I
	"Francia": "I",
	"Senegal": "I",

	// Group J
	"Argentina": "J",
	"Argelia":   "J",
	"Austria":   "J",
	"Jordania":  "J",

	// Group K
	"Colombia":   "K",
	"Uzbekistán": "K",

	// Group L
	"Inglaterra": "L",
	"Croacia":    "L",
	"Ghana":      "L",
	"Panamá":     "L",
}
