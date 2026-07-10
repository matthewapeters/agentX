package prompting

import "strings"

// DefaultSystemPrompt is the built-in system prompt used when none is configured.
const DefaultSystemPrompt = "You are AgentX, a local-first AI assistant. Be concise and helpful."

// DefaultThinkingPrompt is the built-in thinking guidance used when no
// agentx-thinking.md is configured. It steers reasoning toward the "useful but
// bounded" sweet spot and scales depth to the classified route.
const DefaultThinkingPrompt = "Think briefly and purposefully before answering. " +
	"Reason in at most a few short steps and stop as soon as you can give a clear " +
	"answer or a concrete next action. Do not restate the question or narrate at " +
	"length. Scale your reasoning to the task: a direct question needs little or " +
	"none; a multi-step task warrants a short plan."

// Message is a single assembled chat message.
type Message struct {
	Role    string
	Content string
}

// Fact is a single working-memory entry rendered into a turn's context.
type Fact struct {
	Key   string
	Value string
}

// WorkingMemoryMessage renders enabled working-memory facts into a single system
// message (context band 0 — see docs/ux/03_PANEL_DETAILS.md). It returns
// ok=false when there are no facts, so the caller can omit the message entirely
// rather than emit an empty one.
func WorkingMemoryMessage(facts []Fact) (Message, bool) {
	if len(facts) == 0 {
		return Message{}, false
	}
	var b strings.Builder
	b.WriteString("Working memory — durable facts about this session:")
	for _, f := range facts {
		b.WriteString("\n- ")
		b.WriteString(f.Key)
		b.WriteString(": ")
		b.WriteString(f.Value)
	}
	return Message{Role: "system", Content: b.String()}, true
}

// PlanFindingsMessage renders a decomposition plan's live findings-so-far (see
// internal/planfindings) into its own system message, separate from
// WorkingMemoryMessage — persistent session facts and one specific plan's in-flight
// discoveries are different context shapes (Context Curation, CLAUDE.md) and
// deliberately not folded into the same message. Returns ok=false for an empty
// string, so the caller can omit the message entirely.
func PlanFindingsMessage(findings string) (Message, bool) {
	if strings.TrimSpace(findings) == "" {
		return Message{}, false
	}
	return Message{Role: "system", Content: findings}, true
}

// Assembler builds the message sequence for a user turn.
type Assembler struct {
	systemPrompt string
}

// New returns an Assembler with the given system prompt. An empty system prompt
// is omitted from assembled output.
func New(systemPrompt string) *Assembler {
	return &Assembler{systemPrompt: strings.TrimSpace(systemPrompt)}
}

// Assemble returns the messages for a user turn: an optional system message
// followed by the user message.
func (a *Assembler) Assemble(userText string) []Message {
	msgs := make([]Message, 0, 2)
	if a.systemPrompt != "" {
		msgs = append(msgs, Message{Role: "system", Content: a.systemPrompt})
	}
	msgs = append(msgs, Message{Role: "user", Content: userText})
	return msgs
}

// AssembleWithThinking returns the messages for a user turn with thinking
// guidance folded into the system message and the classified route appended as a
// calibration hint. An empty guidance behaves like Assemble.
func (a *Assembler) AssembleWithThinking(userText, guidance, route string) []Message {
	guidance = strings.TrimSpace(guidance)
	if guidance == "" {
		return a.Assemble(userText)
	}
	if route != "" {
		guidance += "\n\nClassified route: " + route
	}
	sys := a.systemPrompt
	if sys != "" {
		sys += "\n\n" + guidance
	} else {
		sys = guidance
	}
	return []Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: userText},
	}
}
