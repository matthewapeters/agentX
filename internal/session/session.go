package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Identity is the canonical identity and creation metadata for a session,
// persisted as session.json.
type Identity struct {
	ID           string `json:"session_id"`
	Name         string `json:"session_name"`
	CreatedEpoch int64  `json:"created_epoch"`
}

// Store roots all session persistence under a single directory
// (conventionally ~/.config/agentx/sessions).
type Store struct {
	root string
}

// NewStore returns a Store rooted at the given directory.
func NewStore(root string) *Store { return &Store{root: root} }

// Root returns the session root directory.
func (s *Store) Root() string { return s.root }

// Dir returns the directory for a given session id.
func (s *Store) Dir(id string) string { return filepath.Join(s.root, id) }

type options struct {
	now   func() time.Time
	namer func() string
}

// Option customizes session creation (primarily for deterministic tests).
type Option func(*options)

// WithClock overrides the creation clock.
func WithClock(f func() time.Time) Option { return func(o *options) { o.now = f } }

// WithNamer overrides the human-readable name generator.
func WithNamer(f func() string) Option { return func(o *options) { o.namer = f } }

// Create allocates a new session: a dir-unique canonical id, a root-unique
// human-readable name, the session directory with an events/ folder, and the
// session.json metadata file.
func (s *Store) Create(opts ...Option) (Identity, error) {
	o := options{now: time.Now, namer: defaultNamer}
	for _, fn := range opts {
		fn(&o)
	}

	epoch := o.now().UnixMilli()
	id, dir, err := s.allocateDir(epoch)
	if err != nil {
		return Identity{}, err
	}
	name, err := s.uniqueName(o.namer)
	if err != nil {
		return Identity{}, err
	}

	identity := Identity{ID: id, Name: name, CreatedEpoch: epoch}
	if err := os.MkdirAll(filepath.Join(dir, "events"), 0o755); err != nil {
		return Identity{}, fmt.Errorf("create events dir: %w", err)
	}
	if err := writeJSON(filepath.Join(dir, "session.json"), identity); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

// allocateDir creates a uniquely-named session directory derived from epoch,
// suffixing on collision, and returns the id and directory path.
func (s *Store) allocateDir(epoch int64) (string, string, error) {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return "", "", fmt.Errorf("create session root: %w", err)
	}
	base := fmt.Sprintf("%d", epoch)
	id := base
	for i := 1; ; i++ {
		dir := filepath.Join(s.root, id)
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return id, dir, nil
		}
		if !os.IsExist(err) {
			return "", "", fmt.Errorf("create session dir: %w", err)
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

// uniqueName returns a name from namer that is unique among existing sessions,
// suffixing "-2", "-3", ... on collision.
func (s *Store) uniqueName(namer func() string) (string, error) {
	used, err := s.usedNames()
	if err != nil {
		return "", err
	}
	base := namer()
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name, nil
}

// usedNames collects session_name values already recorded under the root.
func (s *Store) usedNames() (map[string]bool, error) {
	used := map[string]bool{}
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return used, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session root: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var id Identity
		if readJSON(filepath.Join(s.root, e.Name(), "session.json"), &id) == nil && id.Name != "" {
			used[id.Name] = true
		}
	}
	return used, nil
}

// writeJSON writes v to path via a temp-file-then-rename (same pattern as
// Plans.Write) rather than a direct os.WriteFile: on the same filesystem,
// os.Rename is atomic, so a concurrent reader (Recorder.Load listing the
// events directory while Recorder.Write persists the next event, or
// LoadWorkingMemory racing SaveWorkingMemory) always sees either the old file
// or the fully-written new one, never a truncated/partial one. A direct
// os.WriteFile makes the file visible (0 bytes, then partially written) the
// moment it's created, which readJSON's json.Unmarshal reports as "unexpected
// end of JSON input" if it wins that race — observed intermittently in the
// wm_pin integration scenarios before this fix.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
