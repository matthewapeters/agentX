package session

// ContextComponent is one content class's contribution to the assembled context
// window (working memory, instructions, user, assistant, …). It is the unit the
// read-only context-visualizer surface renders as a band. Only classes that
// actually contribute to the current context are emitted; absent classes render
// as zero on the surface.
type ContextComponent struct {
	Class string `json:"class"` // stable id: working-memory|instructions|user|assistant|attachments|thinking|tools
	Chars int    `json:"chars"` // total characters this class contributes to the context
}

// ContextReport is the composition of a session's standing context window: the
// per-class byte contributions plus the model and its window size, for the
// context-visualizer to project against the budget.
type ContextReport struct {
	Model        string             `json:"model"`         // active model name
	WindowTokens int                `json:"window_tokens"` // model's context window (0 → unknown)
	Components   []ContextComponent `json:"components"`
}

// EstTokens is the char/4 token estimate for a component (Ollama has no universal
// local tokenizer, so the visualizer labels its figures "est.").
func (c ContextComponent) EstTokens() int { return (c.Chars + 3) / 4 }

// TotalChars sums every component's character contribution.
func (r ContextReport) TotalChars() int {
	total := 0
	for _, c := range r.Components {
		total += c.Chars
	}
	return total
}

// TotalTokens is the char/4 estimate of the whole assembled context.
func (r ContextReport) TotalTokens() int { return (r.TotalChars() + 3) / 4 }
