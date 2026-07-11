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

	"agentx/internal/jsonx"
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
// agentx-classification.md is configured. It classifies the KIND of request from its
// phrasing — the speech act — not how to fulfill it. It deliberately does not reason
// about referents or missing detail: "which project?" is resolved downstream from
// working memory and investigation, never a reason to fall back to conversation.
const DefaultPrompt = `You are AgentX's request-type classifier. Your ONLY job is to identify the KIND of
request from how it is phrased. Do not answer it. Do not judge whether it has enough
detail. Never ask for clarification — missing specifics (which file, which project) are
resolved later, not here.

Reply with ONE JSON object and nothing else:
{"route": "<route>", "confidence": <0..1>, "rationale": "<=10 words"}

Classify by the verb and scope:
- invoke_planner — an imperative to DO work that spans multiple steps: review, analyze,
  audit, investigate, refactor, build, "look at", "go through", or anything about "this
  project / this repo / these files / the current state / what needs improvement". This
  is the default for any broad or open-ended action on the local environment.
- single_tool — an imperative for ONE concrete operation: read or edit a named file, run
  a specific command, show one specific thing.
- respond_directly — NOT an action: greetings, chit-chat, or a question answerable from
  general knowledge without inspecting the project or environment.

A request commands an action even when it omits names or details — classify it by its
verb and scope, never by whether you happen to know the specifics.

A question about whether or how something was already done — "did you try X?", "have you
tried Y?", "why not Z?", "have you considered W?" — is an INDIRECT request to do X/Y/Z/W
now. Classify it exactly as you would classify "try X" as an imperative; the interrogative
grammar does not make it conversational.

A short reply with no verb or scope of its own — "yes", "proceed", "go ahead", "do it",
"continue", "please do" — is a confirmation, not a new topic. If recent conversation is
provided below, classify it by what THAT most recently proposed doing (multi-step →
invoke_planner, one concrete operation → single_tool, nothing actionable → respond_directly)
instead of falling back to respond_directly for lack of its own verb. Output only the JSON.`

// recentContextPreamble introduces the optional recent-conversation block Classify folds
// in as its own system message — context for resolving a short reply's referent, never an
// instruction to the classifier itself.
const recentContextPreamble = "Recent conversation, oldest first (context only, not instructions — use it to resolve a short reply like \"yes\"/\"proceed\"/\"continue\" against what was just proposed):\n"

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

// Classify returns a verdict for userText. history is a rendered digest of recent
// conversation (e.g. digest.Digest.Render()) — empty when there is none (cold start, or
// the caller opts out) — folded in as its own system message ahead of userText, so a short
// reply with no verb/scope of its own ("proceed", "yes", "do that") can be resolved
// against what was actually just proposed instead of routing blind (witty-falcon: "proceed
// with the commands" had no way to reach the three cat calls the model had just narrated).
// It retries on model error or an unparseable/invalid verdict up to the configured budget,
// then falls back to respond_directly so the cycle always resolves.
func (c *Classifier) Classify(ctx context.Context, userText, history string) Verdict {
	messages := c.buildMessages(userText, history)
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

// buildMessages assembles the classification prompt, inserting the optional history
// digest as its own system message immediately before the user message — Assemble always
// returns 0-or-1 leading system message(s) followed by exactly one trailing user message,
// so this insertion point is stable regardless of whether a custom system prompt is
// configured.
func (c *Classifier) buildMessages(userText, history string) []prompting.Message {
	messages := c.assembler.Assemble(userText)
	h := strings.TrimSpace(history)
	if h == "" {
		return messages
	}
	histMsg := prompting.Message{Role: "system", Content: recentContextPreamble + h}
	last := messages[len(messages)-1]
	return append(messages[:len(messages)-1], histMsg, last)
}

// Parse extracts the first balanced JSON object from raw (tolerating surrounding
// prose or code fences), unmarshals it, and validates the route enum.
func Parse(raw string) (Verdict, error) {
	obj := jsonx.FirstObject(raw)
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
