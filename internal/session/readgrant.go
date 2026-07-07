package session

import (
	"path/filepath"
	"strings"
)

// GrantScope is an approver's answer to an out-of-working-directory read request.
// It is the infra-level approval decision — surfaced through the executor's Approver
// seam, never as a conversation turn.
type GrantScope int

const (
	// GrantDeny — refuse this read.
	GrantDeny GrantScope = iota
	// GrantOnce — allow this one call; do not remember it.
	GrantOnce
	// GrantSession — allow, and remember the path in working memory for the rest of
	// the session so later reads under it need no further prompt.
	GrantSession
)

// readPathPrefix namespaces a granted read path as a working-memory fact key, so each
// grant is a first-class, user-visible, user-revocable entry (disabling or deleting the
// fact revokes the grant). Session-scoped by construction: working memory is per-session.
const readPathPrefix = "read_path:"

// GrantReadPath records an absolute path as permitted for reading, as an enabled,
// user-owned working-memory fact. It reports whether the grant was newly added. The
// path is cleaned but not otherwise validated here.
func GrantReadPath(wm *WorkingMemory, path string) bool {
	return wm.Set(readPathPrefix+filepath.Clean(path), "granted")
}

// PermittedReadPaths returns the absolute paths currently granted for reading — the
// enabled read_path: facts, with the namespace stripped. A disabled fact is a revoked
// grant and is excluded.
func PermittedReadPaths(wm *WorkingMemory) []string {
	var out []string
	for _, f := range wm.Enabled() {
		if p, ok := strings.CutPrefix(f.Key, readPathPrefix); ok {
			out = append(out, p)
		}
	}
	return out
}

// ReadAllowed reports whether reading path is permitted without a fresh prompt: it is
// within the confinement root, or within any already-granted path. An empty root means
// no confinement boundary (everything allowed). Relative paths are resolved against root.
func ReadAllowed(root string, granted []string, path string) bool {
	if root == "" {
		return true
	}
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(root, path)
	}
	full = filepath.Clean(full)
	if withinDir(root, full) {
		return true
	}
	for _, g := range granted {
		if withinDir(g, full) {
			return true
		}
	}
	return false
}

// ApplyGrant is the infra action behind an out-of-cwd read approval prompt: it records
// the decision and reports whether the call may proceed. GrantSession persists the path
// to working memory (so later reads under it skip the prompt) and allows; GrantOnce
// allows without persisting; GrantDeny refuses. It never runs anything itself.
func ApplyGrant(wm *WorkingMemory, path string, scope GrantScope) bool {
	switch scope {
	case GrantSession:
		GrantReadPath(wm, path)
		return true
	case GrantOnce:
		return true
	default:
		return false
	}
}

// WMReadGrants adapts working-memory read_path: grants into the set the executor
// consults for standing out-of-root read access (satisfies executor.ReadGrants). A
// path is allowed when it is within any enabled granted path.
type WMReadGrants struct{ WM *WorkingMemory }

// Allows reports whether path falls under a standing read grant.
func (g WMReadGrants) Allows(path string) bool {
	full := filepath.Clean(path)
	for _, p := range PermittedReadPaths(g.WM) {
		if withinDir(p, full) {
			return true
		}
	}
	return false
}

// withinDir reports whether full is dir itself or a descendant of it.
func withinDir(dir, full string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), full)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
