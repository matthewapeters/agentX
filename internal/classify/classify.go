// Package classify implements the v1 prompt classification step: it asks the
// model to label a user prompt with a route, parses a tolerant strict-JSON
// verdict, retries on failure, and falls back to respond_directly so the prompt
// cycle never stalls. See docs/implementation/04_llm_prompt_tooling_runtime.md
// (Classification Cycle) and docs/build-plan/03_chat_surface_backlog.md (CHT-D3).
package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"agentx/internal/prompting"
)

// Route is a routable classification outcome. The set is a fixed contract the
// runtime knows how to execute; the prompt may only emit one of these.
type Route string

const (
	// RespondDirectly streams a conversational answer (the only v1-executable route).
	RespondDirectly Route = "respond_directly"
	// SingleTool is reserved (M3b); falls back to RespondDirectly until tools land.
	SingleTool Route = "single_tool"
	// InvokePlanner is reserved; falls back to RespondDirectly until the planner lands.
	InvokePlanner Route = "invoke_planner"
)

func (r Route) valid() bool {
	switch r {
	case RespondDirectly, SingleTool, InvokePlanner:
		return true
	default:
		return false
	}
}

// Verdict is the parsed classification result.
type Verdict struct {
	Route      Route   `json:"route"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

// DefaultPrompt is the built-in classification system prompt used when no
// agentx-classification.md is configured.
const DefaultPrompt = `You are AgentX's prompt classifier. Read the user's message and decide how the
assistant should handle it. Reply with ONE JSON object and nothing else:

{"route": "<route>", "confidence": <0..1>, "rationale": "<=10 words"}

Routes:
- respond_directly — conversation, questions, explanations, or anything answerable
  without running tools or a multi-step plan.
- single_tool — the request needs exactly one tool/command (e.g. read or edit a
  file, run a command).
- invoke_planner — a complex, multi-step task needing decomposition into a plan.

Prefer respond_directly when unsure. Output only the JSON object.`

// ChatFunc runs a non-streaming chat completion for the assembled messages and
// returns the full response text.
type ChatFunc func(ctx context.Context, messages []prompting.Message) (string, error)

// Classifier drives the classification call with retry + fallback.
type Classifier struct {
	assembler *prompting.Assembler
	chat      ChatFunc
	retries   int
}

// New returns a Classifier using prompt as the classification system prompt
// (DefaultPrompt when empty) and a retry budget of retries (>= 0).
func New(prompt string, retries int, chat ChatFunc) *Classifier {
	if strings.TrimSpace(prompt) == "" {
		prompt = DefaultPrompt
	}
	if retries < 0 {
		retries = 0
	}
	return &Classifier{assembler: prompting.New(prompt), chat: chat, retries: retries}
}

// Classify returns a verdict for userText. It retries on model error or an
// unparseable/invalid verdict up to the configured budget, then falls back to
// respond_directly so the cycle always resolves.
func (c *Classifier) Classify(ctx context.Context, userText string) Verdict {
	messages := c.assembler.Assemble(userText)
	for attempt := 0; attempt <= c.retries; attempt++ {
		raw, err := c.chat(ctx, messages)
		if err != nil {
			if ctx.Err() != nil {
				break // canceled: stop retrying
			}
			continue
		}
		if v, perr := Parse(raw); perr == nil {
			return v
		}
	}
	return Verdict{Route: RespondDirectly, Rationale: "classification fallback"}
}

// Parse extracts the first balanced JSON object from raw (tolerating surrounding
// prose or code fences), unmarshals it, and validates the route enum.
func Parse(raw string) (Verdict, error) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return Verdict{}, fmt.Errorf("no JSON object in classification response")
	}
	var v Verdict
	if err := json.Unmarshal([]byte(obj), &v); err != nil {
		return Verdict{}, fmt.Errorf("invalid classification JSON: %w", err)
	}
	if !v.Route.valid() {
		return Verdict{}, fmt.Errorf("unknown route %q", v.Route)
	}
	return v, nil
}

// extractJSONObject returns the first balanced {...} substring, respecting string
// literals and escapes, or "" if none is present.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case ch == '\\':
				esc = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
