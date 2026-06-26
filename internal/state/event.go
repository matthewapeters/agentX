package state

import "fmt"

// ContentType enumerates the persisted event content types
// (docs/architecture/runtime_contracts/event-envelope.schema.json).
type ContentType string

const (
	ContentUserPrompt      ContentType = "user_prompt"
	ContentSystemPrompt    ContentType = "system_prompt"
	ContentThinking        ContentType = "thinking"
	ContentAgentResponse   ContentType = "agent_response"
	ContentAttachments     ContentType = "attachments"
	ContentToolCall        ContentType = "tool_call"
	ContentToolResult      ContentType = "tool_result"
	ContentProcessingState ContentType = "processing_state"
)

var validContentTypes = map[ContentType]bool{
	ContentUserPrompt: true, ContentSystemPrompt: true, ContentThinking: true,
	ContentAgentResponse: true, ContentAttachments: true, ContentToolCall: true,
	ContentToolResult: true, ContentProcessingState: true,
}

// Event is the canonical session-event envelope fanned out over the Bus and
// persisted to disk. Field names/json tags mirror the frozen event-envelope schema.
type Event struct {
	Epoch         int64       `json:"epoch"`
	SessionID     string      `json:"session_id"`
	EventType     string      `json:"event_type"`
	ContentType   ContentType `json:"content_type"`
	Payload       any         `json:"payload"`
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
