package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"agentx/internal/prompting"
)

// Proposal is a parsed tool call: a tool id and its string-valued arguments.
type Proposal struct {
	Tool string
	Args map[string]string
}

// ChatFunc runs a non-streaming chat completion and returns the full text.
type ChatFunc func(ctx context.Context, messages []prompting.Message) (string, error)

// Proposer asks the model to choose one tool call for a prompt, using the tool
// catalog as its system prompt. It mirrors the classifier: strict-JSON parsing
// with retry, falling back to "no tool" so the cycle can answer directly.
type Proposer struct {
	assembler *prompting.Assembler
	chat      ChatFunc
	retries   int
}

// NewProposer returns a proposer using catalog as the tool-catalog system prompt
// (DefaultCatalog when empty) and a retry budget of retries (>= 0).
func NewProposer(catalog string, retries int, chat ChatFunc) *Proposer {
	if strings.TrimSpace(catalog) == "" {
		catalog = DefaultCatalog
	}
	if retries < 0 {
		retries = 0
	}
	return &Proposer{assembler: prompting.New(catalog), chat: chat, retries: retries}
}

// Propose returns the model's chosen tool call for userText. The bool is false
// when no tool should run (the model replied {"tool":"none"}, or every attempt
// failed to produce a parseable proposal) — the caller then answers directly.
func (p *Proposer) Propose(ctx context.Context, userText string) (Proposal, bool) {
	msgs := p.assembler.Assemble(userText)
	for attempt := 0; attempt <= p.retries; attempt++ {
		raw, err := p.chat(ctx, msgs)
		if err != nil {
			continue
		}
		prop, perr := ParseProposal(raw)
		if perr != nil {
			continue
		}
		if prop.Tool == "" || prop.Tool == "none" {
			return Proposal{}, false
		}
		return prop, true
	}
	return Proposal{}, false
}

// ParseProposal extracts the first balanced JSON object from raw and decodes it
// into a Proposal. Argument values are normalized to strings (JSON numbers become
// their integer/float text), matching the executor's string-args model.
func ParseProposal(raw string) (Proposal, error) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return Proposal{}, fmt.Errorf("no JSON object in proposal")
	}
	var decoded struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(obj), &decoded); err != nil {
		return Proposal{}, fmt.Errorf("decode proposal: %w", err)
	}
	if decoded.Tool == "" {
		return Proposal{}, fmt.Errorf("proposal missing tool")
	}
	args := make(map[string]string, len(decoded.Args))
	for k, v := range decoded.Args {
		args[k] = stringifyArg(v)
	}
	return Proposal{Tool: decoded.Tool, Args: args}, nil
}

func stringifyArg(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// extractJSONObject returns the first top-level {...} object in s, honoring
// strings and escapes so braces inside strings do not unbalance the scan.
func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// skip
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
