package tools

import "sort"

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
		// Read & search (read-only).
		{ID: "read_file", Command: "cat", Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "list_dir", Command: "ls", Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}}},
		{ID: "find_path", Command: "find", Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "root", Kind: KindPath, Required: true}, {Name: "name", Kind: KindString, Required: true}}},
		{ID: "read_output", Command: "", Risk: RiskRead, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "ref", Kind: KindString, Required: true}, {Name: "offset", Kind: KindInt}, {Name: "limit", Kind: KindInt}}},

		// Write & modify (mutating; approval required).
		{ID: "write_file", Command: "", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}, {Name: "content", Kind: KindString, Required: true}}},
		{ID: "apply_patch", Command: "patch", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "patch", Kind: KindString, Required: true}}},
		{ID: "edit_file", Command: "sed", Risk: RiskWrite, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}, {Name: "script", Kind: KindString, Required: true}}},

		// Network (egress; approval required).
		{ID: "http_get", Command: "curl", Risk: RiskNetwork, RequiresApproval: true, TimeoutSeconds: 30,
			Args: []ArgSpec{{Name: "url", Kind: KindString, Required: true}}},
		{ID: "download", Command: "wget", Risk: RiskNetwork, RequiresApproval: true, TimeoutSeconds: 30,
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
