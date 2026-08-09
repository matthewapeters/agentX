package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"agentx/internal/prompting"
	"agentx/internal/session"
	"agentx/internal/state"
)

// This file holds ContextStore's Orchestrator-side implementation (ADR 0013
// Phase 4) and everything it depends on: the session's on-disk working memory
// plus its in-process turn history. These stay Orchestrator methods, not
// ConversationCore methods — o.mu/o.history/o.store are Orchestrator-private
// state (ADR 0013 §"What stays on Orchestrator, unconditionally"), and Core
// reaches them only through Augment/Record, never directly. Moving here from
// orchestrator.go is a pure code-organization change: continuation.go and
// classifier_pipeline.go's disconnected pipeline still call withContext/
// recordTurn directly (same as Phase 3's streamResponse/finishCycle), so both
// method names and behavior are unchanged, just relocated for cohesion with
// Augment/Record.

// turnMsg is one conversation element in the in-memory context history: a user
// prompt, a complete agent response, or a pinned tool_call/tool_result. ordinal is
// the element's durable identity (its source event's ordinal); enabled controls
// whether it folds into the next prompt's assembled context (toggled from the
// context surface — a "tool" entry starts disabled and is the pin affordance).
type turnMsg struct {
	ordinal uint64
	role    string // "user" | "assistant" | "tool"
	content string
	enabled bool
}

// Augment satisfies ContextStore as a pure pass-through to withContext.
func (o *Orchestrator) Augment(base []prompting.Message) []prompting.Message {
	return o.withContext(base)
}

// Record satisfies ContextStore as a pure pass-through to recordTurn, unpacking
// entry's fields into recordTurn's existing parameter list.
func (o *Orchestrator) Record(entry TurnRecord) {
	o.recordTurn(entry.Err, entry.Record, entry.UserOrd, entry.UserText, entry.RespOrd, entry.Response, entry.Pins)
}

// recordTurn appends the completed turn to the in-memory conversation history when
// the cycle ended cleanly (success or user interrupt), so the next turn carries
// the prior user prompt and agent response as enabled context. Each entry keeps the
// ordinal of its source event (user_prompt / complete agent_response) as its stable
// identity, so the context surface can toggle it. The bootstrap turn
// (recordTurn=false, like its user-prompt event) is excluded: it engages the
// session but is irrelevant to the user's intent. Hard failures are not recorded.
//
// pins registers every native tool call this turn made as pinnable entries
// too (a turn may call more than one tool now, unlike the old single_tool
// cycle's one-call-per-turn limit), ordered between the user prompt and the
// answer (their real chronological place). They start disabled — matching
// their checkbox and state.DefaultEnabled — so nothing changes until the
// context surface pins one; from then on it folds into every subsequent
// turn's assembled context, same as any other toggled-on element, until
// unpinned.
func (o *Orchestrator) recordTurn(err error, record bool, userOrd uint64, userText string, respOrd uint64, response string, pins []*toolPin) {
	if !record {
		return
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if userText != "" {
		o.history = append(o.history, turnMsg{ordinal: userOrd, role: "user", content: userText, enabled: true})
	}
	for _, pin := range pins {
		if pin.callOrdinal != 0 {
			o.history = append(o.history, turnMsg{ordinal: pin.callOrdinal, role: "tool",
				content: "[pinned tool call] " + pin.callText, enabled: state.DefaultEnabled(state.ContentToolCall)})
		}
		if pin.resultOrdinal != 0 {
			o.history = append(o.history, turnMsg{ordinal: pin.resultOrdinal, role: "tool",
				content: "[pinned tool result] " + pin.resultText, enabled: state.DefaultEnabled(state.ContentToolResult)})
		}
	}
	if response != "" {
		o.history = append(o.history, turnMsg{ordinal: respOrd, role: "assistant", content: response, enabled: true})
	}
}

// historyFromEvents reconstructs the in-memory conversation history from a
// session's persisted event log — the reverse of recordTurn, used when
// resuming a session
// (docs/architecture/behavior/session_resume.feature.md §3). Unlike
// recordTurn's forward-direction construction (which hardcodes enabled: true
// for a fresh user/assistant turn), this reads enabled: ev.Enabled uniformly
// for every entry: SetEventEnabled already rewrites the specific persisted
// event file on every toggle (Recorder.SetEnabled), so the on-disk log is
// already the correct, up-to-date source of truth for this — no new
// persistence work, only the reverse-direction read. Ephemeral events (the
// bootstrap exchange) are skipped, the same exclusion
// internal/surfaces/context.Model.Apply already applies for the same
// reason. Content shape (the "[pinned tool call] "/"[pinned tool result] "
// prefixes) matches recordTurn's exactly, so a reconstructed tool entry is
// indistinguishable from one recordTurn would have produced live.
func historyFromEvents(events []state.Event) []turnMsg {
	hist := make([]turnMsg, 0, len(events))
	for _, ev := range events {
		if ev.Ephemeral {
			continue
		}
		text, ok := eventText(ev)
		switch ev.ContentType {
		case state.ContentUserPrompt:
			if !ok || text == "" {
				continue
			}
			hist = append(hist, turnMsg{ordinal: ev.Ordinal, role: "user", content: text, enabled: ev.Enabled})
		case state.ContentAgentResponse:
			if !ok || text == "" {
				continue
			}
			hist = append(hist, turnMsg{ordinal: ev.Ordinal, role: "assistant", content: text, enabled: ev.Enabled})
		case state.ContentToolCall:
			hist = append(hist, turnMsg{ordinal: ev.Ordinal, role: "tool", content: "[pinned tool call] " + text, enabled: ev.Enabled})
		case state.ContentToolResult:
			hist = append(hist, turnMsg{ordinal: ev.Ordinal, role: "tool", content: "[pinned tool result] " + text, enabled: ev.Enabled})
		}
	}
	return hist
}

// eventText extracts an event's generic "text" payload field, the common
// shape user_prompt/agent_response/tool_call/tool_result events all share.
func eventText(ev state.Event) (string, bool) {
	p, ok := ev.Payload.(map[string]any)
	if !ok {
		return "", false
	}
	text, ok := p["text"].(string)
	return text, ok
}

// maxEventOrdinal returns the highest Ordinal among events, or 0 for an
// empty log — the value a resumed session's state.NewBusFrom seeds its
// ordinal counter from, so the first newly published event is stamped past
// every ordinal already on disk
// (docs/architecture/behavior/session_resume.feature.md §3).
func maxEventOrdinal(events []state.Event) uint64 {
	var max uint64
	for _, ev := range events {
		if ev.Ordinal > max {
			max = ev.Ordinal
		}
	}
	return max
}

// historyMessages returns the enabled prior-turn conversation history as assembler
// messages — disabled elements (toggled off from the context surface) are withheld.
// A pinned tool entry (role "tool") is sent as a user-role message: the safest
// broadly-compatible representation for a hand-rolled (non-native-tool-calling)
// chat loop, tagged so the model reads it as reference material, not user speech.
func (o *Orchestrator) historyMessages() []prompting.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]prompting.Message, 0, len(o.history))
	for _, h := range o.history {
		if !h.enabled {
			continue
		}
		role := h.role
		if role == "tool" {
			role = "user"
		}
		out = append(out, prompting.Message{Role: role, Content: h.content})
	}
	return out
}

// withContext folds working memory and enabled prior-turn history into an
// assembled [system?, user] message list, in layer order: instructions (Layer 0)
// → working memory (band 0) → enabled conversation history → the current user
// turn. Both are re-read fresh each turn so edits/new turns take effect on the
// next post. System messages are merged into a single system message at the
// beginning to satisfy Jinja template requirements (llama.cpp compat).
func (o *Orchestrator) withContext(msgs []prompting.Message) []prompting.Message {
	at := 0
	for at < len(msgs) && msgs[at].Role == "system" {
		at++
	}
	out := make([]prompting.Message, 0, len(msgs)+len(o.history)+1)
	// Merge all leading system messages (instructions + working memory) into one
	// Make a copy to avoid mutating the input slice via append aliasing
	sysMsgs := make([]prompting.Message, at)
	copy(sysMsgs, msgs[:at])
	if wmMsg, ok := o.workingMemoryMessage(); ok {
		sysMsgs = append(sysMsgs, wmMsg)
	}
	if len(sysMsgs) > 0 {
		merged := mergeSystemMessages(sysMsgs)
		out = append(out, merged)
	}
	out = append(out, o.historyMessages()...)
	out = append(out, msgs[at:]...)
	return out
}

// mergeSystemMessages combines multiple system messages into one, separated by
// newlines. This is required for llama.cpp Jinja templates which expect a
// single system message at the beginning.
func mergeSystemMessages(msgs []prompting.Message) prompting.Message {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(m.Content)
	}
	return prompting.Message{Role: "system", Content: b.String()}
}

// workingMemoryMessage renders the session's enabled working-memory facts into a
// system message (band 0). The file is the source of truth, re-read fresh each
// turn. ok is false on a read error or an empty fact set.
func (o *Orchestrator) workingMemoryMessage() (prompting.Message, bool) {
	return prompting.WorkingMemoryMessage(o.workingMemoryFacts())
}

// workingMemoryFacts loads the session's enabled working-memory facts — the shared
// grounding primitive for any LLM call that needs to resolve "the project"/"here" to a
// real path. Used for the conversational path (via workingMemoryMessage, folded with full
// history through withContext) and, more narrowly, for the tool proposer (facts only, no
// history — a single_tool resolution is a narrow job, not a conversation; see CLAUDE.md's
// Context Curation principle). nil (via a load error) is a valid "no grounding" result, not
// a fatal one — callers already treat an empty/absent fact set as "omit the message."
func (o *Orchestrator) workingMemoryFacts() []prompting.Fact {
	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		return nil
	}
	enabled := wm.Enabled()
	facts := make([]prompting.Fact, 0, len(enabled))
	for _, f := range enabled {
		facts = append(facts, prompting.Fact{Key: f.Key, Value: pinAnnotatedValue(f)})
	}
	return facts
}

// projectRoot returns the session's project boundary for approval scoping
// (internal/tools.ClassifyPath) — the bootstrap-seeded "cwd" working-memory
// fact (session.BootstrapFacts), not a fresh os.Getwd() call, so the boundary
// stays whatever was true when the session started and stays consistent with
// the same fact the working-memory surface shows/lets the user edit. Returns
// "" if the fact is absent, disabled, or deleted — ClassifyPath then
// conservatively classifies every path as outside the project, never wider.
// Reads the raw fact value directly (not via workingMemoryFacts/
// pinAnnotatedValue) since that path appends a "(pinned ..., age ...)" display
// suffix for a pin-owned fact — a real filesystem path must stay exact.
func (o *Orchestrator) projectRoot() string {
	wm, err := o.store.LoadWorkingMemory(o.id.ID)
	if err != nil {
		return ""
	}
	for _, f := range wm.Enabled() {
		if f.Key == "cwd" {
			return f.Value
		}
	}
	return ""
}

// pinAnnotatedValue appends a static/live + age tag to a pinned fact's value, so
// the model has the same staleness signal the working-memory surface shows the
// user (docs/implementation/03_configuration_and_storage.md "Pinning to Working
// Memory"). A plain user/agent fact's value is returned unchanged.
func pinAnnotatedValue(f session.Fact) string {
	if f.Owner != session.OwnerPin {
		return f.Value
	}
	liveness := "static"
	if f.Live {
		liveness = "live"
	}
	return fmt.Sprintf("%s (pinned %s, age %s)", f.Value, liveness, f.Age().Round(time.Second))
}
