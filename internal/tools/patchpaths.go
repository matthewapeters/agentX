package tools

import "strings"

// parsePatchPaths extracts every target file path from a unified diff's
// "--- "/"+++ " header lines (and, redundantly but harmlessly, any
// "diff --git a/X b/Y" lines) — nothing else is parsed, no hunk content, no
// line-level changes. Paths are taken exactly as written, with no
// conventional git a/-b/-prefix stripping: apply_patch runs `patch -p0`
// (internal/tools/descriptors.go), which strips zero leading path
// components, so the literal header text is exactly what the running patch
// command will actually target — stripping an a/-b/ prefix here would
// silently disagree with what the tool itself does. "/dev/null" (the
// create/delete sentinel some diff tools emit for the non-existent side) is
// never a real target and is skipped. ok is false when no path can be
// extracted at all — an unparseable or empty patch — so the caller can fall
// back to a safe default instead of silently granting a coarser scope than
// could actually be proven.
func parsePatchPaths(patch string) (paths []string, ok bool) {
	seen := make(map[string]bool)
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || p == "/dev/null" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for line := range strings.SplitSeq(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "):
			add(headerPath(line[len("--- "):]))
		case strings.HasPrefix(line, "+++ "):
			add(headerPath(line[len("+++ "):]))
		case strings.HasPrefix(line, "diff --git "):
			fields := strings.Fields(line[len("diff --git "):])
			if len(fields) == 2 {
				add(fields[0])
				add(fields[1])
			}
		}
	}
	return out, len(out) > 0
}

// headerPath strips a unified-diff header's optional trailing
// tab-and-timestamp ("path\t2024-01-01 12:00:00 +0000" -> "path").
func headerPath(rest string) string {
	if i := strings.IndexByte(rest, '\t'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}
