package prompting

import "strings"

// DefaultSystemPrompt is the built-in system prompt used when none is configured.
const DefaultSystemPrompt = "You are AgentX, a local-first AI assistant. Be concise and helpful."

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
