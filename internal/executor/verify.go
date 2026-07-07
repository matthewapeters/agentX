package executor

import (
	"os"
	"path/filepath"

	"agentx/internal/tools"
)

// FSVerifier verifies a tool call's effect. Two regimes by risk class:
//
//   - Read tools (Risk == read): the effect IS the returned output, so a clean exit
//     with output present verifies. Stat-ing the target is wrong here — list_dir's
//     "path" arg is a directory, and the old file-target check phantomed every
//     successful directory listing (the mellow-meadow plan died on its first leaf).
//   - Everything else (writes/commands): the run must have exited cleanly, and if the
//     call named a file target (a "path" arg), that file must now exist and be
//     non-empty. This is what catches a write that claims success but never landed
//     on disk.
//
// Root resolves relative paths (the working directory the runner executes in);
// an empty Root treats paths as-is.
type FSVerifier struct {
	Root string
}

// Verify reports whether the effect is real.
func (v FSVerifier) Verify(d tools.Descriptor, args map[string]string, res tools.Result) bool {
	if res.Status != "ok" || res.Exit != 0 {
		return false
	}
	if d.Risk == tools.RiskRead {
		// A read that returned nothing proved nothing; output is the effect. Bytes
		// counts the full result, Preview covers runners that only surface a snippet.
		return res.Bytes > 0 || res.Preview != ""
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
