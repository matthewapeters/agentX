package state

// ApprovalOption is one selectable choice in an approval request: Label is the
// text shown in the navigable list, Decision is the canonical string echoed
// back through Orchestrator.Resolve (e.g. "session", "allow_once", "deny").
// This is the generic wire shape every interactive decision uses, regardless
// of what's being decided — the surface renders whatever it's handed and
// never hardcodes a per-kind option vocabulary.
type ApprovalOption struct {
	Label    string `json:"label"`
	Decision string `json:"decision"`
}

// DecodeApprovalOptions normalizes the "options" value of an approval_request
// event's payload into []ApprovalOption regardless of source shape: a literal
// []ApprovalOption (in-process, freshly published) or a JSON-round-tripped
// []any of map[string]any (replayed from the session recorder on resume, or
// delivered to a remote SSE-attached surface). Malformed entries are skipped
// rather than erroring, matching the defensive-decode style used elsewhere for
// Payload any map fields.
func DecodeApprovalOptions(v any) []ApprovalOption {
	switch opts := v.(type) {
	case []ApprovalOption:
		return opts
	case []any:
		out := make([]ApprovalOption, 0, len(opts))
		for _, item := range opts {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			label, _ := m["label"].(string)
			decision, _ := m["decision"].(string)
			if label == "" && decision == "" {
				continue
			}
			out = append(out, ApprovalOption{Label: label, Decision: decision})
		}
		return out
	default:
		return nil
	}
}
