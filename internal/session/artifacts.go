package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// Artifacts persists large tool outputs as files under a session's artifacts/
// directory and reads them back by ref with line-based windowing. It backs the
// read_output tool and the tool_result preview+ref model
// (docs/implementation/05_security_approvals_and_command_policy.md).
type Artifacts struct {
	dir string
	seq atomic.Uint64
}

// Artifacts returns the artifact store for a session id, creating its directory.
func (s *Store) Artifacts(id string) (*Artifacts, error) {
	dir := filepath.Join(s.Dir(id), "artifacts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifacts dir: %w", err)
	}
	return &Artifacts{dir: dir}, nil
}

// Write stores data as a new artifact and returns a session-relative ref
// ("artifacts/<seq>.txt") usable by Read.
func (a *Artifacts) Write(data []byte) (string, error) {
	seq := a.seq.Add(1)
	name := fmt.Sprintf("%012d.txt", seq)
	if err := os.WriteFile(filepath.Join(a.dir, name), data, 0o644); err != nil {
		return "", fmt.Errorf("write artifact: %w", err)
	}
	return "artifacts/" + name, nil
}

// Read returns a line window of the artifact named by ref: offset is a 0-based
// line index and limit caps the number of lines (<=0 means to the end). When both
// offset and limit are <=0 the full artifact is returned. The ref is confined to
// the artifacts directory (its base name only), so it cannot escape via traversal.
func (a *Artifacts) Read(ref string, offset, limit int) ([]byte, error) {
	name := filepath.Base(filepath.Clean(ref))
	data, err := os.ReadFile(filepath.Join(a.dir, name))
	if err != nil {
		return nil, fmt.Errorf("read artifact %q: %w", ref, err)
	}
	if offset <= 0 && limit <= 0 {
		return data, nil
	}
	lines := strings.Split(string(data), "\n")
	if offset < 0 {
		offset = 0
	}
	if offset >= len(lines) {
		return []byte{}, nil
	}
	end := len(lines)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return []byte(strings.Join(lines[offset:end], "\n")), nil
}
