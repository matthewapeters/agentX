package tools

import (
	"path/filepath"
	"strings"
)

// PathScope classifies a resolved file path relative to the project root for
// approval-scoping purposes.
type PathScope struct {
	Resolved      string // cleaned; joined against projectRoot first if path was relative
	InsideProject bool
	Ext           string // filepath.Ext, lowercased; "" if none — always derived from Resolved,
	// never taken from caller-supplied input, so it can't be spoofed to "*"
	// or otherwise widened by a crafted argument.
}

// ClassifyPath resolves path against projectRoot and classifies it, reusing
// the same Clean+Rel containment check internal/executor.Executor.escapesRoot
// already used (not a naive string-prefix match, so a "../../etc/passwd"
// relative path can't be misclassified as inside). An empty projectRoot (the
// session's cwd working-memory fact absent, disabled, or deleted) always
// classifies as outside the project — the conservative default when the
// boundary can't be determined. This is a different posture from
// escapesRoot's own "no root configured -> no boundary at all" convention:
// that backs a read-grant check, this backs an approval-scoping decision, so
// it fails closed instead.
func ClassifyPath(path, projectRoot string) PathScope {
	full := path
	if projectRoot != "" && !filepath.IsAbs(path) {
		full = filepath.Join(projectRoot, path)
	}
	full = filepath.Clean(full)
	ext := strings.ToLower(filepath.Ext(full))

	if projectRoot == "" {
		return PathScope{Resolved: full, InsideProject: false, Ext: ext}
	}
	rel, err := filepath.Rel(filepath.Clean(projectRoot), full)
	inside := err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	return PathScope{Resolved: full, InsideProject: inside, Ext: ext}
}

// ScopeArgs returns the set of canonical "scope" argument maps this call
// needs approved. Each map is turned into an actual policy key via the
// existing approvalKey(d.ID, map) — same function every other tool already
// uses, unchanged — so this is purely about WHAT gets keyed and persisted,
// not a new key format:
//
//   - write_file/edit_file (a single "path" arg): one map, {"ext": ...} when
//     the resolved path is inside projectRoot, or {"ext": ..., "path":
//     resolved} when it's outside. Approving one call then covers every
//     future call at the same scope (verb + extension inside the project;
//     verb + exact path + extension outside it) instead of today's
//     per-call-content key — the fix for "approve for all sessions" almost
//     never producing a cache hit on a content-bearing tool.
//   - apply_patch (no structured path argument — its target files live only
//     in the diff's own headers): one map per file the diff touches,
//     deduplicated. An unparseable diff falls back to returning args
//     unchanged (today's exact per-call behavior) rather than silently
//     granting a coarser scope than could actually be proven.
//   - every other tool: args unchanged — today's behavior, untouched.
func (d Descriptor) ScopeArgs(args map[string]string, projectRoot string) []map[string]string {
	switch d.ID {
	case "write_file", "edit_file":
		return []map[string]string{pathScopeArgs(args["path"], projectRoot)}
	case "apply_patch":
		paths, ok := parsePatchPaths(args["patch"])
		if !ok {
			return []map[string]string{args}
		}
		seen := make(map[string]bool, len(paths))
		out := make([]map[string]string, 0, len(paths))
		for _, p := range paths {
			sa := pathScopeArgs(p, projectRoot)
			dedupeKey := sa["ext"] + "\x00" + sa["path"]
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			out = append(out, sa)
		}
		return out
	default:
		return []map[string]string{args}
	}
}

// pathScopeArgs builds one ScopeArgs entry for a single path.
func pathScopeArgs(path, projectRoot string) map[string]string {
	scope := ClassifyPath(path, projectRoot)
	if scope.InsideProject {
		return map[string]string{"ext": scope.Ext}
	}
	return map[string]string{"ext": scope.Ext, "path": scope.Resolved}
}
