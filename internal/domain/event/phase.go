package event

// Phase identifies the competition stage of an Event. Only PhaseGroupStage
// is present in the World Cup 2026 group-stage sample dataset; knockout
// phases can be appended to this list without touching any caller.
type Phase string

// PhaseGroupStage is the only phase present in the 2026 sample dataset.
const PhaseGroupStage Phase = "group_stage"
