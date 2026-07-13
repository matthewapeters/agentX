package state

import (
	"fmt"
	"sync"
)

// RunState is the coarse processing state of a session.
type RunState string

const (
	StateIdle      RunState = "idle"
	StateWorking   RunState = "working"
	StateCompleted RunState = "completed"
	StateFailed    RunState = "failed"
	// StateAwaitingInput pauses the cycle for a user decision (e.g. tool approval
	// or Stage-2 clarification). The runtime blocks until the surface resolves it.
	StateAwaitingInput RunState = "awaiting_input"
)

// Phase is the fine-grained prompt-cycle phase.
type Phase string

const (
	PhaseClassify Phase = "classify"
	PhaseThinking Phase = "thinking"
	PhaseTool     Phase = "tool"
	PhaseRespond  Phase = "respond"
	// PhasePlanning is the plan-cycle phase (ADR 0009): the scheduler is decomposing and
	// executing a plan, so the surface shows life during multi-step work.
	PhasePlanning Phase = "planning"
	// PhaseVerb pauses the cycle in StateAwaitingInput for a verb-continuation decision
	// (an unrecognized "Let me <verb>..." stated intent, neither allow- nor
	// deny-listed) — distinct from PhaseTool so the surface can show the right
	// hint/keymap (4-way: allow once/always, deny once/always) instead of the
	// 3-way tool-approval one.
	PhaseVerb Phase = "verb"
	// PhaseOutputSize pauses the cycle in StateAwaitingInput for an oversized-tool-
	// output decision (TOOL-6): a tool result was truncated by the output_max_bytes
	// safety net and no remembered per-tool preference applies — distinct from
	// PhaseTool since this is a post-execution decision (the call already ran),
	// not a pre-execution approval, and offers its own option set.
	PhaseOutputSize Phase = "output_size"
	PhaseNone       Phase = "none"
)

var (
	validStates = map[RunState]bool{StateIdle: true, StateWorking: true, StateCompleted: true, StateFailed: true, StateAwaitingInput: true}
	validPhases = map[Phase]bool{PhaseClassify: true, PhaseThinking: true, PhaseTool: true, PhaseRespond: true, PhasePlanning: true, PhaseVerb: true, PhaseOutputSize: true, PhaseNone: true}
)

// ProcessingState is the session-level processing-state payload
// (docs/architecture/runtime_contracts/processing-state.schema.json).
type ProcessingState struct {
	SessionID   string         `json:"session_id"`
	State       RunState       `json:"state"`
	Phase       Phase          `json:"phase"`
	PromptCycle map[string]any `json:"prompt_cycle"`
}

// Validate checks the processing-state fields against the frozen contract.
func (p ProcessingState) Validate() error {
	if p.SessionID == "" {
		return fmt.Errorf("processing-state session_id is required")
	}
	if !validStates[p.State] {
		return fmt.Errorf("processing-state state %q is invalid", p.State)
	}
	if !validPhases[p.Phase] {
		return fmt.Errorf("processing-state phase %q is invalid", p.Phase)
	}
	if p.PromptCycle == nil {
		return fmt.Errorf("processing-state prompt_cycle is required")
	}
	return nil
}

// ProcessingPublisher holds the latest processing state for a session and
// broadcasts changes to subscribers. Updates are low-frequency and coalescing:
// a subscriber always observes the most recent state even if it lags.
type ProcessingPublisher struct {
	mu   sync.Mutex
	cur  ProcessingState
	subs map[int]chan ProcessingState
	next int
}

// NewProcessingPublisher returns a publisher seeded to idle/none for the session.
func NewProcessingPublisher(sessionID string) *ProcessingPublisher {
	return &ProcessingPublisher{
		cur: ProcessingState{
			SessionID:   sessionID,
			State:       StateIdle,
			Phase:       PhaseNone,
			PromptCycle: map[string]any{},
		},
		subs: make(map[int]chan ProcessingState),
	}
}

// Current returns the latest processing state.
func (p *ProcessingPublisher) Current() ProcessingState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cur
}

// Set updates the state/phase and broadcasts to subscribers.
func (p *ProcessingPublisher) Set(state RunState, phase Phase) {
	p.mu.Lock()
	p.cur.State = state
	p.cur.Phase = phase
	cur := p.cur
	chans := make([]chan ProcessingState, 0, len(p.subs))
	for _, ch := range p.subs {
		chans = append(chans, ch)
	}
	p.mu.Unlock()

	for _, ch := range chans {
		coalesceSend(ch, cur)
	}
}

// Subscribe returns a channel that immediately yields the current state and then
// receives subsequent updates, plus a cancel function.
func (p *ProcessingPublisher) Subscribe() (<-chan ProcessingState, func()) {
	p.mu.Lock()
	id := p.next
	p.next++
	ch := make(chan ProcessingState, 1)
	ch <- p.cur
	p.subs[id] = ch
	p.mu.Unlock()

	return ch, func() {
		p.mu.Lock()
		delete(p.subs, id)
		p.mu.Unlock()
	}
}

// coalesceSend delivers v to ch, replacing any pending value so the consumer
// always sees the latest state without the publisher blocking.
func coalesceSend(ch chan ProcessingState, v ProcessingState) {
	select {
	case ch <- v:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- v:
		default:
		}
	}
}
