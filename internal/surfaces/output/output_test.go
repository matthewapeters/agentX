package output

import (
	"strings"
	"testing"

	"agentx/internal/state"
)

// GIVEN a widget body containing tab-indented content (real Go source, the
// case that reached a real session's approval prompt) at a variety of
// realistic panel sizes
// WHEN the panel is applied several such widgets and rendered
// THEN View()'s row count never exceeds the panel's configured height —
// the confirmed overflow (a tab-containing line ansi.StringWidth measured
// as fitting exactly still got soft-wrapped an extra row by lipgloss's own
// renderer) closed by expanding tabs before any width measurement
// (docs/architecture/behavior/scrollutil_tab_width_disagreement.feature.md).
func TestViewNeverExceedsHeightWithTabContent(t *testing.T) {
	tabBody := "write_file content=package task\n\nimport (\n\t\"encoding/json\"\n\n\t\"testing\"\n)\n\nfunc T… path=internal/prompting/task/hypothesis_test.go"

	for _, size := range []struct{ w, h int }{
		{100, 30}, {80, 24}, {60, 15}, {60, 10}, {40, 8},
	} {
		m := New()
		m.SetSize(size.w, size.h)

		for i := 0; i < 15; i++ {
			m.Apply(state.Event{EventType: "TOOL_CALL", ContentType: state.ContentApprovalRequest,
				Payload: map[string]any{"prompt": tabBody}, Enabled: true})
			m.Apply(state.Event{EventType: "APPROVAL_DECISION", ContentType: state.ContentApprovalDecision,
				Payload: map[string]any{"prompt": tabBody, "chosen_label": "Approve for this session"}, Enabled: true})
			m.Apply(state.Event{EventType: "TOOL_RESULT", ContentType: state.ContentToolResult, ToolName: "write_file",
				Payload: map[string]any{"text": ""}, Enabled: true})
		}
		m.Apply(state.Event{EventType: "AGENT_RESPONSE", ContentType: state.ContentAgentResponse,
			Payload: map[string]any{"text": "[stopped: reached the tool-call limit for this turn without a final answer]"}, Enabled: true})

		rows := strings.Split(m.View(), "\n")
		if len(rows) > size.h {
			t.Errorf("size %dx%d: View() produced %d rows, want <= %d", size.w, size.h, len(rows), size.h)
		}
	}
}
