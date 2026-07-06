package executor

import (
	"os"
	"path/filepath"

	"agentx/internal/tools"
)

// FSVerifier verifies a tool call's effect against the filesystem: the run must
// have exited cleanly, and if the call named a file target (a "path" arg), that
// file must now exist and be non-empty. This is what catches a write that claims
// success but never landed on disk.
//
// Root resolves relative paths (the working directory the runner executes in);
// an empty Root treats paths as-is.
type FSVerifier struct {
	Root string
}

// Verify reports whether the effect is real.
func (v FSVerifier) Verify(_ tools.Descriptor, args map[string]string, res tools.Result) bool {
	if res.Status != "ok" || res.Exit != 0 {
		return false
	}
	p := args["path"]
	if p == "" {
		return true // no file target to check; a clean exit is the effect
	}
	full := p
	if v.Root != "" && !filepath.IsAbs(p) {
		full = filepath.Join(v.Root, p)
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	return true
}
