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

// GIVEN a ContentApprovalRequest event with a multi-line prompt
// WHEN it's applied
// THEN the resulting widget renders collapsed by default — a single-line
// preview, not the full multi-line prompt — matching kindToolResult's
// existing default. Expanding it (ToggleCollapse) still reveals the full
// body on demand (docs/architecture/behavior/
// output_approval_request_collapsed_by_default.feature.md).
func TestApprovalRequestWidgetCollapsedByDefault(t *testing.T) {
	m := New()
	m.SetSize(100, 30)
	prompt := "write_file content=package task\n\nimport (\n\t\"encoding/json\"\n\n\t\"testing\"\n)\n\nfunc T… path=internal/prompting/task/hypothesis_test.go"
	m.Apply(state.Event{EventType: "TOOL_CALL", ContentType: state.ContentApprovalRequest,
		Payload: map[string]any{"prompt": prompt}, Enabled: true})

	collapsedRows := strings.Split(m.View(), "\n")
	// border top + one collapsed preview row + border bottom, at most (exact
	// framing depends on boxify, but it must NOT contain the full body).
	if strings.Contains(m.View(), "encoding/json") {
		t.Errorf("View() while collapsed contains full body content, want only a one-line preview: %v", collapsedRows)
	}

	m.ToggleCollapse(0)
	if !strings.Contains(m.View(), "encoding/json") {
		t.Errorf("View() after ToggleCollapse does not contain the full body, want it expanded: %q", m.View())
	}
}
