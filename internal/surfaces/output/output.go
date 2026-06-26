// Package output is the chat surface's output panel: a fixed-height region that
// renders conversation events (user, thinking, assistant, tool call/result,
// system, error) with streaming assistant text, collapsible thinking/tool
// blocks, and bottom-anchored scrolling.
//
// Source contract: docs/ux/03_PANEL_DETAILS.md PD-01 (re-authored for the TUI).
// Backlog task: CHT-B2.
package output

import (
	"fmt"
	"strings"

	"agentx/internal/state"
)

type entryKind int

const (
	kindUser entryKind = iota
	kindAssistant
	kindThinking
	kindToolCall
	kindToolResult
	kindSystem
	kindError
)

type entry struct {
	kind        entryKind
	header      string
	body        string
	collapsible bool
	collapsed   bool
}

// Model is the output panel state.
type Model struct {
	width   int
	height  int
	entries []entry
	scroll  int // rows scrolled up from the bottom
}

// New returns an empty output panel.
func New() *Model { return &Model{} }

// SetSize sets the panel's render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
	m.clampScroll()
}

// Height returns the panel's row count.
func (m *Model) Height() int { return m.height }

// Apply folds a bus event into the panel. Assistant responses stream into a
// single entry; thinking and tool-result blocks start collapsed.
func (m *Model) Apply(ev state.Event) {
	if ev.EventType == "ERROR" {
		m.entries = append(m.entries, entry{kind: kindError, header: "⚠ " + eventText(ev)})
		m.scroll = 0
		return
	}

	switch ev.ContentType {
	case state.ContentUserPrompt:
		m.entries = append(m.entries, entry{kind: kindUser, header: "👤 " + eventText(ev)})
	case state.ContentAgentResponse:
		m.appendAssistant(eventText(ev))
	case state.ContentThinking:
		m.entries = append(m.entries, entry{kind: kindThinking, header: "💭 thinking", body: eventText(ev), collapsible: true, collapsed: true})
	case state.ContentToolCall:
		m.entries = append(m.entries, entry{kind: kindToolCall, header: "🔧 " + ev.ToolName, body: eventText(ev), collapsible: true})
	case state.ContentToolResult:
		m.entries = append(m.entries, entry{kind: kindToolResult, header: "📋 result", body: eventText(ev), collapsible: true, collapsed: true})
	case state.ContentSystemPrompt:
		m.entries = append(m.entries, entry{kind: kindSystem, header: "⚙ " + eventText(ev)})
	default:
		// Ignored in the output panel (e.g. processing_state, attachments).
		return
	}
	m.scroll = 0
}

func (m *Model) appendAssistant(text string) {
	if n := len(m.entries); n > 0 && m.entries[n-1].kind == kindAssistant {
		m.entries[n-1].header += text
		return
	}
	m.entries = append(m.entries, entry{kind: kindAssistant, header: "🤖 " + text})
}

// AssistantEntries returns the number of distinct assistant entries (one per
// streamed response).
func (m *Model) AssistantEntries() int {
	n := 0
	for _, e := range m.entries {
		if e.kind == kindAssistant {
			n++
		}
	}
	return n
}

// ToggleCollapse flips the collapsed state of the i-th entry if it is
// collapsible.
func (m *Model) ToggleCollapse(i int) {
	if i < 0 || i >= len(m.entries) || !m.entries[i].collapsible {
		return
	}
	m.entries[i].collapsed = !m.entries[i].collapsed
	m.clampScroll()
}

// ScrollUp scrolls toward older content.
func (m *Model) ScrollUp(n int) { m.scroll += n; m.clampScroll() }

// ScrollDown scrolls toward newer content.
func (m *Model) ScrollDown(n int) { m.scroll -= n; m.clampScroll() }

func (m *Model) clampScroll() {
	maxScroll := len(m.renderLines()) - m.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// renderLines flattens entries to display lines, including bodies of expanded
// collapsible entries.
func (m *Model) renderLines() []string {
	var lines []string
	for _, e := range m.entries {
		lines = append(lines, e.header)
		if e.collapsible && !e.collapsed && e.body != "" {
			for _, bl := range strings.Split(e.body, "\n") {
				lines = append(lines, "  "+bl)
			}
		}
	}
	return lines
}

// View renders exactly Height rows, bottom-anchored and offset by the scroll
// position, padding with blanks so the panel always fills its region.
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	all := m.renderLines()
	end := len(all) - m.scroll
	if end > len(all) {
		end = len(all)
	}
	if end < 0 {
		end = 0
	}
	start := end - m.height
	if start < 0 {
		start = 0
	}

	rows := make([]string, 0, m.height)
	for _, l := range all[start:end] {
		rows = append(rows, fitWidth(l, m.width))
	}
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	if len(rows) > m.height {
		rows = rows[len(rows)-m.height:]
	}
	return strings.Join(rows, "\n")
}

func eventText(ev state.Event) string {
	switch p := ev.Payload.(type) {
	case map[string]any:
		if t, ok := p["text"].(string); ok {
			return t
		}
	case string:
		return p
	}
	return fmt.Sprintf("%v", ev.Payload)
}

func fitWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}
