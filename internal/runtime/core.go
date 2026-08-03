package runtime

import (
	"context"
	"time"

	"agentx/internal/prompting"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// ConversationCore is the seam ADR 0013 (docs/architecture/adr/0013-conversationcore-extraction.md)
// extracts the prompt/tool/hook loop behind. Phase 1 introduces only the three
// interfaces below and Orchestrator's implementations of them — nothing in
// loop.go/tool_cycle.go depends on these yet, so this phase is additive and
// behavior-preserving by construction. A ConversationCore type itself, and the
// move of runPrompt's body onto it, is Phase 2.

// ApprovalSeeker requests a policy decision for a proposed tool call or an
// oversized-output recovery choice. Orchestrator satisfies this with its existing
// RequestApproval/RequestOutputSizeDecision methods unchanged — a future nested
// loop's implementation is an open question (ADR 0013 §Open Questions), not decided
// here.
type ApprovalSeeker interface {
	RequestApproval(ctx context.Context, d tools.Descriptor, args map[string]string, pol *tools.Policy, root string) (tools.Verdict, error)
	RequestOutputSizeDecision(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result) (tools.Result, bool, error)
}

// EventSink records this loop's events. Orchestrator's implementation is a pure
// pass-through to publishEv (the live, transport-visible bus). A future nested
// loop's implementation is the ConversationCore-shaped equivalent of
// branch.Branch's Emit/Events — an isolated, in-memory log, never published to the
// parent bus — per ADR 0013 §Open Questions; not decided here.
type EventSink interface {
	Publish(eventType string, ct state.ContentType, payload any, ephemeral bool) uint64
	// PublishTool records a tool_call/tool_result-shaped event — distinct from
	// Publish because these carry a ToolName the generic event shape doesn't.
	// Added in Phase 3 when runNativeToolCall's move onto ConversationCore
	// surfaced that Orchestrator's pre-extraction publishToolCall/
	// publishToolResult never went through publishEv/Publish at all — they
	// called o.bus.Publish directly with an extra field. Orchestrator's
	// implementation preserves that pre-extraction behavior exactly, including
	// stamping ModelName from settings.OllamaModel rather than the dynamic
	// modelName() every other event uses — a pre-existing discrepancy, not
	// corrected here (ADR 0013 is an extraction, not a bugfix pass).
	PublishTool(evt ToolEvent) uint64
}

// ToolEvent is PublishTool's payload — a tool_call/tool_result event.
type ToolEvent struct {
	EventType   string
	ContentType state.ContentType
	ToolName    string
	Payload     map[string]any
}

// TurnRecord carries recordTurn's existing parameters as one value, so
// ContextStore.Record has a single-argument shape. Field-for-field, this is
// recordTurn's current parameter list (orchestrator.go) unpacked into a struct —
// no new semantics.
type TurnRecord struct {
	Err      error
	Record   bool
	UserOrd  uint64
	UserText string
	RespOrd  uint64
	Response string
	Pins     []*toolPin
}

// ContextStore supplies this loop's next-call context and records what happened.
// Orchestrator's implementation is a pure pass-through to withContext/recordTurn
// (the session's on-disk working memory plus its in-process history slice). A
// future nested loop's implementation is the conversational analogue of
// branch.Branch's Facts()/SetLocalFact/LocalWM split — per ADR 0013 §Open
// Questions; not decided here.
type ContextStore interface {
	Augment(base []prompting.Message) []prompting.Message
	Record(entry TurnRecord)
}

// Compile-time proof Orchestrator satisfies all three seams.
var (
	_ ApprovalSeeker = (*Orchestrator)(nil)
	_ EventSink      = (*Orchestrator)(nil)
	_ ContextStore   = (*Orchestrator)(nil)
)

// Publish satisfies EventSink as a pure pass-through to publishEv. Unused by the
// existing loop as of Phase 1 — loop.go/tool_cycle.go keep calling publishEv
// directly until Phase 3 redirects them.
func (o *Orchestrator) Publish(eventType string, ct state.ContentType, payload any, ephemeral bool) uint64 {
	return o.publishEv(eventType, ct, payload, ephemeral)
}

// PublishTool satisfies EventSink's tool-event method, reproducing the
// pre-extraction publishToolCall/publishToolResult behavior exactly (see
// EventSink's doc comment for the ModelName discrepancy this preserves rather
// than fixes).
func (o *Orchestrator) PublishTool(evt ToolEvent) uint64 {
	return o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   evt.EventType,
		ContentType: evt.ContentType,
		ToolName:    evt.ToolName,
		Payload:     evt.Payload,
		Enabled:     state.DefaultEnabled(evt.ContentType),
		ModelName:   o.settings.OllamaModel,
	})
}

// Augment and Record satisfy ContextStore — see core_context.go, co-located
// with withContext/recordTurn and everything they depend on (ADR 0013 Phase 4).
