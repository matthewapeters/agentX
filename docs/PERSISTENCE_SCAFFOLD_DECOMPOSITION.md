# Persistence-Ready Applet View-State Scaffold: Delivery Decomposition

**Date**: 2026-06-13  
**Status**: Frozen Architecture → Executable Slices  
**Owner**: Go staff programmer + SDET  
**Confidence**: High (frozen RFC + existing session patterns)  

---

## Executive Summary

Five **independently-compilable slices** decompose the VersionedAppletState persistence layer from frozen RFC into executable work. Each slice compiles, tests, and delivers independently; runtime reads both v0 and v1, upgrades on write.

**Delivery Sequencing**:

1. **Foundation** (Slice A) → **Adapters** (Slice B) → **Output Widget** (Slice C) → **Context Widget** (Slice D) → **Input Widget** (Slice E)
2. Slices A and B block everything; Slices C, D, E can parallelize after B completes.
3. User-visible value delivered at C (collapse/focus persistence).

**Estimated Scope**: ~3–4 days of focused development (2 days foundation + adapters, 1–2 days widget integration per slice).

---

## Slice A: Versioned Envelope Struct + JSON Marshaling

### Goal

Define the canonical `VersionedAppletState` struct, scope enums, and JSON marshaling contract. No runtime integration yet—foundation only.

### Scope

**Includes**:

- `VersionedAppletState` struct with Version, Widget, Scope, Payload, LastUpdateMs fields
- Scope enums: `APPLET_SCOPE_GLOBAL`, `APPLET_SCOPE_SESSION`
- JSON marshaling/unmarshaling tests (round-trip)
- Timestamp generation helper
- File-path builder for session persistence (e.g., `applet_state.json`)

**Excludes**:

- Migration adapters (Slice B)
- Widget runtime integration
- Config file I/O
- In-memory state cache

### Files to Create/Modify

**Create**:

- `cmd/agentx-core/internal/state/applet_state.go` (main struct + helpers)
- `cmd/agentx-core/internal/state/applet_state_test.go` (JSON round-trip tests)
- `cmd/agentx-core/internal/state/scope.go` (scope enum + formatters)

**Modify**:

- `cmd/agentx-core/config.go` (add `AppletStateDir(sessionID string) string` helper)

### Implementation Task for Go Staff Programmer

1. Define `VersionedAppletState` struct in `applet_state.go`:

   ```go
   type VersionedAppletState struct {
     Version      int       // 0 = index-based keys; 1 = semantic keys
     Widget       string    // "output" | "context" | "input"
     Scope        string    // "global" | "session:<uuid>"
     Payload      []byte    // JSON-marshaled state object
     LastUpdateMs int64     // millisecond timestamp
   }
   ```

2. Implement `NewVersionedAppletState(widget, scope string, payload []byte) *VersionedAppletState` constructor.

3. Implement `(v *VersionedAppletState) MarshalJSON() ([]byte, error)` and `UnmarshalJSON(data []byte) error`.

4. Add `ScopeForSession(sessionID string) string` formatter returning `"session:<sessionID>"`.

5. Add `AppletStateDir(sessionID string) string` to config helper that returns `Config.SessionDataDir(sessionID)` (defined in Slice A, used in Slice B+).

6. Write JSON round-trip tests:
   - Marshaling → unmarshaling preserves all fields
   - Payload bytes survive round-trip
   - Invalid JSON unmarshaling returns clear error
   - Scope formatter round-trips correctly

### Acceptance Criteria

1. ✅ `VersionedAppletState` struct compiles and has all 5 required fields
2. ✅ JSON marshaling/unmarshaling round-trip preserves all fields (3+ test cases)
3. ✅ `ScopeForSession()` returns correctly formatted string
4. ✅ Invalid JSON returns `ErrInvalidAppletState` (or similar)
5. ✅ `go test cmd/agentx-core/internal/state -v` passes 100%

### Dependencies + Risk

**Blocked by**: Nothing (foundation)  
**Enables**: Slices B, C, D, E  
**Breaks if wrong**:

- Bad marshaling → silent data loss
- Scope formatter bug → persistence written to wrong path
- Version field never used → breaks adapter routing logic

**Break-glass revert**: Delete `cmd/agentx-core/internal/state/` directory; Slices B–E roll back automatically (no dep outside state package yet).

### Expected Review Gate

**SDET focus areas**:

- Verify JSON struct tags are present and correct
- Test JSON invalid input error handling
- Test round-trip for all V1 fields

**Domain reviewer**: Application Architect (RFC alignment check only; struct is frozen)

---

## Slice B: v0↔v1 Migration Adapters + Round-Trip Logic

### Goal

Implement bidirectional adapters (`v0 map[string]interface{}` ↔ `VersionedAppletState v1`) and verify round-trip correctness. This is the bridge between old implicit format and new semantic format.

### Scope

**Includes**:

- `StateAdapter` interface: `ToV1(v0 map[string]interface{}) (VersionedAppletState, error)` and `FromV1(v1 VersionedAppletState) (map[string]interface{}, error)`
- Concrete adapters for output, context, and input widgets (separate implementations or factory)
- v0→v1 semantic key mapping (turn index → turn_id + session_id)
- Round-trip test suite: v0 → v1 → v0 preserves usable state
- Error handling for malformed v0 input

**Excludes**:

- Widget-specific rendering logic (that's Slices C–E)
- Runtime persistence I/O (that's Slices C–E)
- Versioning logic in the applet event loop (that's Slices C–E)

### Files to Create/Modify

**Create**:

- `cmd/agentx-core/internal/state/adapter.go` (StateAdapter interface + factory)
- `cmd/agentx-core/internal/state/adapter_v0_to_v1.go` (v0→v1 conversion logic)
- `cmd/agentx-core/internal/state/adapter_v1_to_v0.go` (v1→v0 conversion logic)
- `cmd/agentx-core/internal/state/adapter_test.go` (round-trip + edge cases)

**Modify**:

- `cmd/agentx-core/internal/state/applet_state.go` (add adapter factory method)

### Implementation Task for Go Staff Programmer

1. Define `StateAdapter` interface in `adapter.go`:

   ```go
   type StateAdapter interface {
     ToV1(ctx context.Context, v0 map[string]interface{}) (VersionedAppletState, error)
     FromV1(ctx context.Context, v1 VersionedAppletState) (map[string]interface{}, error)
   }
   ```

2. In `adapter_v0_to_v1.go`, implement v0→v1 for each widget type:
   - **Output widget v0 format**: Assume index-based keys like `"0"`, `"1"`, `"2"` for turns or entries.
   - Map to v1 stable keys: `"session_<id>:turn_<idx>:collapsed"`, `"session_<id>:turn_<idx>:focused_entry"`
   - Preserve boolean and array fields from v0
   - Return error if v0 is nil or missing required top-level keys

3. In `adapter_v1_to_v0.go`, implement reverse:
   - Extract semantic key parts (session_id, turn_id, entry_kind)
   - Rebuild index-based keys for compatibility
   - Return error if v1 Payload is malformed JSON

4. Add factory in `applet_state.go`:

   ```go
   func NewStateAdapter(widget string) (StateAdapter, error)
   ```

5. Write comprehensive adapter tests:
   - **Round-trip**: v0 → v1 → v0 preserves collapse/focus state
   - **Edge case**: Empty v0 map → v1 (should succeed with empty payload or sensible defaults)
   - **Error case**: Malformed v0 (missing keys) → error with clear message
   - **Error case**: Invalid v1 JSON → error in FromV1

### Acceptance Criteria

1. ✅ `StateAdapter` interface compiles and both methods are callable
2. ✅ Round-trip test (v0 → v1 → v0) preserves collapse/focus state (at least 3 test cases)
3. ✅ Malformed v0 returns `ErrInvalidV0Format` with descriptive message
4. ✅ Invalid v1 Payload JSON returns `ErrMalformedPayload`
5. ✅ `go test cmd/agentx-core/internal/state -v` passes 100% (including A + B)

### Dependencies + Risk

**Blocked by**: Slice A (VersionedAppletState struct)  
**Enables**: Slices C, D, E (widget integration)  
**Breaks if wrong**:

- Bad v0→v1 key mapping → focus state restored to wrong turn
- Silent data loss in round-trip (v0 field dropped) → user doesn't see persistence
- Error handling missing → panic on malformed old data instead of graceful fallback

**Break-glass revert**: Delete adapter files; Slice A remains. Slices C–E must defer to runtime version check to skip migration (add `if v1.Version != 1 { skip persistence }` fallback in C–E).

### Expected Review Gate

**SDET focus areas**:

- Verify all round-trip test cases exercise both directions
- Check error messages are actionable (not generic)
- Verify no data loss in v0→v1 conversion for all widget types

**Domain reviewer**: Application Architect (semantic key naming alignment)

---

## Slice C: Output Widget Persistence Integration

### Goal

Integrate VersionedAppletState persistence for output widget: read/restore collapse-state + focused turn on load; write on state change.

### Scope

**Includes**:

- Load applet state on output widget startup (read `applet_state.json`)
- Restore collapse state (`output_widget_state.Collapsed map[int]bool` or similar)
- Restore focused turn marker
- Write applet state on collapse/expand or focus-change action
- Graceful fallback if applet_state.json missing or unreadable
- Session-scoped persistence (per-session config path)

**Excludes**:

- Context or input widget integration
- Config file I/O (use existing session data path only)
- New rendering or command handling (only state read/write)

### Files to Create/Modify

**Create**:

- `cmd/agentx-core/internal/state/output_applet_state.go` (output widget state struct)
- `cmd/agentx-core/internal/output_widget_persistence_test.go` (integration tests)

**Modify**:

- `cmd/agentx-core/output_widget.go` (add state load/save calls)
- `cmd/agentx-core/widget_screen_models.go` (add persistence helpers to `OutputWidgetScreenState`)
- `cmd/agentx-core/output_widget_components.go` (add persistence call on collapse/expand)

### Implementation Task for Go Staff Programmer

1. Define output widget state struct in `output_applet_state.go`:

   ```go
   type OutputAppletState struct {
     CollapsedTurns map[int]bool `json:"collapsed_turns"`       // turn idx → collapsed
     FocusedTurnIdx int          `json:"focused_turn_idx"`      // -1 if none
     EntryFocusPath []string     `json:"entry_focus_path"`      // nested focus breadcrumb
   }
   ```

2. In `output_widget.go`, add to widget startup (`Init` or similar):
   - Call `LoadAppletState(sessionID, "output")` → `VersionedAppletState`
   - Unmarshal to `OutputAppletState` or adapt v0 using Slice B adapter
   - Restore collapsed state to widget screen model
   - Log non-fatal errors if file missing (first run)

3. Add to `OutputWidgetScreenState` in `widget_screen_models.go`:

   ```go
   func (o *OutputWidgetScreenState) SaveAppletState(sessionID string) error
   func (o *OutputWidgetScreenState) LoadAppletState(sessionID string) error
   ```

4. Hook in `output_widget_components.go` on collapse/expand action:
   - After state changes, call `SaveAppletState(sessionID)`
   - Include timestamp

5. Write integration tests in `output_widget_persistence_test.go`:
   - Load → modify collapse → save → reload verifies state persists
   - Session-scoped: load session A's state does not affect session B
   - Graceful fallback: missing applet_state.json on first run (no error)
   - v0→v1 migration: load old format, adapt, save as v1

### Acceptance Criteria

1. ✅ Output widget loads applet state on startup without error (first run: graceful fallback)
2. ✅ Collapse/expand action saves state with timestamp
3. ✅ Full cycle test (save → reload → verify state) passes
4. ✅ Session scoping verified (session A state ≠ session B state)
5. ✅ `go test cmd/agentx-core/output_widget_persistence_test.go -v` passes 100%

### Dependencies + Risk

**Blocked by**: Slices A + B  
**Enables**: Slices D, E (parallel OK)  
**Breaks if wrong**:

- Wrong session scope → focus state stomps between sessions
- Payload unmarshaling fails silently → collapse state not restored
- Save called on every render frame → excessive disk I/O

**Break-glass revert**: Remove persistence calls from output_widget.go; delete `output_applet_state.go`. Widget rendering unchanged.

### Expected Review Gate

**SDET focus areas**:

- Verify session scoping in test (separate sessions must not share state)
- Verify graceful fallback on missing file (test first-run scenario)
- Verify save only called on actual state change (not render frame)
- Check timestamp is set correctly

**Domain reviewer**: Output widget owner (UX impact check)

---

## Slice D: Context Widget Persistence Integration

### Goal

Integrate VersionedAppletState persistence for context widget: read/restore scroll position + sort filter on load; write on scroll/filter change.

### Scope

**Includes**:

- Load applet state on context widget startup
- Restore scroll viewport (row offset, focused row key)
- Restore sort filter (ascending/descending, sort key)
- Write applet state on scroll or filter-change action
- Graceful fallback if applet_state.json missing
- Session-scoped persistence

**Excludes**:

- Output or input widget integration
- New rendering or command handling
- History-specific logic (context-history model integration comes in follow-up)

### Files to Create/Modify

**Create**:

- `cmd/agentx-core/internal/state/context_applet_state.go` (context widget state struct)
- `cmd/agentx-core/internal/context_widget_persistence_test.go` (integration tests)

**Modify**:

- `cmd/agentx-core/context_widget.go` (add state load/save calls)
- `cmd/agentx-core/widget_screen_models.go` (add persistence to screen model, if applicable)
- `cmd/agentx-core/context_widget_components.go` or command handler (add persistence call on scroll/filter)

### Implementation Task for Go Staff Programmer

1. Define context widget state struct in `context_applet_state.go`:

   ```go
   type ContextAppletState struct {
     ScrollRowOffset   int    `json:"scroll_row_offset"`      // viewport top
     FocusedRowKey     string `json:"focused_row_key"`        // stable row key
     SortKey           string `json:"sort_key"`               // e.g., "timestamp" | "name"
     SortAscending     bool   `json:"sort_ascending"`
   }
   ```

2. In `context_widget.go`, add to startup:
   - Call `LoadAppletState(sessionID, "context")` → `VersionedAppletState`
   - Unmarshal to `ContextAppletState` or adapt v0
   - Restore scroll offset and focused row
   - Restore sort filter
   - Log non-fatal errors if file missing

3. Hook in context widget command handler on scroll:
   - After `j/k` or arrow actions, call `SaveAppletState(sessionID)`
   - Include timestamp

4. Hook in context widget command handler on filter/sort change:
   - After sort key or direction changes, call `SaveAppletState(sessionID)`

5. Write integration tests in `context_widget_persistence_test.go`:
   - Scroll → save → reload verifies scroll position persists
   - Sort filter → save → reload verifies filter persists
   - Session scoping verified

### Acceptance Criteria

1. ✅ Context widget loads applet state on startup without error
2. ✅ Scroll action saves state with correct row offset
3. ✅ Sort filter change saves state with correct filter key/direction
4. ✅ Full cycle test (save → reload → verify state) passes
5. ✅ `go test cmd/agentx-core/internal_context_widget_persistence_test.go -v` passes 100%

### Dependencies + Risk

**Blocked by**: Slices A + B  
**Enables**: Slice E (parallel OK)  
**Breaks if wrong**:

- Wrong row key → focus lost after reload
- Sort filter not restored → user sees default sort instead of saved preference
- Save called too frequently → excessive I/O

**Break-glass revert**: Remove persistence calls from context_widget.go; delete `context_applet_state.go`.

### Expected Review Gate

**SDET focus areas**:

- Verify row key stability (same row key after reload)
- Verify sort filter round-trip
- Verify scroll offset does not exceed content bounds on reload
- Check graceful fallback on malformed state

**Domain reviewer**: Context widget owner

---

## Slice E: Input Widget Persistence Skeleton

### Goal

Establish persistence contract for input widget (compose-box scroll, optional input history) without full implementation. Prepares for Phase 2 but does not block any current delivery.

### Scope

**Includes**:

- Skeleton struct `InputAppletState` (fields defined but not fully integrated)
- Adapter plumbing for v0→v1 (no-op implementation)
- Test stub (compile but not run)
- Documentation of Phase 2 scope

**Excludes**:

- Runtime integration (load/save on input widget startup)
- Input history collection
- Compose-box scroll restoration

### Files to Create/Modify

**Create**:

- `cmd/agentx-core/internal/state/input_applet_state.go` (skeleton struct)
- `cmd/agentx-core/internal/state/adapter_input.go` (v0→v1 no-op stub)
- `cmd/agentx-core/docs/INPUT_WIDGET_PERSISTENCE_PHASE2.md` (scope + next steps)

**Modify**:

- `cmd/agentx-core/internal/state/adapter.go` (add input case to factory)

### Implementation Task for Go Staff Programmer

1. Define skeleton struct in `input_applet_state.go`:

   ```go
   type InputAppletState struct {
     ComposeBoxScrollOffset int      `json:"compose_box_scroll_offset"`  // Phase 2
     RecentInputHistory     []string `json:"recent_input_history,omitempty"`  // Phase 2
     LastInputTimestamp     int64    `json:"last_input_timestamp"`
   }
   ```

2. In `adapter_input.go`, implement no-op adapters:

   ```go
   func (a *InputAdapter) ToV1(ctx context.Context, v0 map[string]interface{}) (VersionedAppletState, error) {
     // Return empty state; Phase 2 will populate
     return VersionedAppletState{Version: 1, Widget: "input"}, nil
   }
   ```

3. Add input case to factory in `adapter.go`.

4. Create test stub in `*_test.go` (compiles but skips actual assertions).

5. Document Phase 2 scope in `INPUT_WIDGET_PERSISTENCE_PHASE2.md`:
   - When to collect compose-box scroll
   - When to capture input history (on send? on change?)
   - How to avoid exposing sensitive input in history
   - Scope decision: per-session vs global

### Acceptance Criteria

1. ✅ `InputAppletState` struct compiles with all planned fields
2. ✅ Adapter factory recognizes "input" widget type
3. ✅ No-op adapters compile and return empty state without error
4. ✅ Test stubs compile (assertions can be skipped)
5. ✅ Phase 2 scope doc includes concrete decisions needed

### Dependencies + Risk

**Blocked by**: Nothing (skeleton)  
**Enables**: Phase 2 input persistence  
**Breaks if wrong**: None (not integrated yet)

**Break-glass revert**: Delete skeleton files; no user impact (never called at runtime).

### Expected Review Gate

**SDET focus areas**:

- Verify factory recognizes input widget type
- Verify skeleton compiles (no dead code)

**Domain reviewer**: Product (scope decisions for Phase 2)

---

## Cross-Slice Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Silent v0 format deserialize failure | Collapse state not restored; user thinks persistence is broken | Slice B: test malformed v0 → error; Slices C/D: log errors at WARN level |
| Scope string mismatch (session ID format) | Persistence written to wrong path; state lost or mixed between sessions | Slice A: define scope formatter once; use everywhere; Slices C/D: unit test session scoping |
| Adapter round-trip loses fields | User state incomplete after migration | Slice B: comprehensive round-trip tests covering all field types |
| Excessive I/O on save | Performance regression; visible lag in widget | Slices C/D: save only on actual state change (not per-frame); debounce if needed |
| File permission errors on write | Silent state loss; no warning to user | All slices: handle I/O errors, log at WARN, fall back to in-memory (no crash) |

---

## Integration Test Checklist (SDET)

After all slices complete, run full integration suite:

- [ ] Load session A output state, load session B output state → states isolated
- [ ] Collapse turn in session A, restart → collapse persists only in session A
- [ ] Modify context widget sort filter, restart → filter persists
- [ ] Run v0-to-v1 migration on real old-format applet_state.json → no data loss
- [ ] Stress: rapid collapse/scroll → no file corruption or crash
- [ ] Graceful: delete applet_state.json, restart → widget loads without error (empty state)
- [ ] Verify timestamps are correct and increase on each save

---

## Sequencing & Timeline Estimate

| Slice | Est. Days | Parallelizable | Owner |
|-------|-----------|-----------------|-------|
| A: Envelope + marshaling | 0.5 | – | Go programmer |
| B: Adapters + round-trip | 1 | After A | Go programmer |
| C: Output widget | 1 | After B | Go programmer |
| D: Context widget | 1 | After B (parallel with C) | Go programmer |
| E: Input skeleton | 0.25 | After B (parallel with C/D) | Go programmer |
| **Integration + SDET** | 0.5–1 | After C/D/E | SDET |
| **Total** | 3.5–4 days | 2.5 days critical path | – |

**Critical path**: A → B → C/D/E → Integration  
**Parallelization window**: After B, all three widget slices can proceed concurrently.

---

## Sign-Off Gates

Each slice requires:

1. ✅ Code compiles (`go build cmd/agentx-core`)
2. ✅ Tests pass (`go test ./... -v`)
3. ✅ No regressions in existing tests
4. ✅ Code review (Go best practices + domain expert review per slice)
5. ✅ SDET validates acceptance criteria

---

## Appendix: File Structure After All Slices

```
cmd/agentx-core/
├── internal/
│   └── state/
│       ├── applet_state.go                 (A)
│       ├── applet_state_test.go            (A)
│       ├── scope.go                        (A)
│       ├── adapter.go                      (B)
│       ├── adapter_v0_to_v1.go             (B)
│       ├── adapter_v1_to_v0.go             (B)
│       ├── adapter_test.go                 (B)
│       ├── output_applet_state.go          (C)
│       ├── context_applet_state.go         (D)
│       ├── input_applet_state.go           (E)
│       ├── adapter_input.go                (E)
│       └── [other state files]
├── output_widget.go                        (C: modified)
├── output_widget_components.go             (C: modified)
├── output_widget_persistence_test.go       (C)
├── context_widget.go                       (D: modified)
├── context_widget_components.go            (D: modified)
├── context_widget_persistence_test.go      (D)
├── input_widget.go                         (E: future)
├── widget_screen_models.go                 (C/D: modified)
├── config.go                               (A: modified)
└── [other existing files]

docs/
├── PERSISTENCE_SCAFFOLD_DECOMPOSITION.md  (this file)
└── INPUT_WIDGET_PERSISTENCE_PHASE2.md     (E)
```

---

## Success Criteria (Product-Level)

After all slices + integration:

1. **User opens session** → all widget view states restored (collapse, scroll, sort)
2. **User modifies state** → state persists across session restart
3. **Session v0 → v1 migration** → backward compatible, no data loss
4. **Performance** → no visible lag from persistence I/O
5. **Reliability** → graceful fallback if state file corrupted or missing
