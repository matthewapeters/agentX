package tidal

// RunStatus is the wrapper's structured signal — the stall-detection gate
// branches on this, never on raw error text.
type RunStatus int

const (
	RunStatusDone  RunStatus = iota // scheduler.Run() returned nil
	RunStatusStalled                // scheduler.Run() returned scheduler.ErrStalled
	RunStatusError                  // scheduler.Run() returned some other error
)

// Status is the structured return from Wrapper.Run — consumed by Tier 2's
// stall-detection gate and (optionally) surfaced to the model as the tool's
// output in a later phase.
type Status struct {
	NodesResolved int       // count of terminal nodes after Run completes
	Status        RunStatus // one of Done / Stalled / Error
	Error         string    // non-empty only when Status == RunStatusError
	Snapshot      string    // non-empty only when snapshotFn was provided
}
