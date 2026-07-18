package tools

import (
	"fmt"
	"sort"
	"strings"
)

// Registry is a lookup of curated tool descriptors by id.
type Registry struct {
	byID map[string]Descriptor
}

// DefaultRegistry returns the built-in curated toolset. It mirrors the LLM-facing
// catalog seeded at config/seed/agentx-shell-commands.md. Backing commands run as
// argv vectors (no shell); built-ins (empty Command) are implemented in Go by the
// executor (TOOL-2).
func DefaultRegistry() *Registry {
	descs := []Descriptor{
		// Read & search (read-only). "--" stops option parsing so a path value is
		// never treated as a flag.
		{ID: "read_file", Command: "cat", Argv: []string{"cat", "--", "{path}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "list_dir", Command: "ls", Argv: []string{"ls", "-la", "--", "{path}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		// tree: a bounded structural overview in one call, instead of one list_dir per
		// directory (each an LLM-authored guess at what to look at next — the repeated
		// cause of hallucinated paths this session: package.json/tsconfig guessed instead
		// of observed). Depth capped at 3 and common generated/vendored dirs excluded so
		// output stays bounded and signal-dense on a large repo; no arg for either, since
		// BuildArgv requires every "{name}" placeholder to be supplied and this tool's
		// only variable input is the target path.
		{ID: "tree", Command: "tree", Argv: []string{"tree", "-L", "3",
			"-I", "node_modules|.git|vendor|__pycache__|.venv|dist|build|.next|target", "--", "{path}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "find_path", Command: "find", Argv: []string{"find", "{root}", "-name", "{name}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "root", Kind: KindPath, Required: true}, {Name: "name", Kind: KindString, Required: true}}},
		// date: no variable input, so Argv carries no "{name}" placeholder and Args is
		// nil — BuildArgv only requires a placeholder's arg when the template uses one.
		{ID: "date", Command: "date", Argv: []string{"date"},
			Risk: RiskRead, TimeoutSeconds: 5},
		{ID: "git_status", Command: "git", Argv: []string{"git", "-C", "{path}", "status"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "read_output", Builtin: "read_output", Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "ref", Kind: KindString, Required: true}, {Name: "offset", Kind: KindInt}, {Name: "limit", Kind: KindInt}}},

		// Write & modify (mutating; approval required). write_file is a Go built-in;
		// apply_patch feeds the diff via stdin (no shell).
		{ID: "write_file", Builtin: "write_file", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}, {Name: "content", Kind: KindString, Required: true}}},
		{ID: "apply_patch", Command: "patch", Argv: []string{"patch", "-p0"}, StdinArg: "patch",
			Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "patch", Kind: KindString, Required: true}}},
		{ID: "edit_file", Command: "sed", Argv: []string{"sed", "-i", "-e", "{script}", "--", "{path}"},
			Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}, {Name: "script", Kind: KindString, Required: true}}},

		// Network (egress; approval required).
		{ID: "http_get", Command: "curl", Argv: []string{"curl", "-sSL", "--", "{url}"},
			Risk: RiskNetwork, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "url", Kind: KindString, Required: true}}},
		{ID: "download", Command: "wget", Argv: []string{"wget", "-O", "{output}", "--", "{url}"},
			Risk: RiskNetwork, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "url", Kind: KindString, Required: true}, {Name: "output", Kind: KindPath, Required: true}}},
	}
	r := &Registry{byID: make(map[string]Descriptor, len(descs))}
	for _, d := range descs {
		r.byID[d.ID] = d
	}
	return r
}

// Lookup returns the descriptor for id.
func (r *Registry) Lookup(id string) (Descriptor, bool) {
	d, ok := r.byID[id]
	return d, ok
}

// Available returns the descriptors permitted by the enabled tiers, sorted by id.
// When readOnly is true only read-risk tools are returned.
func (r *Registry) Available(readOnly bool) []Descriptor {
	out := make([]Descriptor, 0, len(r.byID))
	for _, d := range r.byID {
		if readOnly && d.Risk != RiskRead {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Validate checks a proposed call to tool against its descriptor's argument
// contract, returning nil when args satisfies it and otherwise a specific,
// human-readable reason (unknown tool, or Descriptor.Validate's own "missing
// required argument %q" / "unknown argument %q" text). Callers that need to
// tell a proposing model exactly what was wrong — not just that something
// was — can use this error text as-is.
func (r *Registry) Validate(tool string, args map[string]string) error {
	d, ok := r.Lookup(tool)
	if !ok {
		return fmt.Errorf("unknown tool %q", tool)
	}
	return d.Validate(args)
}

// Describe renders tool's argument contract compactly (e.g.
// "list_dir(path required)"), for feedback to a model that proposed an invalid
// call — "" for an unknown tool, so a caller can distinguish "no args" from
// "not a real tool".
func (r *Registry) Describe(tool string) string {
	d, ok := r.Lookup(tool)
	if !ok {
		return ""
	}
	if len(d.Args) == 0 {
		return fmt.Sprintf("%s takes no arguments", tool)
	}
	parts := make([]string, len(d.Args))
	for i, a := range d.Args {
		req := "optional"
		if a.Required {
			req = "required"
		}
		parts[i] = fmt.Sprintf("%s %s", a.Name, req)
	}
	return fmt.Sprintf("%s(%s)", tool, strings.Join(parts, ", "))
}
