package memory

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/memory/seed"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// defaultMarketOrder is the load-time filter and fixed display order
// required by D5 and specs/events-catalog "Event Detail With Ordered
// Default Markets": 1X2, Total Goals, Both Teams to Score, First Goal.
// Every other market type present in the seed data is dropped at load time.
var defaultMarketOrder = []event.MarketTypeID{
	event.MarketTypeMoneyline,
	event.MarketTypeTotalGoals,
	event.MarketTypeBothTeamsScore,
	event.MarketTypeFirstGoal,
}

// --- raw seed JSON shapes ---
//
// These mirror only the fields the domain needs; encoding/json silently
// skips every other key in data.json at no cost (there are ~40 unused keys
// per event/market/selection in the source dataset).

type rawData struct {
	Events []rawEvent `json:"Events"`
}

type rawEvent struct {
	ID             string           `json:"_id"`
	EventName      string           `json:"EventName"`
	StartEventDate string           `json:"StartEventDate"`
	LeagueName     string           `json:"LeagueName"`
	IsLive         bool             `json:"IsLive"`
	IsSuspended    bool             `json:"IsSuspended"`
	Settings       rawSettings      `json:"Settings"`
	Participants   []rawParticipant `json:"Participants"`
	Markets        []rawMarket      `json:"Markets"`
}

type rawSettings struct {
	HasStatistics       bool `json:"HasStatistics"`
	IsBetBuilderEnabled bool `json:"IsBetBuilderEnabled"`
}

type rawParticipant struct {
	ID        string `json:"_id"`
	Name      string `json:"Name"`
	VenueRole string `json:"VenueRole"`
}

type rawMarket struct {
	ID         string         `json:"_id"`
	MarketType rawMarketType  `json:"MarketType"`
	Name       string         `json:"Name"`
	Selections []rawSelection `json:"Selections"`
}

type rawMarketType struct {
	ID string `json:"_id"`
}

type rawSelection struct {
	ID         string   `json:"_id"`
	MarketID   string   `json:"MarketId"`
	EventID    string   `json:"EventId"`
	Name       string   `json:"Name"`
	Points     *float64 `json:"Points"`
	TrueOdds   float64  `json:"TrueOdds"`
	IsDisabled bool     `json:"IsDisabled"`
}

// loadEvents decodes raw (the embedded seed JSON), applies the load-time
// market filter/sort (D5), resolves each event's phase/group enrichment,
// and returns the events sorted by StartsAt then Name.
func loadEvents(raw []byte) ([]event.Event, error) {
	var data rawData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("memory: decode seed data: %w", err)
	}

	events := make([]event.Event, 0, len(data.Events))
	for _, re := range data.Events {
		e, err := buildEvent(re)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}

	sort.Slice(events, func(i, j int) bool {
		if !events[i].StartsAt.Equal(events[j].StartsAt) {
			return events[i].StartsAt.Before(events[j].StartsAt)
		}
		return events[i].Name < events[j].Name
	})

	return events, nil
}

func buildEvent(re rawEvent) (event.Event, error) {
	startsAt, err := time.Parse(time.RFC3339, re.StartEventDate)
	if err != nil {
		return event.Event{}, fmt.Errorf("memory: event %s: parse StartEventDate: %w", re.ID, err)
	}

	home, away := participants(re.Participants)

	group, resolved := ResolveGroup(home.Name, away.Name)
	if !resolved {
		slog.Warn("event group could not be resolved from the seed map",
			"event_id", re.ID, "event_name", re.EventName)
	}

	markets, err := buildMarkets(re.ID, re.Markets)
	if err != nil {
		return event.Event{}, err
	}

	return event.Event{
		ID:                  re.ID,
		Name:                re.EventName,
		StartsAt:            startsAt.UTC(),
		League:              re.LeagueName,
		Home:                home,
		Away:                away,
		Phase:               event.PhaseGroupStage,
		Group:               group,
		IsLive:              re.IsLive,
		IsSuspended:         re.IsSuspended,
		Markets:             markets,
		HasStatistics:       re.Settings.HasStatistics,
		IsBetBuilderEnabled: re.Settings.IsBetBuilderEnabled,
	}, nil
}

// participants splits the raw participant list into home/away by VenueRole.
// A missing role simply leaves the corresponding Participant zero-valued —
// resolveGroup then treats the empty name as unseeded (fallback, never a
// panic).
func participants(raw []rawParticipant) (home, away event.Participant) {
	for _, p := range raw {
		switch p.VenueRole {
		case "Home":
			home = event.Participant{ID: p.ID, Name: p.Name}
		case "Away":
			away = event.Participant{ID: p.ID, Name: p.Name}
		}
	}
	return home, away
}

// ResolveGroup resolves the group letter shared by home and away using the
// verified seed map (seed.GroupByTeam). It returns (Group(""), false) when
// either team is unseeded or the two teams disagree on their letter — the
// required fallback path (spec: events-catalog/Phase and Group Enrichment)
// — and never panics.
func ResolveGroup(home, away string) (event.Group, bool) {
	homeLetter, homeOK := seed.GroupByTeam[home]
	awayLetter, awayOK := seed.GroupByTeam[away]
	if !homeOK || !awayOK || homeLetter != awayLetter {
		return event.Group(""), false
	}

	g, err := event.NewGroup(homeLetter)
	if err != nil {
		// Defensive: a malformed seed letter must still fall back safely.
		return event.Group(""), false
	}
	return g, true
}

// buildMarkets keeps only the four default market types (D5), in their
// fixed display order. An event missing one or more of them (the seed data
// does have such events) is simply served without those markets — never an
// error, never a panic.
func buildMarkets(eventID string, raw []rawMarket) ([]event.Market, error) {
	byType := make(map[event.MarketTypeID]rawMarket, len(raw))
	for _, m := range raw {
		byType[event.MarketTypeID(m.MarketType.ID)] = m
	}

	markets := make([]event.Market, 0, len(defaultMarketOrder))
	for order, typeID := range defaultMarketOrder {
		rm, ok := byType[typeID]
		if !ok {
			continue
		}

		selections, err := buildSelections(rm.Selections)
		if err != nil {
			return nil, fmt.Errorf("memory: event %s: market %s: %w", eventID, rm.ID, err)
		}

		markets = append(markets, event.Market{
			ID:         rm.ID,
			TypeID:     typeID,
			Name:       rm.Name,
			Order:      order,
			Selections: selections,
		})
	}
	return markets, nil
}

// buildSelections maps the raw selections of one market into domain
// Selections.
//
// A small number of seed selections are disabled placeholders carrying a
// TrueOdds below the domain minimum (e.g. 0) — real betting odds that were
// never published. Since a disabled selection can never be priced
// (BetSlip.Quote rejects it via IsDisabled before Odds is ever read), such a
// selection is clamped to the domain minimum instead of failing the entire
// boot load; an enabled selection with invalid odds is a genuine data
// defect and still fails loudly.
func buildSelections(raw []rawSelection) ([]event.Selection, error) {
	selections := make([]event.Selection, 0, len(raw))
	for _, rs := range raw {
		odds, err := money.NewOddsFromFloat(rs.TrueOdds)
		if err != nil {
			if !rs.IsDisabled {
				return nil, fmt.Errorf("selection %s: %w", rs.ID, err)
			}
			odds, err = money.NewOddsFromFloat(1.01)
			if err != nil {
				return nil, fmt.Errorf("selection %s: clamp disabled odds: %w", rs.ID, err)
			}
		}

		var line *decimal.Decimal
		if rs.Points != nil {
			d := decimal.NewFromFloat(*rs.Points)
			line = &d
		}

		selections = append(selections, event.Selection{
			ID:         rs.ID,
			MarketID:   rs.MarketID,
			EventID:    rs.EventID,
			Name:       rs.Name,
			Line:       line,
			Odds:       odds,
			IsDisabled: rs.IsDisabled,
		})
	}
	return selections, nil
}
