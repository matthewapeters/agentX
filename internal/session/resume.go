package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"agentx/internal/state"
)

// ResumeCandidate is one entry in a resume picker: enough to identify a
// session and show what it was last about. See
// docs/architecture/behavior/session_resume.feature.md §2.
type ResumeCandidate struct {
	ID           string
	Name         string
	LastPrompt   string
	LastPromptAt time.Time
}

// defaultResumableLimit bounds ListResumable's result size. This is a
// constant, not a flag — the CLI has no precedent for that level of
// configurability and doesn't need one here. A picker showing every session
// ever created would be unusable in practice (this machine alone has 200+
// session directories on disk).
const defaultResumableLimit = 20

// ListResumable enumerates sessions under the store's root with at least one
// recorded, non-ephemeral user prompt, most-recent-first, capped at limit
// (<= 0 uses defaultResumableLimit). A session with zero user prompts is
// excluded — there is no last-prompt line to show, and nothing meaningful to
// resume. A session whose session.json or event log can't be read is
// skipped, not fatal to the rest of the listing.
func (s *Store) ListResumable(limit int) ([]ResumeCandidate, error) {
	if limit <= 0 {
		limit = defaultResumableLimit
	}
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session root: %w", err)
	}

	var candidates []ResumeCandidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		identity, err := s.Load(id)
		if err != nil {
			continue
		}
		found, ev, err := s.lastUserPrompt(id)
		if err != nil || !found {
			continue
		}
		text, _ := textOfUserPrompt(ev)
		candidates = append(candidates, ResumeCandidate{
			ID:           id,
			Name:         identity.Name,
			LastPrompt:   text,
			LastPromptAt: time.UnixMilli(ev.Epoch),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastPromptAt.After(candidates[j].LastPromptAt)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

// HasUserPrompt reports whether session id has at least one recorded,
// non-ephemeral user_prompt event. This is the single definition of "this
// session has real content," shared by ListResumable (deciding what to show
// in the picker) and the mid-session resume trigger's abandoned-session
// cleanup (deciding whether an outgoing session's directory is safe to
// delete) — one predicate, two call sites, so "empty" can never mean two
// subtly different things in two places
// (docs/architecture/behavior/session_resume.feature.md §2 and §4).
func (s *Store) HasUserPrompt(id string) (bool, error) {
	found, _, err := s.lastUserPrompt(id)
	return found, err
}

// RemoveIfEmpty deletes session id's entire directory if it has no
// recorded, non-ephemeral user prompt (per HasUserPrompt) — the mid-session
// resume trigger's abandoned-session cleanup: a fresh session the user
// never typed into before switching to a different one is removed as part
// of the switch; a session with even one real prompt, including one
// abandoned mid-conversation, is never touched
// (docs/architecture/behavior/session_resume.feature.md §4). Reports
// whether it actually removed anything — false with a nil error for a
// non-empty session is the ordinary "left alone" case, not a failure.
func (s *Store) RemoveIfEmpty(id string) (removed bool, err error) {
	hasPrompt, err := s.HasUserPrompt(id)
	if err != nil {
		return false, err
	}
	if hasPrompt {
		return false, nil
	}
	if err := os.RemoveAll(s.Dir(id)); err != nil {
		return false, err
	}
	return true, nil
}

// lastUserPrompt finds the most recent non-ephemeral user_prompt event for
// session id without loading its full event log. Recorder filenames are
// epoch-then-seq prefixed and zero-padded
// (<epoch>_<seq>_<content_type>.json), so a directory listing already sorts
// chronologically (os.ReadDir sorts by name); reading entries in reverse and
// stopping at the first match is far cheaper than Recorder.Load's
// read-everything-then-filter for a listing that must scan many sessions —
// this machine alone has 200+ session directories on disk.
func (s *Store) lastUserPrompt(id string) (bool, state.Event, error) {
	dir := filepath.Join(s.Dir(id), "events")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, state.Event{}, nil
	}
	if err != nil {
		return false, state.Event{}, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].IsDir() {
			continue
		}
		var ev state.Event
		if readJSON(filepath.Join(dir, entries[i].Name()), &ev) != nil {
			continue // unreadable/malformed event file: skip, keep scanning
		}
		if ev.Ephemeral || ev.ContentType != state.ContentUserPrompt {
			continue
		}
		return true, ev, nil
	}
	return false, state.Event{}, nil
}

// textOfUserPrompt extracts a user_prompt event's text field from its
// generic (JSON-decoded) payload.
func textOfUserPrompt(ev state.Event) (string, bool) {
	p, ok := ev.Payload.(map[string]any)
	if !ok {
		return "", false
	}
	text, ok := p["text"].(string)
	return text, ok
}
