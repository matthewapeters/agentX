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
