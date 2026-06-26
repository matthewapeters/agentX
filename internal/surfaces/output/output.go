// Package output is the chat surface's output panel: a fixed-height region that
// renders conversation events (user, thinking, assistant, tool call/result,
// system, error) with streaming assistant text, collapsible thinking/tool
// blocks, and bottom-anchored scrolling.
//
// The panel hosts its content in a charm.land/bubbles/v2/viewport: the entry
// model produces word-wrapped display lines and the viewport owns scrolling,
// height padding, and (when the program enables the mouse) wheel handling.
//
// Source contract: docs/ux/03_PANEL_DETAILS.md PD-01 (re-authored for the TUI).
// Backlog task: CHT-B2.
package output

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
	vp      viewport.Model
	width   int
	height  int
	entries []entry
}

// New returns an empty output panel backed by a viewport.
func New() *Model {
	vp := viewport.New()
	vp.FillHeight = true
	return &Model{vp: vp}
}

// SetSize sets the panel's render dimensions and reflows content to the new
// width.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
	m.vp.SetWidth(m.width)
	m.vp.SetHeight(m.height)
	m.refresh(false)
}

// Height returns the panel's row count.
func (m *Model) Height() int { return m.height }

// Update forwards scrolling messages (viewport keys, mouse wheel) to the
// embedded viewport and returns any resulting command.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return cmd
}

// Apply folds a bus event into the panel. Assistant responses stream into a
// single entry; thinking and tool-result blocks start collapsed. New content
// pins the view to the bottom so the stream stays in sight.
func (m *Model) Apply(ev state.Event) {
	if ev.EventType == "ERROR" {
		m.entries = append(m.entries, entry{kind: kindError, header: "⚠ " + eventText(ev)})
		m.refresh(true)
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
	m.refresh(true)
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
	m.refresh(false)
}

// ScrollUp scrolls toward older content.
func (m *Model) ScrollUp(n int) { m.vp.ScrollUp(n) }

// ScrollDown scrolls toward newer content.
func (m *Model) ScrollDown(n int) { m.vp.ScrollDown(n) }

// PageUp and PageDown scroll by a panel height.
func (m *Model) PageUp()   { m.vp.ScrollUp(m.height) }
func (m *Model) PageDown() { m.vp.ScrollDown(m.height) }

// refresh re-renders the entry list into the viewport. When pinBottom is set (or
// the view was already at the bottom) it follows the newest content; otherwise
// it preserves the current scroll position across the content change.
func (m *Model) refresh(pinBottom bool) {
	atBottom := m.vp.AtBottom()
	m.vp.SetContent(strings.Join(m.renderLines(), "\n"))
	if pinBottom || atBottom {
		m.vp.GotoBottom()
	}
}

// renderLines flattens entries to display lines, word-wrapping each entry to the
// panel width so long responses reflow instead of being truncated. Bodies of
// expanded collapsible entries are wrapped to a narrower width and indented.
func (m *Model) renderLines() []string {
	var lines []string
	for _, e := range m.entries {
		lines = append(lines, m.wrap(e.header, m.width)...)
		if e.collapsible && !e.collapsed && e.body != "" {
			for _, bl := range m.wrap(e.body, m.width-2) {
				lines = append(lines, "  "+bl)
			}
		}
	}
	return lines
}

// wrap word-wraps s to limit display cells, preserving existing newlines and
// hard-breaking words longer than the limit. A non-positive limit disables
// wrapping (the raw lines are returned).
func (m *Model) wrap(s string, limit int) []string {
	if limit <= 0 {
		return strings.Split(s, "\n")
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, strings.Split(ansi.Wrap(line, limit, " -"), "\n")...)
	}
	return out
}

// View renders the viewport: exactly Height rows, bottom-anchored, padded to the
// panel region.
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	return m.vp.View()
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
