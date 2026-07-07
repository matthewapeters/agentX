// Package planner is the decomposition planner of ADR 0008 (amended 2026-07-07): it
// renders the prompt that asks the model to break a compound goal into a DAG of typed
// nodes, under Ollama JSON-schema-constrained decoding, and parses the model's plan JSON
// into child task records.
//
// Each DAG node the model emits carries exactly one of "task" (a leaf: a resolved tool
// call — id, args, explanation) or "step" (a sub-goal that still needs further breakdown —
// description, deliverable). This is a discriminated union by key presence (JSON oneOf),
// not a flat "kind" field, because it structurally prevents a task from acquiring
// step-shaped fields or vice versa. A Task node's tool call is pre-resolved by the planner
// itself (reusing the exact {"tool","args"} shape agentx-shell-commands.md's catalog and
// tools.Proposal already speak) rather than a goal string resolved later by a separate
// proposer call — this removes one LLM hop per leaf.
//
// Child ids are namespaced under the parent (parentID-1, parentID-2, …) so they are
// session-unique (the Phase-1 DAG requires it), and the model's local step ids are remapped
// onto them so dependency edges survive.
//
// Design: docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md
// (amendment section). Behavior: docs/architecture/behavior/adr/0008_task_decomposition_integration.feature.md.
package planner

import (
	"encoding/json"
	"fmt"
	"strings"

	"agentx/internal/jsonx"
	"agentx/internal/prompting/task"
)

// DefaultPromptTemplate is the built-in planner system prompt, used when no
// agentx-planner.md is configured (config/seed/agentx-planner.md ships the same content
// as the editable seed — this constant is only the fallback, mirroring
// classify.DefaultPrompt). {{catalog}} is the compact tool catalog; {{goal}} is the
// compound goal; {{context}} is the branch's investigation context (working memory +
// read-tool findings so far).
const DefaultPromptTemplate = `You are planning how to accomplish a compound goal by breaking it
into a DAG of at most 5 nodes.

For each node, prefer "task" — a single concrete tool call — over "step" — a sub-goal that
still needs further breakdown. Use "step" ONLY when the child genuinely cannot yet resolve
to one tool call; a node that could be a tool call must be a task, never a step. A step you
emit is drilled down further the next time it is dispatched (recursively, same rules,
same cap) — you do not need to (and must not) resolve it all the way down in one pass.

Every node needs a short local id ("s1", "s2", ...) — never leave it blank. A node's "deps"
lists the ids of sibling nodes that must finish first; an EMPTY OR OMITTED "deps" means no
ordering constraint, so that node can run in PARALLEL with any other ready sibling. Prefer
empty deps — add a dependency only when a node truly needs another node's result first.

Never restate the goal itself as a node. If the goal is already a single concrete action,
reply with exactly one task node naming that action directly.

Tools available for "task" nodes (use "tool" and "args" from this list only):
{{catalog}}
No shell syntax in args: no pipes, redirects, $VARIABLES, or command chaining.

Only use paths/facts you actually know from "What you know" below. Do not invent a path
that isn't given to you — prefer a task that lists a directory to discover real filenames
before a task that reads one.

What you know:
{{context}}

Goal:
{{goal}}

Reply with JSON matching this shape exactly (ids/paths below are illustrative, not to be
copied):
{"plan":{"name":"<3-6 words>","objective":"<one sentence>","dag":[
 {"id":"s1","deps":[],"task":{"tool":"list_dir","args":{"path":"."},"explanation":"<why>"}},
 {"id":"s2","deps":["s1"],"step":{"description":"<a coarse sub-goal still needing breakdown>","deliverable":"<what it must produce>"}}
]}}`

// Plan is a parsed decomposition: the child task records (DAG-shaped, with deps rewritten
// to the namespaced child ids), plus the model's plan name and objective (synthesis).
type Plan struct {
	Name      string
	Synthesis string
	Records   []task.Record
}

// taskPayload is a "task" node's resolved tool call.
type taskPayload struct {
	Tool        string            `json:"tool"`
	Args        map[string]string `json:"args"`
	Explanation string            `json:"explanation"`
}

// stepPayload is a "step" node's still-coarse sub-goal.
type stepPayload struct {
	Description string `json:"description"`
	Deliverable string `json:"deliverable"`
}

// dagNode is one entry of the model's dag array: exactly one of Task/Step must be set
// (a nil pointer means "absent" — Go's natural decoding of a JSON oneOf-by-key-presence).
type dagNode struct {
	ID   string       `json:"id"`
	Deps []string     `json:"deps"`
	Task *taskPayload `json:"task"`
	Step *stepPayload `json:"step"`
}

type planJSON struct {
	Plan struct {
		Name      string    `json:"name"`
		Objective string    `json:"objective"`
		Dag       []dagNode `json:"dag"`
	} `json:"plan"`
}

// maxDagNodes is the hard cap on a single decomposition's children (ADR 0008 amendment) —
// enforced here as a defensive backstop; PlanSchema's maxItems should prevent a violation
// from reaching Parse at all under constrained decoding, but a model/Ollama version that
// falls back to unconstrained text still needs the check.
const maxDagNodes = 5

// PlanSchema is the JSON Schema handed to Ollama's Format field to constrain generation to
// the oneOf(task,step) wire shape. It is a fixed schema (unlike the fan-group classifiers'
// per-contract schemaFor in internal/llm/invoke), so it is built once, not per call.
func PlanSchema() json.RawMessage {
	return json.RawMessage(planSchemaJSON)
}

const planSchemaJSON = `{
  "type": "object", "required": ["plan"],
  "properties": {"plan": {"type": "object", "required": ["name", "objective", "dag"],
    "properties": {
      "name": {"type": "string"}, "objective": {"type": "string"},
      "dag": {"type": "array", "maxItems": 5, "items": {"type": "object",
        "oneOf": [
          {"required": ["id", "task"], "properties": {
            "id": {"type": "string", "minLength": 1}, "deps": {"type": "array", "items": {"type": "string"}},
            "task": {"type": "object", "required": ["tool", "args", "explanation"],
              "properties": {"tool": {"type": "string"}, "args": {"type": "object"}, "explanation": {"type": "string"}}}}},
          {"required": ["id", "step"], "properties": {
            "id": {"type": "string", "minLength": 1}, "deps": {"type": "array", "items": {"type": "string"}},
            "step": {"type": "object", "required": ["description", "deliverable"],
              "properties": {"description": {"type": "string"}, "deliverable": {"type": "string"}}}}}
        ]}}
    }}}
}`

// Render fills template (DefaultPromptTemplate, or the configured agentx-planner.md
// content) for a goal, its investigation context, and the tool catalog.
func Render(template, goal, context, catalog string) string {
	r := strings.NewReplacer("{{goal}}", goal, "{{context}}", context, "{{catalog}}", catalog)
	return r.Replace(template)
}

// Parse turns the model's plan JSON into child task records under parentID. It rewrites the
// model's local node ids to session-unique child ids (parentID-1, parentID-2, …) and remaps
// deps onto them. It rejects an empty plan, more than maxDagNodes nodes, a node missing an
// id, a node with both or neither of task/step set, a duplicate id, or a dep referencing an
// unknown node (a dangling edge) — failing loudly rather than emitting a broken plan. These
// checks are a defensive backstop: PlanSchema's constrained decoding should prevent most of
// them, but Parse must not trust that blindly (a model/Ollama version can still fall back to
// unconstrained text, recovered via jsonx.FirstObject below).
func Parse(parentID string, data []byte) (Plan, error) {
	// Models wrap the plan in ```json fences despite "JSON only"; extract the object first
	// so a strict decode does not choke on the fence (see internal/jsonx).
	obj := jsonx.FirstObject(string(data))
	if obj == "" {
		return Plan{}, fmt.Errorf("planner: no JSON object in plan response")
	}
	var pj planJSON
	if err := json.Unmarshal([]byte(obj), &pj); err != nil {
		return Plan{}, fmt.Errorf("planner: parse plan json: %w", err)
	}
	nodes := pj.Plan.Dag
	if len(nodes) == 0 {
		return Plan{}, fmt.Errorf("planner: plan has no dag nodes")
	}
	if len(nodes) > maxDagNodes {
		return Plan{}, fmt.Errorf("planner: plan has %d dag nodes, maximum is %d", len(nodes), maxDagNodes)
	}

	// Map each local node id to its namespaced child id, rejecting missing/duplicate ids
	// and a node that doesn't declare exactly one of task/step.
	idFor := make(map[string]string, len(nodes))
	for i, n := range nodes {
		local := strings.TrimSpace(n.ID)
		if local == "" {
			return Plan{}, fmt.Errorf("planner: dag node %d has no id", i+1)
		}
		if (n.Task == nil) == (n.Step == nil) {
			return Plan{}, fmt.Errorf("planner: node %q must have exactly one of \"task\" or \"step\"", local)
		}
		if _, dup := idFor[local]; dup {
			return Plan{}, fmt.Errorf("planner: duplicate node id %q", local)
		}
		idFor[local] = fmt.Sprintf("%s-%d", parentID, i+1)
	}

	records := make([]task.Record, 0, len(nodes))
	for _, n := range nodes {
		deps := make([]string, 0, len(n.Deps))
		for _, d := range n.Deps {
			mapped, ok := idFor[strings.TrimSpace(d)]
			if !ok {
				return Plan{}, fmt.Errorf("planner: node %q depends on unknown node %q", n.ID, d)
			}
			deps = append(deps, mapped)
		}
		id := idFor[strings.TrimSpace(n.ID)]
		var rec task.Record
		switch {
		case n.Task != nil:
			args := make(map[string]any, len(n.Task.Args))
			for k, v := range n.Task.Args {
				args[k] = v
			}
			rec = task.Record{
				ID: id, Goal: n.Task.Explanation, Type: task.Query, Kind: task.KindTask,
				Status: task.Proposed, Deps: deps,
				Params:     map[string]any{"tool": n.Task.Tool, "args": args},
				Provenance: task.Provenance{Source: "planner"},
			}
		case n.Step != nil:
			if strings.TrimSpace(n.Step.Description) == "" {
				return Plan{}, fmt.Errorf("planner: step %q has no description", n.ID)
			}
			rec = task.Record{
				ID: id, Goal: n.Step.Description, Type: task.Query, Kind: task.KindStep,
				Status: task.Proposed, Deps: deps,
				Params:     map[string]any{"deliverable": n.Step.Deliverable},
				Provenance: task.Provenance{Source: "planner"},
			}
		}
		records = append(records, rec)
	}

	return Plan{Name: pj.Plan.Name, Synthesis: pj.Plan.Objective, Records: records}, nil
}
