package logs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agentx/internal/state"
)

// formatEvent renders one state.Event as a single logical (pre-wrap) line: a
// millisecond timestamp, the content type, the tool name when set, and a
// summary of the payload. The caller wraps this to the pane width — no
// truncation happens here, since PD-LOGS-AF-006 requires the full content to
// stay visible via wrapping rather than being clipped.
func formatEvent(ev state.Event) string {
	var b strings.Builder
	b.WriteString(time.UnixMilli(ev.Epoch).Format("15:04:05.000"))
	b.WriteString("  ")
	fmt.Fprintf(&b, "%-18s", string(ev.ContentType))
	if ev.ToolName != "" {
		fmt.Fprintf(&b, "  [%s]", ev.ToolName)
	}
	if summary := summarizePayload(ev.Payload); summary != "" {
		b.WriteString("  ")
		b.WriteString(summary)
	}
	return b.String()
}

// summarizePayload renders an event payload as compact JSON so the whole
// event stays one logical line before wrapping. A raw string payload (a
// tool's textual output, most commonly) renders unquoted for readability
// instead of round-tripping through JSON. Falls back to a %v dump if the
// payload isn't JSON-marshalable — a persisted event shouldn't hit this, but
// a read-only viewer must never panic on unexpected data.
func summarizePayload(payload any) string {
	if payload == nil {
		return ""
	}
	if s, ok := payload.(string); ok {
		return s
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%v", payload)
	}
	return string(b)
}
