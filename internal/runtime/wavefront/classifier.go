package wavefront

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"agentx/internal/jsonx"
)

// Know is a fact already true, or derivable purely by combining facts already known
// — never a guess.
type Know struct{ Name, Value string }

// Command is a resolved tool call, matching the existing planner's task payload
// shape ({"tool","args"}) — the same catalog/executor path handles both engines'
// resolved calls identically; there is no separate "raw shell command" concept.
type Command struct {
	Tool string
	Args map[string]string
}

// Need is an open question: something required to answer the question at hand
// that is not yet in working memory, and that cannot yet be resolved by a
// concrete tool call (see Tool for that case). It always becomes a new child
// node, classified again once more is known.
type Need struct {
	Name string
}

// Tool is a resolved tool call: the model both identified something needed and
// can name a concrete command that answers it directly from working memory.
// Command.Args must come from the working-memory text Classify was given, never
// from a fact expected from a sibling, not-yet-executed Need — the core
// grounding rule ADR 0012 exists to enforce. This is the contract-level
// statement of that rule, not a runtime check: enforcement is prompt discipline
// (the classify prompt states it explicitly) plus the fact that nothing
// downstream binds an unresolved value into Args.
type Tool struct {
	Name    string
	Command Command
}

// Result is one Classify call's KNOW/NEED/TOOL classification of a single
// question.
type Result struct {
	Knows []Know
	Needs []Need
	Tools []Tool
}

// Classifier asks, for one open question against the current blackboard (rendered
// working-memory text — the graph itself, per the ADR 0012 amendment, not a
// separate struct), what's already known/synthesizable, what's still needed, and
// what's needed but already directly answerable by a tool call. Unlike
// planner.Planner.Plan, a Tool's Command must be answerable from wm alone.
type Classifier interface {
	Classify(ctx context.Context, wm, question string) (Result, error)
}

// Wire shapes for the model's JSON reply.

type knowPayload struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type needPayload struct {
	Name string `json:"name"`
}

type toolPayload struct {
	Name string            `json:"name"`
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
}

// classifyItem is a discriminated union by key presence (JSON oneOf), mirroring
// planner.dagNode: exactly one of Know/Need/Tool is set — a nil pointer means
// absent, not a null/empty sentinel value. Promoting TOOL to a sibling of
// KNOW/NEED (rather than an optional "command" field nested inside NEED, the
// prior encoding) lets the schema require tool+args unconditionally whenever
// TOOL is chosen at all, the same enforcement KNOW and NEED already get.
type classifyItem struct {
	Know *knowPayload `json:"KNOW,omitempty"`
	Need *needPayload `json:"NEED,omitempty"`
	Tool *toolPayload `json:"TOOL,omitempty"`
}

type classifyJSON struct {
	Classification []classifyItem `json:"classification"`
}

// ClassifySchema is the JSON Schema handed to Ollama's Format field to constrain
// generation to the oneOf(KNOW,NEED,TOOL) wire shape. A fixed schema, built
// once, same convention as planner.PlanSchema.
func ClassifySchema() json.RawMessage {
	return json.RawMessage(classifySchemaJSON)
}

const classifySchemaJSON = `{
  "type": "object", "required": ["classification"],
  "properties": {"classification": {"type": "array", "items": {"type": "object",
    "oneOf": [
      {"required": ["KNOW"], "properties": {
        "KNOW": {"type": "object", "required": ["name", "value"],
          "properties": {"name": {"type": "string", "minLength": 1}, "value": {"type": "string"}}}}},
      {"required": ["NEED"], "properties": {
        "NEED": {"type": "object", "required": ["name"],
          "properties": {"name": {"type": "string", "minLength": 1}}}}},
      {"required": ["TOOL"], "properties": {
        "TOOL": {"type": "object", "required": ["name", "tool", "args"],
          "properties": {
            "name": {"type": "string", "minLength": 1},
            "tool": {"type": "string", "minLength": 1},
            "args": {"type": "object"}
          }}}}
    ]}}}
}`

// Parse turns the model's classify JSON into a Result. Rejects an item with
// zero or more than one of KNOW/NEED/TOOL, an empty name on any of them, and a
// TOOL with no tool id — failing loudly rather than emitting a partially broken
// classification. These checks are a defensive backstop: ClassifySchema's
// constrained decoding should prevent most of them, but Parse must not trust that
// blindly (mirrors planner.Parse's own posture) — a model/Ollama version can still
// fall back to unconstrained text, recovered via jsonx.FirstObject below.
func Parse(data []byte) (Result, error) {
	obj := jsonx.FirstObject(string(data))
	if obj == "" {
		return Result{}, fmt.Errorf("wavefront: no JSON object in classify response")
	}
	var cj classifyJSON
	if err := json.Unmarshal([]byte(obj), &cj); err != nil {
		return Result{}, fmt.Errorf("wavefront: parse classify json: %w", err)
	}

	var res Result
	for i, item := range cj.Classification {
		set := 0
		for _, present := range []bool{item.Know != nil, item.Need != nil, item.Tool != nil} {
			if present {
				set++
			}
		}
		if set != 1 {
			return Result{}, fmt.Errorf("wavefront: item %d must have exactly one of KNOW, NEED, or TOOL", i+1)
		}
		switch {
		case item.Know != nil:
			name := strings.TrimSpace(item.Know.Name)
			if name == "" {
				return Result{}, fmt.Errorf("wavefront: KNOW item %d has no name", i+1)
			}
			res.Knows = append(res.Knows, Know{Name: name, Value: item.Know.Value})
		case item.Need != nil:
			name := strings.TrimSpace(item.Need.Name)
			if name == "" {
				return Result{}, fmt.Errorf("wavefront: NEED item %d has no name", i+1)
			}
			res.Needs = append(res.Needs, Need{Name: name})
		case item.Tool != nil:
			name := strings.TrimSpace(item.Tool.Name)
			if name == "" {
				return Result{}, fmt.Errorf("wavefront: TOOL item %d has no name", i+1)
			}
			tool := strings.TrimSpace(item.Tool.Tool)
			if tool == "" {
				return Result{}, fmt.Errorf("wavefront: TOOL %q has no tool", name)
			}
			args := make(map[string]string, len(item.Tool.Args))
			maps.Copy(args, item.Tool.Args)
			res.Tools = append(res.Tools, Tool{Name: name, Command: Command{Tool: tool, Args: args}})
		}
	}
	return res, nil
}
