package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"

	"agentx/internal/state"
)

// Recorder persists a session's events to its events/ directory, append-only and
// ordered by epoch then arrival. Source contract:
// docs/implementation/03_configuration_and_storage.md (Persistence Behavior).
type Recorder struct {
	eventsDir string
	seq       atomic.Uint64
}

// Recorder returns a Recorder for the given session id.
func (s *Store) Recorder(id string) *Recorder {
	return &Recorder{eventsDir: filepath.Join(s.Dir(id), "events")}
}

// Write persists one event as
// <zero-padded-epoch>_<zero-padded-seq>_<content_type>.json. The arrival
// sequence is a per-recorder monotonic counter so events sharing a millisecond
// epoch still load back in the order they were written. Writes are append-only:
// an existing file is never overwritten; a numeric suffix is added instead.
func (r *Recorder) Write(ev state.Event) error {
	if err := ev.Validate(); err != nil {
		return fmt.Errorf("refusing to persist invalid event: %w", err)
	}
	if err := os.MkdirAll(r.eventsDir, 0o755); err != nil {
		return fmt.Errorf("create events dir: %w", err)
	}
	seq := r.seq.Add(1) - 1
	base := fmt.Sprintf("%013d_%06d_%s", ev.Epoch, seq, ev.ContentType)
	path := filepath.Join(r.eventsDir, base+".json")
	for i := 1; fileExists(path); i++ {
		path = filepath.Join(r.eventsDir, fmt.Sprintf("%s_%d.json", base, i))
	}
	return writeJSON(path, ev)
}

// Run drains a bus subscription, persisting each event, until the subscription
// channel closes (call Subscription.Close to stop).
func (r *Recorder) Run(sub *state.Subscription) error {
	for ev := range sub.C {
		if err := r.Write(ev); err != nil {
			return err
		}
	}
	return nil
}

// Load returns all persisted events ordered by epoch (then by filename, which is
// epoch-prefixed and zero-padded for stable ordering).
func (r *Recorder) Load() ([]state.Event, error) {
	entries, err := os.ReadDir(r.eventsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read events dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	events := make([]state.Event, 0, len(names))
	for _, name := range names {
		var ev state.Event
		if err := readJSON(filepath.Join(r.eventsDir, name), &ev); err != nil {
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
		events = append(events, ev)
	}
	return events, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
