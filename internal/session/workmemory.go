package session

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// Owner identifies who controls a working-memory fact. Bootstrap facts are
// seeded by the agent but immediately user-owned, so the agent never overwrites
// them on a later run; the user may add, enable, or disable facts at any time
// (docs/ux/03_PANEL_DETAILS.md §PD-03, docs/ux/02_USER_FLOWS.md UF-09).
type Owner string

const (
	// OwnerUser marks a fact the user controls; the agent must not overwrite it.
	OwnerUser Owner = "user"
	// OwnerAgent marks a fact the agent maintains and may overwrite.
	OwnerAgent Owner = "agent"
)

// Fact is one working-memory entry. Disabled facts are retained on disk but
// excluded from the assembled context.
type Fact struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Owner   Owner  `json:"owner"`
	Enabled bool   `json:"enabled"`
}

// WorkingMemory is the ordered set of facts persisted as working_memory.json in
// the session directory. It is the source of truth for context assembly and is
// rewritten in place on every change (unlike the append-only events log).
type WorkingMemory struct {
	Facts []Fact `json:"facts"`
}

// Get returns the fact with the given key, if present.
func (wm *WorkingMemory) Get(key string) (Fact, bool) {
	for _, f := range wm.Facts {
		if f.Key == key {
			return f, true
		}
	}
	return Fact{}, false
}

// SeedIfAbsent appends each fact whose key is not already present, leaving
// existing facts (including user edits and disabled state) untouched. It reports
// whether anything was added, so the caller can avoid an unnecessary write.
func (wm *WorkingMemory) SeedIfAbsent(facts ...Fact) bool {
	added := false
	for _, f := range facts {
		if _, ok := wm.Get(f.Key); ok {
			continue
		}
		wm.Facts = append(wm.Facts, f)
		added = true
	}
	return added
}

// Enabled returns the enabled facts in their stored order.
func (wm *WorkingMemory) Enabled() []Fact {
	out := make([]Fact, 0, len(wm.Facts))
	for _, f := range wm.Facts {
		if f.Enabled {
			out = append(out, f)
		}
	}
	return out
}

// BootstrapFacts returns the stable environment facts the agent seeds on startup:
// the user id, working directory, project name, home directory, OS/arch, and the
// git repository root when the working directory is inside one. All are user-owned
// and enabled — the agent seeds them once but never overwrites them thereafter. A
// value that cannot be determined is returned empty (and repo_root is omitted when
// there is no repository); SeedIfAbsent still records present keys so the user can
// fill them in.
func BootstrapFacts() []Fact {
	var uid string
	if u, err := user.Current(); err == nil {
		uid = u.Username
	}
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	repoRoot := gitToplevel(cwd)

	project := ""
	switch {
	case repoRoot != "":
		project = filepath.Base(repoRoot)
	case cwd != "":
		project = filepath.Base(cwd)
	}

	facts := []Fact{
		userFact("userid", uid),
		userFact("cwd", cwd),
		userFact("project", project),
		userFact("home", home),
		userFact("os", runtime.GOOS),
		userFact("arch", runtime.GOARCH),
	}
	if repoRoot != "" {
		facts = append(facts, userFact("repo_root", repoRoot))
	}
	return facts
}

// userFact builds an enabled, user-owned fact.
func userFact(key, value string) Fact {
	return Fact{Key: key, Value: value, Owner: OwnerUser, Enabled: true}
}

// gitToplevel returns the git repository root containing dir, or "" when dir is
// not inside a work tree or git is unavailable.
func gitToplevel(dir string) string {
	if dir == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// workingMemoryFile is the per-session WM persistence path.
func (s *Store) workingMemoryFile(id string) string {
	return filepath.Join(s.Dir(id), "working_memory.json")
}

// LoadWorkingMemory reads a session's working_memory.json. A missing file yields
// an empty WorkingMemory and no error, so callers can load-then-seed uniformly.
func (s *Store) LoadWorkingMemory(id string) (*WorkingMemory, error) {
	wm := &WorkingMemory{}
	err := readJSON(s.workingMemoryFile(id), wm)
	if os.IsNotExist(err) {
		return wm, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read working memory: %w", err)
	}
	return wm, nil
}

// SaveWorkingMemory writes a session's working_memory.json, creating the session
// directory if needed.
func (s *Store) SaveWorkingMemory(id string, wm *WorkingMemory) error {
	if err := os.MkdirAll(s.Dir(id), 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	return writeJSON(s.workingMemoryFile(id), wm)
}
