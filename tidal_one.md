Implement Phase 1 of ADR 0014 (Tidal), following docs/architecture/behavior/adr/0014_tidal_schema.feature.md exactly. Read that file and docs/architecture/adr/0014-tidal-hypothesis-grounded-consolidation.md's Phase 1 build-plan entry first. This phase is schema-only -- do not implement anything beyond what the behavior doc's GIVEN/WHEN/THEN scenarios require. make all must pass when you're done.

## Implementation Summary

### Files Created/Modified:
1. **`internal/prompting/task/hypothesis.go`** (NEW) — Added `Likelihood`, `Stance`, `Evidence`, `AssertionOutcome`, and `ResolutionAssertion` types
2. **`internal/prompting/task/hypothesis_test.go`** (NEW) — Tests for schema types and JSON round-trip
3. **`internal/prompting/task/task.go`** (MODIFIED) — Added `KindHypothesis` to Kind constants, added `Likelihood`, `Evidence`, `ResolutionCriteria`, and `Deferred` fields to Record
4. **`internal/prompting/task/graph.go`** (MODIFIED) — Added `KindHypothesis` exclusion check in `Ready()`
5. **`internal/prompting/task/graph_test.go`** (MODIFIED) — Extended with 5 Ready() scenarios from behavior doc

### Key Changes:
- ✅ `KindHypothesis Kind = \"hypothesis\"` added alongside KindTask/KindStep
- ✅ `Likelihood` type with H/M/L/I constants (LLM judgment only, never fact)
- ✅ `Stance` type with StanceSupports/StanceRefutes (required on every Evidence link)
- ✅ `Evidence` struct with NodeID and Stance (separate from Deps — retrospective, non-blocking)
- ✅ `AssertionOutcome` type mirroring ADR 0010 (satisfied/refuted/abstained)
- ✅ `ResolutionAssertion` struct with Text, Outcome, Evidence, Declared
- ✅ `Record` gains 4 additive fields: Likelihood, Evidence, ResolutionCriteria, Deferred
- ✅ `Graph.Ready()` excludes KindHypothesis — the hazard fix that prevents scheduler dispatch to hypothesis nodes

### Hazard Fixed:
`Graph.Ready()` previously filtered by Status only, not Kind. A KindHypothesis record with Status Proposed/Ready and no Deps would be handed to schedulers' dispatch switches, hitting `default: doneError(\"node has no valid Kind\")`. The fix adds a Kind check at the top of Ready() loop body — the one shared read seam both schedulers use.

## Next Step:

Run `make all` to verify the implementation compiles and passes all tests.

## Implementation Summary:
```bash
make all
```

This runs `go vet ./...` and `go test ./...` which validates:
- Schema types compile and work correctly
- JSON round-tripping for new fields
- Ready() exclusion behavior (5 scenarios from behavior doc)
- Existing tests pass unchanged (additive only)
- No regressions in scheduler/wavefront suites
