# ADR 0014 - TIDAL Continue Investigating (Phase 3)

- **Status:** Implemented
- **Date:** 2024-01-15
- **Decisions:** This section codifies the `tidal.Wrapper` behavior: the `Run()` contract, error mapping, and the status struct shape.

---

## Context

TIDAL's scheduler (`wavefront.Scheduler`) is the execution engine. The runtime exposes it through `tidal.Wrapper`, a zero-arg native-tool wrapper. This document specifies how `Run()` maps scheduler outcomes to `RunStatus`, ensuring phase 5's stall-detection gate receives a clean, uniform signal.

## Decision

1. **`tidal.Status`** — lightweight status struct:
   ```go
   type Status struct {
       NodesResolved int       // count of terminal nodes after Run completes
       Status        RunStatus // one of Done / Stalled / Error
       Error         string    // non-empty only when Status == RunStatusError
       Snapshot      string    // non-empty only when snapshotFn was provided
   }
   ```

2. **`tidal.RunStatus`** — three states, `int` with iota:
   ```go
   type RunStatus int
   const (
       RunStatusDone  RunStatus = iota // scheduler.Run() returned nil
       RunStatusStalled                // scheduler.Run() returned scheduler.ErrStalled
       RunStatusError                  // scheduler.Run() returned some other error
   )
   ```

3. **`tidal.Wrapper.Run()`** mapping:
   - `scheduler.ErrStalled` → `RunStatusStalled`
   - `nil` → `RunStatusDone`
   - any other error → `RunStatusError` (with `err.Error()` captured in `Status.Error`)

4. **`NodesResolved`** — counts *total terminal nodes after Run*, not delta. Simpler for both wrapper and downstream gate. Terminal = Done / Failed / Denied / Cancelled.

5. **`SnapshotFunc`** — optional `func() string` for full five-section render vs. minimal summary. `nil` ⇒ no snapshot captured.

## Design Deferred to Phase 6

- Tool input parameters (zero-arg assumed for phase 3, extensible).
- Model-facing rendering of `Status` (phase 6's concern).

## Consequences

- **Stall detection:** Phase 5 reads `RunStatusStalled` and can trigger user notification or escalation.
- **Error isolation:** `RunStatusError` lets the runtime distinguish scheduler failures from stalls.
- **Snapshot flexibility:** Optional render function allows phase 5 to request full output or skip it.
- **No new scheduler methods:** The wrapper holds the graph separately so it can count terminal nodes without adding an accessor to `wavefront.Scheduler`.

## Scenario 3.1: Immediate Done
**Given** an empty graph (scheduler has nothing to do)  
**When** `Run()` is called  
**Then** returns `RunStatusDone`  
**And** `NodesResolved == 0`

## Scenario 3.2: All Terminal Before Run
**Given** a graph where every node is already in a terminal status (Done, Failed, Denied, Cancelled)  
**When** `Run()` is called  
**Then** returns `RunStatusDone`  
**And** `NodesResolved` equals the graph's node count

## Scenario 3.3: Mixed Terminal and Open
**Given** a graph with some Done nodes and some open nodes (Proposed, Ready)  
**When** `Run()` is called  
**Then** scheduler drives open nodes to terminal state  
**And** returns `RunStatusDone`  
**And** `NodesResolved` equals total node count

## Scenario 3.4: Cancelled Context
**Given** an already-cancelled context  
**When** `Run()` is called  
**Then** returns `RunStatusError`  
**And** `Error` contains "context" (from `ctx.Err()`)

## Scenario 3.5: Nil Snapshot
**Given** a wrapper with `SnapshotFunc == nil`  
**When** `Run()` is called  
**Then** snapshotFn is not invoked  
**And** `Status.Snapshot == ""`

## Scenario 3.6: Snapshot Captured
**Given** a wrapper with a non-nil `SnapshotFunc`  
**When** `Run()` is called  
**Then** snapshotFn is invoked after the run completes  
**And** `Status.Snapshot` equals the function's return value

## Scenario 3.7: Nil Graph
**Given** a wrapper with `graph == nil`  
**When** `Run()` is called  
**Then** `NodesResolved == 0` (countTerminal guards on nil)

## Scenario 3.8: Panic Propagation
**Given** a scheduler that panics during Run  
**When** `Run()` is called  
**Then** the panic propagates to the caller (no recovery in Wrapper)

## Scenario 3.9: RunStatus Constants
**Given** the `RunStatus` iota  
**Then** `RunStatusDone == 0`, `RunStatusStalled == 1`, `RunStatusError == 2`

---

## Phase 3 Build Plan (from ADR 0014 §1)

**Step 3: Implement `tidal.Wrapper` and `Run()`**
- [x] Create `tidal/status.go` — `Status` and `RunStatus` types
- [x] Create `tidal/continue_investigating.go` — `Wrapper` type and `Run()` method
- [x] Wire error mapping per decision #3

---

## Test Plan

### Test 3.1: Immediate Done
```go
func TestRun_Done_EmptyGraph(t *testing.T)
```

### Test 3.2: All Terminal Before Run
```go
func TestRun_Done_AllTerminal(t *testing.T)
```

### Test 3.3: Mixed Terminal and Open
```go
func TestRun_Done_MixedTerminalAndOpen(t *testing.T)
```

### Test 3.4: Cancelled Context
```go
func TestRun_Error_CancelledContext(t *testing.T)
```

### Test 3.5: Nil Snapshot
```go
func TestRun_Snapshot_NotCaptured(t *testing.T)
```

### Test 3.6: Snapshot Captured
```go
func TestRun_Snapshot_Captured(t *testing.T)
```

### Test 3.7: Nil Graph
```go
func TestRun_CountTerminal_NilGraph(t *testing.T)
```

### Test 3.8: Panic Propagation
```go
// Verified by design — no recover() in Wrapper. No scheduler mock available
// to force a panic; the contract is documented in Run()'s godoc.
```

### Test 3.9: RunStatus Constants
```go
func TestRunStatus_Constants(t *testing.T)
```
