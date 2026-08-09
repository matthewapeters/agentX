package tools

import (
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
		{ID: "read_file", Description: "Return a file's contents.",
			Command: "cat", Argv: []string{"cat", "--", "{path}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "list_dir", Description: "List a directory's contents.",
			Command: "ls", Argv: []string{"ls", "-la", "--", "{path}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		// tree: a bounded structural overview in one call, instead of one list_dir per
		// directory (each an LLM-authored guess at what to look at next — the repeated
		// cause of hallucinated paths this session: package.json/tsconfig guessed instead
		// of observed). Depth capped at 3 and common generated/vendored dirs excluded so
		// output stays bounded and signal-dense on a large repo; no arg for either, since
		// BuildArgv requires every "{name}" placeholder to be supplied and this tool's
		// only variable input is the target path.
		{ID: "tree", Description: "Show a directory's structure (depth-limited to 3, vendored dirs excluded).",
			Command: "tree", Argv: []string{"tree", "-L", "3",
				"-I", strings.Join(excludedDirs, "|"), "--", "{path}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "find_path", Description: "Find files by name under a directory.",
			Command: "find", Argv: []string{"find", "{root}", "-name", "{name}"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "root", Kind: KindPath, Required: true}, {Name: "name", Kind: KindString, Required: true}}},
		// grep_files: a Go builtin (no external grep/rg dependency), so
		// behavior is identical on every OS Go itself supports — see
		// docs/architecture/behavior/tool_grep_files.feature.md.
		// max_results is optional (KindInt, not Required) — omitted or
		// non-positive falls back to defaultGrepMaxResults in executor.go.
		{ID: "grep_files", Description: "Search file contents for a regular-expression pattern (RE2 syntax), returning matching lines as path:line: text.",
			Builtin: "grep_files", Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{
				{Name: "path", Kind: KindPath, Required: true},
				{Name: "pattern", Kind: KindString, Required: true},
				{Name: "max_results", Kind: KindInt},
			}},
		// date: no variable input, so Argv carries no "{name}" placeholder and Args is
		// nil — BuildArgv only requires a placeholder's arg when the template uses one.
		{ID: "date", Description: "Return the current date/time.",
			Command: "date", Argv: []string{"date"},
			Risk: RiskRead, TimeoutSeconds: 5},
		{ID: "git_status", Description: "Show a directory's git working-tree status.",
			Command: "git", Argv: []string{"git", "-C", "{path}", "status"},
			Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		// git: unlike git_status (one fixed, read-only subcommand), this runs
		// ANY git subcommand with any arguments — add, commit, push, reset,
		// branch -D, rebase, whatever the model needs — deliberately
		// unrestricted in scope (explicit product decision: no subcommand
		// allowlist/denylist). Blast radius is gated the same way every other
		// write-risk tool here is: RiskWrite + RequiresApproval, not by
		// limiting what git commands are reachable. args is a JSON array of
		// argv tokens (e.g. ["commit", "-m", "message"]), not a shell string
		// — BuildArgv's "{name}" template only substitutes one token per
		// placeholder, so a multi-word commit message can't round-trip
		// through it. This ships as a Builtin that JSON-decodes args and
		// execs the vector itself (no shell — so no quoting/injection
		// surface regardless of which subcommand or flags are chosen).
		{ID: "git", Description: "Run any git subcommand with any arguments (add, commit, push, pull, reset, branch, log, diff, rebase, tag, ...) against a repository — no subcommand is restricted. `args` is a JSON array of argv tokens, e.g. [\"commit\", \"-m\", \"message\"].",
			Builtin: "git", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 120,
			Args: []ArgSpec{
				{Name: "path", Kind: KindPath, Required: true},
				{Name: "args", Kind: KindString, Required: true},
			}},
		{ID: "read_output", Description: "Re-read a previous tool result stored in the session, by ref.",
			Builtin: "read_output", Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "ref", Kind: KindString, Required: true}, {Name: "offset", Kind: KindInt}, {Name: "limit", Kind: KindInt}}},

		// Write & modify (mutating; approval required). write_file is a Go built-in;
		// apply_patch feeds the diff via stdin (no shell).
		{ID: "write_file", Description: "Create or overwrite a file with the given content.",
			Builtin: "write_file", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}, {Name: "content", Kind: KindString, Required: true}}},
		{ID: "apply_patch", Description: "Apply a unified diff to the working tree.",
			Command: "patch", Argv: []string{"patch", "-p0"}, StdinArg: "patch",
			Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "patch", Kind: KindString, Required: true}}},
		// edit_file is a Go built-in (no subprocess, no regex): old_string must
		// match an exact, literal, unique substring of the file (unless
		// replace_all is set), removing the sed escaping/addressing surface
		// that made the original implementation unreliable — see the
		// editFile doc comment in executor.go.
		{ID: "edit_file", Description: "Replace an exact, unique block of text in a file with new text.",
			Builtin: "edit_file", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{
				{Name: "path", Kind: KindPath, Required: true},
				{Name: "old_string", Kind: KindString, Required: true},
				{Name: "new_string", Kind: KindString, Required: true},
				{Name: "replace_all", Kind: KindString},
			}},
		// delete_file/move_file are Go built-ins (os.Remove/os.Rename — already
		// cross-platform in the Go standard library, no OS-specific Command/Argv
		// needed). Both are deliberately bounded to a single file, never a
		// directory — see docs/architecture/behavior/tool_delete_file.feature.md
		// and tool_move_file.feature.md.
		{ID: "delete_file", Description: "Delete a single file. Refuses to delete directories.",
			Builtin: "delete_file", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "move_file", Description: "Move or rename a single file. Refuses to overwrite an existing destination or move a directory.",
			Builtin: "move_file", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{
				{Name: "from", Kind: KindPath, Required: true},
				{Name: "to", Kind: KindPath, Required: true},
			}},

		// Verification (executes the repo's build/test gate; approval required).
		// Fixed to exactly `make all` — no target argument — so there is no
		// injection surface and the model can only ever run the same canonical
		// gate CLAUDE.md already holds every merge to (docs/implementation/
		// 09_makefile_and_quality_gate_contract.md), never an arbitrary Makefile
		// target. This is a materially bigger blast radius than any other tool
		// here: it executes real test code, not just a bounded read/write/fetch,
		// so it stays approval-gated like write_file/apply_patch despite running
		// nothing the model authored itself. TimeoutSeconds is generous (a cold
		// build + full suite is minutes, not the ~30s other tools budget for).
		{ID: "run_checks", Description: "Run the repository's canonical build/test gate (`make all`): vets, builds, and runs the full test suite.",
			Command: "make", Argv: []string{"make", "-C", "{path}", "all"},
			Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 300,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},

		// Network (egress; approval required).
		{ID: "http_get", Description: "Fetch a URL and return the response body.",
			Command: "curl", Argv: []string{"curl", "-sSL", "--", "{url}"},
			Risk: RiskNetwork, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "url", Kind: KindString, Required: true}}},
		{ID: "download", Description: "Download a URL's contents to a file.",
			Command: "wget", Argv: []string{"wget", "-O", "{output}", "--", "{url}"},
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
