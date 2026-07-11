package state

import "fmt"

// ContentType enumerates the persisted event content types
// (docs/architecture/runtime_contracts/event-envelope.schema.json).
type ContentType string

const (
	ContentUserPrompt      ContentType = "user_prompt"
	ContentSystemPrompt    ContentType = "system_prompt"
	ContentClassification  ContentType = "classification"
	ContentThinking        ContentType = "thinking"
	// ContentAgentDelta is a transient streaming chunk of the agent's answer,
	// published on the in-process bus for the chat window's live typing effect. It
	// is never persisted and never streamed to external surfaces; the complete
	// answer is emitted once as ContentAgentResponse.
	ContentAgentDelta      ContentType = "agent_delta"
	ContentAgentResponse   ContentType = "agent_response"
	ContentAttachments     ContentType = "attachments"
	ContentToolCall        ContentType = "tool_call"
	ContentToolResult      ContentType = "tool_result"
	ContentProcessingState ContentType = "processing_state"
	// ContentTaskProposed is a durable task record the classifier emits when a turn
	// is recognized as actionable (docs/architecture/task_record.md). It is not
	// conversation context; a future executor drains it.
	ContentTaskProposed ContentType = "task_proposed"
	// ContentTaskResult records the outcome of draining a task record through the
	// executor: executed (and verified) / phantom / denied / etc.
	ContentTaskResult ContentType = "task_result"
	// ContentTaskDiagnostic is a per-turn trace of the task classifier: the three
	// fan-group stage verdicts (triage / action / response), their vote spreads, and
	// the outcome or skip reason. It exists for observability — never conversation
	// context, never executed — so a skipped turn always leaves a legible reason.
	ContentTaskDiagnostic ContentType = "task_diagnostic"
	// ContentTaskPlan is the synthesis of a decomposed compound goal: the plan's node
	// set (goals + deps + final statuses) after the scheduler drained it. Emitted on the
	// Decompose route (ADR 0008 Phase 4); observability, never conversation context.
	ContentTaskPlan ContentType = "task_plan"
	// ContentTaskNode is a live per-node plan delta (ADR 0009 §9a): dispatched /
	// decomposed / completed, streamed as the scheduler drains so execution is
	// user-visible while it happens — never batched to completion.
	ContentTaskNode ContentType = "task_node"
	// ContentApprovalRequest announces a pending interactive decision — a tool call
	// awaiting approval, an unrecognized stated-continuation verb, or any future
	// decision kind — as a generic {prompt, options} pair. Observability, never
	// conversation context; the surface pairs it with StateAwaitingInput and renders
	// it as the swapped-in approval widget regardless of which kind requested it.
	ContentApprovalRequest ContentType = "approval_request"
	// ContentApprovalDecision records the outcome of a resolved ContentApprovalRequest:
	// the original prompt and the chosen option's label (as text, never the raw
	// decision key). Audit trail only — persisted like every other event, but never
	// conversation context.
	ContentApprovalDecision ContentType = "approval_decision"
)

var validContentTypes = map[ContentType]bool{
	ContentUserPrompt: true, ContentSystemPrompt: true, ContentClassification: true,
	ContentThinking: true, ContentAgentDelta: true, ContentAgentResponse: true,
	ContentAttachments: true, ContentToolCall: true, ContentToolResult: true,
	ContentProcessingState: true, ContentTaskProposed: true, ContentTaskResult: true,
	ContentTaskDiagnostic: true, ContentTaskPlan: true, ContentTaskNode: true,
	ContentApprovalRequest: true, ContentApprovalDecision: true,
}

// DefaultEnabled reports whether an event of the given content type participates
// in assembled LLM context by default. User and agent turns (and attachments) are
// on. Thinking events are retained but off and stay inert when toggled (never
// enter context). Tool-call/tool-result events are retained but off too, but
// toggling one on is meaningful: the context surface uses it to pin the element
// into every subsequent turn's context (docs/ux/03_PANEL_DETAILS.md PD-CTX-AF-011).
// Classification/system/processing-state events are not context.
func DefaultEnabled(ct ContentType) bool {
	switch ct {
	case ContentUserPrompt, ContentAgentResponse, ContentAttachments:
		return true
	default:
		return false
	}
}

// Event is the canonical session-event envelope fanned out over the Bus and
// persisted to disk. Field names/json tags mirror the frozen event-envelope schema.
type Event struct {
	Epoch         int64       `json:"epoch"`
	SessionID     string      `json:"session_id"`
	EventType     string      `json:"event_type"`
	ContentType   ContentType `json:"content_type"`
	Payload       any         `json:"payload"`
	Enabled       bool        `json:"enabled"`
	// Ordinal is a per-session monotonic sequence stamped by the Bus at publish
	// time (1-based; 0 means unstamped). It is the canonical total order and the
	// resume cursor for surface attach (seed-then-subscribe). Unlike the recorder's
	// filename seq, it travels on the envelope so live bus events and their
	// persisted files share one identity.
	Ordinal       uint64      `json:"ordinal,omitempty"`
	// Ephemeral marks an event that engages the session but is not part of the
	// user's conversation (e.g. the startup bootstrap exchange). The chat surface
	// still shows it; read-only observers like the context viewer omit it.
	Ephemeral     bool        `json:"ephemeral,omitempty"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	ParentEventID string      `json:"parent_event_id,omitempty"`
	SurfaceID     string      `json:"surface_id,omitempty"`
	ToolName      string      `json:"tool_name,omitempty"`
	ModelName     string      `json:"model_name,omitempty"`
}

// Validate checks the required envelope fields per the frozen contract.
func (e Event) Validate() error {
	if e.Epoch < 0 {
		return fmt.Errorf("event epoch must be >= 0")
	}
	if e.SessionID == "" {
		return fmt.Errorf("event session_id is required")
	}
	if e.EventType == "" {
		return fmt.Errorf("event event_type is required")
	}
	if !validContentTypes[e.ContentType] {
		return fmt.Errorf("event content_type %q is not a known type", e.ContentType)
	}
	if e.Payload == nil {
		return fmt.Errorf("event payload is required")
	}
	return nil
}
