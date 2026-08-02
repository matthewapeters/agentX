# Behavior — Tidal Schema: Hypothesis Kind, Likelihood, Evidence, Resolution Criteria (ADR 0014 Phase 1)

Status: Not started. Realizes ADR 0014's Phased Build Plan step 1. This is the
first phase of an unimplemented design (ADR 0014, Status: Proposed) — no Tidal
code exists yet anywhere in the tree. This phase only makes the schema exist and
behave correctly in isolation; no Tier 1/Tier 2 logic, no render function, no
hook. Everything in this phase is additive to already-shipped, heavily-used
types (`task.Record`, `task.Graph`) — the existing `internal/prompting/task`,
`internal/runtime/scheduler`, and `internal/runtime/wavefront` suites must pass
unchanged when this phase is done.

## Problem

ADR 0014 needs three schema additions: a third `Kind` (`Hypothesis`, alongside
the existing `Task`/`Step`) with a `Likelihood` and evidence links; a
`Stance`-tagged `Evidence` type shared by both hypotheses and resolution
criteria; and a `ResolutionAssertion` type for the problem's disjunctive
success criteria. All three need to land in `internal/prompting/task`, the same
package `Kind`/`Status`/`Provenance`/`Record` already live in, since this is
shared substrate both a future Tidal engine and the existing schedulers read.

**A real hazard, found by checking before writing this phase, not assumed:**
`task.Graph.Ready()` (`internal/prompting/task/graph.go:177-190`) — the read-only
seam both `internal/runtime/scheduler/scheduler.go:250` and
`internal/runtime/wavefront/scheduler.go:213` dispatch from — filters candidates
by `Status` only (`Proposed`/`Ready` with all deps `Done`). It does **not**
filter by `Kind`. Both schedulers' dispatch switches are `case KindTask: ...;
case KindStep: ...; default: doneError("node has no valid Kind")`. If a
`KindHypothesis` record ever sits in the same graph one of these schedulers is
actively dispatching, with a `Status` `Ready()` would select, it gets handed to
that switch and reported as a broken node — a real, live bug, not a
hypothetical one. `KindHypothesis` records are never meant to be
scheduler-dispatched at all — they're written directly by Tidal's own Tier 2
logic (later phases), never discovered via `Ready()`. This phase must close that
gap structurally, at the graph level (the one place both schedulers already
read from), not by asking each scheduler to remember a defensive case.

## Design

### 1. `Kind` gains a third value

`internal/prompting/task/task.go`, in the existing `const` block alongside
`KindStep`/`KindTask`:

```go
// KindHypothesis is a candidate explanation tracked with a Likelihood and
// Evidence links — never scheduler-dispatched (see Graph.Ready()'s exclusion,
// below). Written and read by Tidal (ADR 0014), never by the schedulers in
// internal/runtime/scheduler or internal/runtime/wavefront.
KindHypothesis Kind = "hypothesis"
```

### 2. New file: `internal/prompting/task/hypothesis.go`

A new file, not appended to `task.go` — this is Tidal-specific vocabulary, kept
separate from the Task/Step-oriented types `task.go` already holds, the same way
`graph.go` is already its own file in this package.

```go
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
```

### 3. `Record` gains three additive fields

`internal/prompting/task/task.go`, appended to the existing `Record` struct
(after `Seq`, following that field's own precedent of being introduced as a
purely additive amendment in ADR 0012's phase):

```go
// Likelihood and Evidence are meaningful only when Kind == KindHypothesis —
// zero-valued and ignored otherwise. Set by Tidal (ADR 0014), never by
// either scheduler.
Likelihood Likelihood `json:"likelihood,omitempty"`
Evidence   []Evidence `json:"evidence,omitempty"`

// ResolutionCriteria is meaningful only on a Tidal investigation's root
// record — the disjunctive set of ResolutionAssertions whose satisfaction
// ends the investigation. Empty/nil on every other record.
ResolutionCriteria []ResolutionAssertion `json:"resolution_criteria,omitempty"`

// Deferred marks a Need-to-Know-shaped open question (Status other than
// Done/Failed/Denied/Cancelled) as adjacent context rather than required to
// resolve the investigation's ResolutionCriteria — the diagnostic/deferred
// split a Tidal render groups by. False (diagnostic) is the default for
// every existing record kind; irrelevant once a node resolves.
Deferred bool `json:"deferred,omitempty"`
```

### 4. `Graph.Ready()` excludes `KindHypothesis` — the hazard's fix

`internal/prompting/task/graph.go`, inside the existing loop body (after the
`Status` check, before the deps-satisfied check — excluding on `Kind` is cheaper
than walking `Deps`, so check it first):

```go
func (g *Graph) Ready() []string {
	var out []string
	for _, id := range g.order {
		rec := g.nodes[id]
		if rec.Kind == KindHypothesis {
			continue // never scheduler-dispatched — see this file's package doc / ADR 0014
		}
		if rec.Status != Proposed && rec.Status != Ready {
			continue
		}
		// ... unchanged from here
	}
}
```

```
GIVEN a graph containing only KindTask/KindStep records (today's existing
      shape, no Hypothesis records at all)
WHEN  Ready() is called
THEN  its output is byte-for-byte identical to what it returns today — this
      phase changes nothing observable for the continuous scheduler or
      wavefront's scheduler.

GIVEN a graph containing a KindHypothesis record with Status: Proposed and no
      Deps (the exact shape that would make it schedulable under today's
      Ready() logic, if Kind weren't excluded)
WHEN  Ready() is called
THEN  that record's ID is absent from the returned slice, regardless of its
      Status — this is the hazard's regression test, and it must fail loudly
      if the exclusion is ever removed or reordered incorrectly.

GIVEN a graph containing a mix of KindTask, KindStep, and KindHypothesis
      records, all with a Status/Deps shape that would otherwise qualify
WHEN  Ready() is called
THEN  only the KindTask/KindStep IDs are returned, in the same first-seen
      order Ready() already guarantees — the exclusion doesn't disturb
      ordering for the records that do qualify.

GIVEN a Record constructed with Likelihood/Evidence/ResolutionCriteria/Deferred
      populated
WHEN  it is passed to Graph.Add and then Graph.Update
THEN  both succeed exactly as they do for a Record without these fields set —
      Add/Update's existing validate() (dep integrity, cycle check) never
      inspects Kind or these new fields, so nothing about their happy-path
      behavior changes. (Grounded directly in reading validate()'s current
      body — it only ever inspects rec.Deps and rec.ID.)

GIVEN a Record with Kind == KindHypothesis and a populated Evidence list
      referencing another node's ID by NodeID
WHEN  the graph is serialized to JSON and read back
THEN  every new field round-trips (json tags above use snake_case, matching
      every existing Record field's convention — Value/Error/Seq etc.).
```

## Tests

- `internal/prompting/task/hypothesis_test.go` (new) — constructs `Likelihood`/
  `Stance`/`Evidence`/`AssertionOutcome`/`ResolutionAssertion` values directly;
  no behavior to test beyond "the types exist, zero values are sane, JSON
  round-trips" — this file exists mainly so the schema has a home for future
  tests to extend, not because these plain value types need heavy coverage now.
- `internal/prompting/task/graph_test.go` (extended, not replaced) — the five
  `Ready()` scenarios above, added alongside whatever `Ready()` tests already
  exist there. **Do not modify or remove any existing test in this file** — this
  phase is purely additive to a heavily-depended-on function.
- Full existing suite must pass unchanged: `go test ./internal/prompting/task/...
  ./internal/runtime/scheduler/... ./internal/runtime/wavefront/... ./...`
  (the whole repo, since `Record`/`Graph` are imported broadly) plus `go vet
  ./...` and `gofmt -l` clean.
- `make all` must pass before this phase is considered done — the repo's own
  stated invariant, not optional for this phase specifically.

## Explicitly out of scope for this phase

No render function, no `internal/runtime/tidal` package, no LLM prompts, no
hook, no tool. This phase makes four types and one graph-safety fix exist and
behave correctly in total isolation — nothing else. Do not anticipate later
phases' needs by adding fields or functions this phase's own GIVEN/WHEN/THEN
scenarios don't require; ADR 0014's later phases will each get their own
behavior doc, written immediately before that phase starts, per repo
convention.
