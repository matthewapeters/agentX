package task

// Likelihood is always an LLM judgment — advisory investigation priority,
// never treated as confirmed fact (only Record.Value/Record.Error, set by
// real tool execution, are). Impossible is a terminal state distinct in kind
// from Low, not merely its bottom rung: assignable only when a specific
// Evidence entry with StanceRefutes directly, decisively contradicts the
// hypothesis — not merely "no supporting evidence found yet" (that stays
// Low). Named for "decisively refuted by observation," not logical
// impossibility — real-world evidence rarely proves anything that strongly.
type Likelihood string

const (
	LikelihoodHigh       Likelihood = "H"
	LikelihoodMedium     Likelihood = "M"
	LikelihoodLow        Likelihood = "L"
	LikelihoodImpossible Likelihood = "I"
)

// Stance is how one piece of evidence bears on a hypothesis or resolution
// criterion. Required on every Evidence entry — an unstanced "this fact is
// relevant" link is ambiguous by construction (a fact can be cited as
// support and actually read as refutation to a careless reader).
type Stance string

const (
	StanceSupports Stance = "supports"
	StanceRefutes  Stance = "refutes"
)

// Evidence links a Hypothesis-kind node, or a ResolutionAssertion, to a
// fact-node that bears on it. NodeID references an existing Record.ID — no
// new identity scheme. Deliberately not a reuse of Record.Deps: Deps is a
// scheduling precondition (blocks dispatch until resolved); Evidence is a
// retrospective, non-blocking relationship that must never gate dispatch
// order the way a real dependency does.
type Evidence struct {
	NodeID string `json:"node_id"`
	Stance Stance `json:"stance"`
}

// AssertionOutcome mirrors ADR 0010's tri-state judgment vocabulary
// (satisfied/refuted/abstained) verbatim — ADR 0010 itself is unimplemented
// as of this phase, so no existing Go type is being duplicated; this phase
// defines the vocabulary the way ADR 0010 already specified it in prose, so
// a future ADR 0010 implementation and Tidal share one spelling if ADR 0010
// lands afterward.
type AssertionOutcome string

const (
	OutcomeSatisfied AssertionOutcome = "satisfied"
	OutcomeRefuted   AssertionOutcome = "refuted"
	OutcomeAbstained AssertionOutcome = "abstained"
)

// ResolutionAssertion is a falsifiable claim whose satisfaction resolves a
// Tidal investigation. At least one is required (enforced starting in a
// later phase, once something actually constructs these — this phase only
// defines the type). Multiple form a disjunction: satisfying any one ends
// the investigation. Declared distinguishes criteria stated up front
// ("initial") from any added mid-investigation ("added round N"), so
// goalpost-movement is auditable rather than silent.
type ResolutionAssertion struct {
	Text     string           `json:"text"`
	Outcome  AssertionOutcome `json:"outcome,omitempty"`
	Evidence []Evidence       `json:"evidence,omitempty"`
	Declared string           `json:"declared"`
}
