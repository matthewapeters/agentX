// Package workmemory is the working-memory editor surface (SS-6): a read-write peer
// surface that lists the session's working-memory facts and lets the user add, edit,
// delete, and enable/disable them. Unlike the context viewer it is not event-driven —
// working memory is a document, so it reads on attach, polls for live refresh, and
// mutates through the dedicated transport endpoints.
//
// Scroll/collapse (PD-WM-AF-010..013, docs/ux/03_PANEL_DETAILS.md) reuses the output
// panel's established viewport/collapse/scrollbar taxonomy (PD-01,
// docs/ux/06_OUTPUT_WIDGET.md) via the shared internal/surfaces/scrollutil helpers,
// including its keybinding split: PgUp/PgDn move the fact cursor, ↑/↓/j/k inner-scroll
// the selected fact's value.
package workmemory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"agentx/internal/session"
	"agentx/internal/surfaces"
	"agentx/internal/surfaces/client"
	"agentx/internal/surfaces/scrollutil"
	transporthttp "agentx/internal/transport/http"
)

// pollInterval is how often the surface refetches working memory so agent-side or
// other-surface changes appear.
const pollInterval = 2 * time.Second

// factMaxBody caps an expanded fact's rendered row count before its value scrolls in
// place, mirroring the output panel's defaultMaxBody (PD-01) but smaller: this
// surface is a quick-reference list, not a primary reading surface.
const factMaxBody = 12

// prefixWidth is the fixed display width of a fact row's
// cursor+enabled-dot+live-mark+space prefix ("› ●▶ " / "  ○   "); continuation
// rows get this many blank columns instead, so wrapped/expanded value text hangs
// aligned under the first row. The live/static mark lives here — not in the
// truncatable value/owner text — so it is never silently clipped away on a narrow
// terminal or a long value: it is the one signal that must always be visible,
// since it is the only thing that says "will this be re-run before the next
// turn," which enabled/disabled (the dot) does not.
const prefixWidth = 5

// headerRows and footerRows are the fixed, never-scrolled chrome around the fact
// list's viewport: title + blank line above; blank line + action/editor line +
// status line below. The status line always renders (blank when there is no status),
// so the viewport height stays constant regardless of whether a message is present.
const (
	headerRows = 2
	footerRows = 3
)

const hintLine = "pgup/pgdn move · ↑/↓ scroll value · ↵ expand/collapse · space toggle · " +
	"l live/static (pinned) · a add · e edit · d delete · r refresh · q quit"

// Options configures the working-memory surface program.
type Options struct {
	Endpoint    string
	Token       string
	SurfaceID   string
	SessionName string
	// SessionRoot and SessionID enable re-attachment after a mid-session
	// resume (docs/architecture/behavior/session_resume.feature.md §5): unlike
	// the event-stream surfaces (client.Host), this document-based surface
	// holds its own transport client/token captured once at launch, so a
	// resume that swaps the session out from under it otherwise leaves it
	// stuck presenting a now-invalid token forever. SessionRoot == "" (an
	// older/headless caller) disables re-attachment — an auth failure then
	// just reports as before.
	SessionRoot string
	SessionID   string
}

// Run launches the working-memory editor against a running orchestrator and blocks
// until the user quits.
func Run(ctx context.Context, opts Options) error {
	// Hold an event stream purely for connection presence (SS-4): the orchestrator
	// reports a surface connected while it has an open stream. WM is document-based
	// (it polls), so it consumes no events — but it still holds a stream so the
	// launch widget shows it as 🟢. The stream closes on exit.
	defer client.Presence(ctx, opts.Endpoint, opts.SurfaceID)()

	m := New(transporthttp.NewClient(opts.Endpoint), opts)
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

type editMode int

const (
	modeList editMode = iota
	modeAdd           // entering "key=value"
	modeEdit          // entering a new value for the selected fact
)

// FactsMsg delivers a fetched working-memory snapshot to the surface. It is exported
// so the surface can be driven without a live server (tests, future hosts).
type FactsMsg []session.Fact

type (
	errMsg  struct{ err error }
	doneMsg struct{} // a mutation completed — refetch
	tickMsg struct{}
)

// reattachedMsg carries the result of re-resolving this session's transport
// endpoint and attach token from disk after an auth failure (see
// Options.SessionRoot).
type reattachedMsg struct {
	endpoint string
	token    string
	err      error
}

// isStaleAttachToken reports whether err is the server rejecting this
// surface's attach token — the specific, recoverable case (a resume swapped
// the underlying session out from under this surface) as opposed to a
// transport/validation failure, which re-resolving from disk cannot fix.
func isStaleAttachToken(err error) bool {
	var ae *transporthttp.AttachError
	return errors.As(err, &ae) && ae.Category == surfaces.CategoryAuth
}

// factUI is a fact's ephemeral render state (collapsed/expanded, inner-scroll
// offset) — kept OUTSIDE session.Fact because Model.facts is replaced wholesale on
// every poll tick (FactsMsg), which would otherwise clobber it every ~2s. Keyed by
// Fact.Key (its stable identity) in Model.ui; entries for keys no longer present are
// pruned on each FactsMsg so a deleted-then-re-added fact doesn't inherit stale
// state. Only created for facts whose value actually wraps to more than one line —
// a single-line fact never needs an entry.
type factUI struct {
	collapsed bool // starts true: a multi-line value collapses by default
	offset    int  // inner-scroll offset (wrapped body-line index) when expanded
}

// Model is the working-memory editor.
type Model struct {
	cl   *transporthttp.Client
	opts Options

	facts    []session.Fact
	selected int
	width    int
	height   int
	vp       viewport.Model
	ui       map[string]*factUI

	mode   editMode
	buf    []rune // inline editor buffer
	status string
}

// New returns a working-memory editor bound to a transport client.
func New(cl *transporthttp.Client, opts Options) Model {
	vp := viewport.New()
	vp.FillHeight = true
	m := Model{cl: cl, opts: opts, width: 80, height: 24, vp: vp, ui: map[string]*factUI{}}
	m.applySize()
	return m
}

// Init fetches the initial facts and starts the refresh poll.
func (m Model) Init() tea.Cmd { return tea.Batch(m.fetch(), tickCmd()) }

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// reattach re-resolves this session's transport endpoint and attach token
// from disk (SS-5) — the same discovery a fresh `agentx surface launch` uses
// — after the server has rejected this surface's token, which happens when a
// mid-session resume (ESC,r) swaps the running session out from under it
// (docs/architecture/behavior/session_resume.feature.md §5). A no-op
// (returns nil) when SessionRoot is unset, matching client.Host's own
// SessionRoot == "" convention for callers that predate reconnection.
func (m Model) reattach() tea.Cmd {
	if m.opts.SessionRoot == "" || m.opts.SessionID == "" {
		return nil
	}
	root, id := m.opts.SessionRoot, m.opts.SessionID
	return func() tea.Msg {
		store := session.NewStore(root)
		info, err := store.ReadTransport(id)
		if err != nil {
			return reattachedMsg{err: err}
		}
		token, err := store.ReadAttachToken(id)
		if err != nil {
			return reattachedMsg{err: err}
		}
		return reattachedMsg{endpoint: info.Endpoint, token: token}
	}
}

func (m Model) fetch() tea.Cmd {
	cl := m.cl
	return func() tea.Msg {
		facts, err := cl.WorkingMemory(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return FactsMsg(facts)
	}
}

func (m Model) setFact(key, value string) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.SetFact(context.Background(), token, key, value); err != nil {
			return errMsg{err}
		}
		return doneMsg{}
	}
}

func (m Model) toggle(key string, enabled bool) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.SetFactEnabled(context.Background(), token, key, enabled); err != nil {
			return errMsg{err}
		}
		return doneMsg{}
	}
}

func (m Model) deleteFact(key string) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.DeleteFact(context.Background(), token, key); err != nil {
			return errMsg{err}
		}
		return doneMsg{}
	}
}

// toggleLive flips a pinned fact between live (▶, re-run before every turn) and
// static (⏸, frozen) — PD-WM-AF-008. The server refuses it on a fact with no
// tool Source (a plain user/agent fact).
func (m Model) toggleLive(key string, live bool) tea.Cmd {
	cl, token := m.cl, m.opts.Token
	return func() tea.Msg {
		if err := cl.SetFactLive(context.Background(), token, key, live); err != nil {
			return errMsg{err}
		}
		return doneMsg{}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.applySize()
		m.refresh()
		return m, nil
	case FactsMsg:
		m.facts = []session.Fact(msg)
		m.pruneUI()
		if m.selected >= len(m.facts) {
			m.selected = max(0, len(m.facts)-1)
		}
		m.refresh()
		return m, nil
	case errMsg:
		if isStaleAttachToken(msg.err) && m.opts.SessionRoot != "" {
			m.status = "reattaching…"
			return m, m.reattach()
		}
		m.status = "error: " + msg.err.Error()
		return m, nil
	case reattachedMsg:
		if msg.err != nil {
			m.status = "error: reattach failed: " + msg.err.Error()
			return m, nil
		}
		m.opts.Endpoint, m.opts.Token = msg.endpoint, msg.token
		m.cl = transporthttp.NewClient(msg.endpoint)
		m.status = ""
		return m, m.fetch()
	case doneMsg:
		return m, m.fetch()
	case tickMsg:
		return m, tea.Batch(m.fetch(), tickCmd())
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

// cur returns the selected fact, if any.
func (m Model) cur() (session.Fact, bool) {
	if m.selected < 0 || m.selected >= len(m.facts) {
		return session.Fact{}, false
	}
	return m.facts[m.selected], true
}

// uiFor returns key's render state, creating a collapsed-by-default entry on first
// sight. It mutates the shared Model.ui map, which every copy of Model (this is a
// value type, copied per Update) points at the same underlying map, so the entry
// persists across the value-receiver copies bubbletea's Elm architecture produces.
func (m Model) uiFor(key string) *factUI {
	u, ok := m.ui[key]
	if !ok {
		u = &factUI{collapsed: true}
		m.ui[key] = u
	}
	return u
}

// pruneUI drops render state for facts no longer present, so a deleted-then-re-added
// fact (or one simply gone after a poll) doesn't inherit stale collapse/offset state.
func (m *Model) pruneUI() {
	live := make(map[string]bool, len(m.facts))
	for _, f := range m.facts {
		live[f.Key] = true
	}
	for k := range m.ui {
		if !live[k] {
			delete(m.ui, k)
		}
	}
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.mode != modeList {
		return m.editKey(msg)
	}
	var cmd tea.Cmd
	switch msg.String() {
	case "ctrl+c", "q":
		_ = m.cl.Shutdown(context.Background(), m.opts.SurfaceID, m.opts.Token)
		return m, tea.Quit
	case "pgup":
		m.moveSelection(-1)
	case "pgdown":
		m.moveSelection(1)
	case "up", "k":
		m.scrollSelected(-1)
	case "down", "j":
		m.scrollSelected(1)
	case "enter":
		m.toggleExpand()
	case " ", "space":
		if f, ok := m.cur(); ok {
			cmd = m.toggle(f.Key, !f.Enabled)
		}
	case "d", "x":
		if f, ok := m.cur(); ok {
			cmd = m.deleteFact(f.Key)
		}
	case "l":
		if f, ok := m.cur(); ok && f.Source != nil {
			cmd = m.toggleLive(f.Key, !f.Live)
		}
	case "a", "n":
		m.mode = modeAdd
		m.buf = nil
		m.status = "add — type key=value, enter to save, esc to cancel"
	case "e":
		if f, ok := m.cur(); ok {
			m.mode = modeEdit
			m.buf = []rune(f.Value)
			m.status = "edit " + f.Key + " — enter to save, esc to cancel"
		}
	case "r":
		cmd = m.fetch()
	}
	m.refresh()
	return m, cmd
}

// editKey handles the inline single-line editor for add/edit.
func (m Model) editKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape":
		m.mode, m.buf, m.status = modeList, nil, ""
		return m, nil
	case "enter":
		mode, buf := m.mode, strings.TrimSpace(string(m.buf))
		m.mode, m.buf, m.status = modeList, nil, ""
		switch mode {
		case modeAdd:
			key, value, ok := strings.Cut(buf, "=")
			key = strings.TrimSpace(key)
			if !ok || key == "" {
				m.status = "expected key=value"
				return m, nil
			}
			return m, m.setFact(key, strings.TrimSpace(value))
		case modeEdit:
			if f, ok := m.cur(); ok {
				return m, m.setFact(f.Key, buf)
			}
		}
		return m, nil
	case "backspace":
		if n := len(m.buf); n > 0 {
			m.buf = m.buf[:n-1]
		}
		return m, nil
	}
	if msg.Text != "" {
		m.buf = append(m.buf, []rune(msg.Text)...)
	}
	return m, nil
}

// moveSelection moves the fact cursor by delta (PgUp/PgDn, PD-WM-AF-002) and scrolls
// the outer viewport to keep the new selection visible (PD-WM-AF-012).
func (m *Model) moveSelection(delta int) {
	if len(m.facts) == 0 {
		return
	}
	m.selected = scrollutil.ClampInt(m.selected+delta, 0, len(m.facts)-1)
	m.scrollSelectedIntoView()
}

// scrollSelected inner-scrolls the selected fact's expanded value by n rows
// (PD-WM-AF-010); a no-op when the fact is collapsed or has nothing to scroll — a
// freshly created factUI defaults collapsed, so this is also a harmless no-op on a
// single-line fact.
func (m *Model) scrollSelected(n int) {
	f, ok := m.cur()
	if !ok {
		return
	}
	ui := m.uiFor(f.Key)
	if ui.collapsed {
		return
	}
	ui.offset += n
	// render() clamps ui.offset against the actual wrapped length as a side effect
	// the next time it runs, mirroring the output panel's renderBody clamp-on-render.
}

// toggleExpand flips the selected fact between collapsed and expanded
// (PD-WM-AF-011); a no-op on a fact whose value is a single line.
func (m *Model) toggleExpand() {
	f, ok := m.cur()
	if !ok {
		return
	}
	if len(scrollutil.WrapLines(f.Value, m.bodyWidth())) <= 1 {
		return
	}
	ui := m.uiFor(f.Key)
	ui.collapsed = !ui.collapsed
	m.scrollSelectedIntoView()
}

// applySize sizes the viewport to the panel width (minus the transcript-scrollbar
// gutter column) and height (minus the fixed header/footer chrome).
func (m *Model) applySize() {
	vpH := m.height - headerRows - footerRows
	if vpH < 0 {
		vpH = 0
	}
	m.vp.SetWidth(m.contentWidth())
	m.vp.SetHeight(vpH)
}

// refresh re-renders every fact into the viewport's content.
func (m *Model) refresh() {
	b := m.render()
	m.vp.SetContent(strings.Join(b.lines, "\n"))
}

// scrollSelectedIntoView scrolls the outer viewport so the selected fact's block is
// visible, the same behavior the output panel's scrollSelectedIntoView provides.
func (m *Model) scrollSelectedIntoView() {
	if m.selected < 0 {
		return
	}
	b := m.render()
	if m.selected >= len(b.starts) {
		return
	}
	top := b.starts[m.selected]
	bottom := top + b.totals[m.selected] - 1
	y := m.vp.YOffset()
	h := m.vp.Height()
	switch {
	case top < y:
		m.vp.SetYOffset(top)
	case bottom >= y+h:
		m.vp.SetYOffset(bottom - h + 1)
	}
}

// contentWidth is the panel width minus the transcript-scrollbar gutter column.
func (m Model) contentWidth() int { return max(m.width-1, 0) }

// bodyWidth is the width available to a fact's value after the cursor/dot prefix.
func (m Model) bodyWidth() int { return max(m.contentWidth()-prefixWidth, 1) }

// blocks holds, per fact, its rendered lines and starting row in the fact list —
// mirrors the output panel's render() bookkeeping so scrollSelectedIntoView can
// locate the selected fact's block.
type blocks struct {
	lines  []string
	starts []int
	totals []int
}

func (m Model) render() blocks {
	var b blocks
	if len(m.facts) == 0 {
		b.lines = []string{"  (no facts — press 'a' to add)"}
		return b
	}
	row := 0
	for i, f := range m.facts {
		lines := m.renderFact(f, i == m.selected)
		b.starts = append(b.starts, row)
		b.totals = append(b.totals, len(lines))
		b.lines = append(b.lines, lines...)
		row += len(lines)
	}
	return b
}

// renderFact renders one fact's rows: a single line when its value fits on one line;
// otherwise a collapsed first-line preview (PD-WM-AF-011 default), or — once
// expanded — a header row plus the wrapped value capped to factMaxBody rows with its
// own scrollbar when it still doesn't fit (PD-WM-AF-010). The owner/pin annotation
// always stays on the fact's first rendered row, never buried inside a wrapped or
// windowed body.
func (m Model) renderFact(f session.Fact, selected bool) []string {
	innerW := m.contentWidth()
	cursor := "  "
	if selected {
		cursor = "› "
	}
	dot := "○"
	if f.Enabled {
		dot = "●"
	}
	prefix := cursor + dot + liveMark(f) + " "
	owner := ownerSuffix(f)
	bodyW := m.bodyWidth()

	wrapped := scrollutil.WrapLines(f.Value, bodyW)
	if len(wrapped) <= 1 {
		line := prefix + f.Key + " = " + f.Value + owner
		return []string{scrollutil.PadTo(scrollutil.TruncateWord(line, innerW), innerW)}
	}

	ui := m.uiFor(f.Key)
	blank := strings.Repeat(" ", prefixWidth)
	if ui.collapsed {
		hint := fmt.Sprintf("%s = %s … (+%d lines)%s", f.Key, wrapped[0], len(wrapped)-1, owner)
		return []string{scrollutil.PadTo(scrollutil.TruncateWord(prefix+hint, innerW), innerW)}
	}

	header := scrollutil.PadTo(scrollutil.TruncateWord(prefix+f.Key+" ="+owner, innerW), innerW)
	rows := []string{header}

	if len(wrapped) <= factMaxBody {
		ui.offset = 0
		for _, l := range wrapped {
			rows = append(rows, blank+scrollutil.PadTo(l, bodyW))
		}
		return rows
	}

	// Over the cap: reserve a scrollbar column and window the value.
	bw := max(bodyW-1, 1)
	rewrapped := scrollutil.WrapLines(f.Value, bw)
	total := len(rewrapped)
	maxOffset := total - factMaxBody
	ui.offset = scrollutil.ClampInt(ui.offset, 0, maxOffset)
	window := rewrapped[ui.offset : ui.offset+factMaxBody]
	for i, l := range window {
		rows = append(rows, blank+scrollutil.PadTo(l, bw)+scrollutil.ScrollbarCell(i, ui.offset, total, factMaxBody))
	}
	return rows
}

// liveMark is the fixed-prefix glyph distinguishing a pin's live/static state
// (PD-WM-AF-007/008): ▶ re-run before every turn, ⏸ frozen since PinnedAt. It is
// orthogonal to the enabled dot — pausing a pin (⏸) is not the same as disabling
// it (○): a paused-but-enabled pin still folds its (frozen) value into context,
// it just stops re-running. A non-pin fact gets a blank column here so every
// row's key/value still starts at the same column regardless of owner.
func liveMark(f session.Fact) string {
	if f.Owner != session.OwnerPin {
		return " "
	}
	if f.Live {
		return "▶"
	}
	return "⏸"
}

// ownerSuffix renders a fact's owner annotation: none for a plain user fact, "
// (agent)" for an agent-maintained one, or the pin's static/live state and age.
// The live/static glyph itself lives in the row's fixed prefix (liveMark), not
// here, so it is never lost to truncation — this text is best-effort detail.
func ownerSuffix(f session.Fact) string {
	switch f.Owner {
	case session.OwnerAgent:
		return " (agent)"
	case session.OwnerPin:
		state := "static"
		if f.Live {
			state = "live"
		}
		return fmt.Sprintf(" (%s, %s)", state, f.Age().Round(time.Second))
	}
	return ""
}

// View implements tea.Model.
func (m Model) View() tea.View {
	var b strings.Builder
	title := "working memory"
	if m.opts.SessionName != "" {
		title += " · " + m.opts.SessionName
	}
	fmt.Fprintf(&b, "%s\n\n", scrollutil.TruncateWord(title, m.width))

	// Defensive: refresh a local copy so the view is correct even before the first
	// WindowSizeMsg/FactsMsg has run (New already seeds a usable default size/vp).
	m.refresh()
	b.WriteString(m.compositedViewport())
	b.WriteString("\n")

	if m.mode != modeList {
		fmt.Fprintf(&b, "> %s█\n", string(m.buf))
	} else {
		fmt.Fprintf(&b, "%s\n", scrollutil.TruncateWord(hintLine, m.width))
	}
	// Always emitted (blank when empty) so the viewport's reserved height never
	// shifts based on whether a status message happens to be present.
	fmt.Fprintf(&b, "%s\n", scrollutil.TruncateWord(m.status, m.width))

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// compositedViewport composites the scrolled fact list with the reserved
// transcript-scrollbar gutter column (blank when everything fits) — mirrors the
// output panel's own View() gutter compositing (PD-01-AF, docs/ux/06_OUTPUT_WIDGET.md).
func (m Model) compositedViewport() string {
	lines := strings.Split(m.vp.View(), "\n")
	total := m.vp.TotalLineCount()
	track := len(lines)
	offset := m.vp.YOffset()
	for i := range lines {
		if total > track {
			lines[i] += scrollutil.ScrollbarCell(i, offset, total, track)
		} else {
			lines[i] += " "
		}
	}
	return strings.Join(lines, "\n")
}
