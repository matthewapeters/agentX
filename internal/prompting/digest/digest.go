// Package digest builds the session digest — the compact catch-up note the
// classifier reads to place a new turn. v1 is a bounded projection of state
// AgentX already persists: the most recent enabled conversational turns from the
// event log (a rolling topic summary is a deferred v2 refinement; open-task refs
// arrive with the task pipeline).
//
// Build is a pure function over an event slice, so the digest is testable without
// disk and always rebuildable from the append-only log — which is why the "turtles"
// bootstrapping never recurses: the digest is consumed by the classifier, never
// built by it.
//
// Design: docs/architecture/prompt_fan_groups.md (relatedness triage / context),
// cascade_classifier.md. Behavior contract:
// tests/features/prompting/session_digest.feature.
package digest

import (
	"fmt"
	"strings"

	"agentx/internal/state"
)

// Turn is one conversational exchange in the digest.
type Turn struct {
	Role    string // "user" | "agent"
	Text    string
	Ordinal uint64
}

// Digest is the classifier's catch-up note over a session.
type Digest struct {
	RecentTurns []Turn // most recent, chronological
	TurnCount   int    // total enabled turns seen (before windowing)
	Cursor      uint64 // highest event ordinal reflected (staleness / rebuild point)
}

// Build projects the enabled conversational turns from an ordered event slice,
// keeping the most recent maxTurns (<= 0 keeps all). Disabled events are skipped,
// so the digest honors the user's context enable/disable choices; non-turn events
// (thinking, tool calls, processing state) are ignored.
func Build(events []state.Event, maxTurns int) Digest {
	var turns []Turn
	var cursor uint64
	for _, ev := range events {
		if ev.Ordinal > cursor {
			cursor = ev.Ordinal
		}
		if !ev.Enabled {
			continue
		}
		role, ok := turnRole(ev.ContentType)
		if !ok {
			continue
		}
		text := payloadText(ev.Payload)
		if strings.TrimSpace(text) == "" {
			continue
		}
		turns = append(turns, Turn{Role: role, Text: text, Ordinal: ev.Ordinal})
	}

	count := len(turns)
	if maxTurns > 0 && len(turns) > maxTurns {
		turns = turns[len(turns)-maxTurns:]
	}
	return Digest{RecentTurns: turns, TurnCount: count, Cursor: cursor}
}

// Render produces the compact {{session_digest}} text block. It is empty when
// there are no turns (cold start) — so relatedness triage sees an empty digest and
// returns "new", the graceful bottom of the bootstrapping regress.
func (d Digest) Render() string {
	if len(d.RecentTurns) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range d.RecentTurns {
		fmt.Fprintf(&b, "%s: %s\n", t.Role, strings.TrimSpace(t.Text))
	}
	return strings.TrimRight(b.String(), "\n")
}

func turnRole(ct state.ContentType) (string, bool) {
	switch ct {
	case state.ContentUserPrompt:
		return "user", true
	case state.ContentAgentResponse:
		return "agent", true
	default:
		return "", false
	}
}

// payloadText pulls the "text" field from an event payload, tolerating both the
// decoded-from-disk map form and a raw string.
func payloadText(p any) string {
	switch v := p.(type) {
	case map[string]any:
		if t, ok := v["text"].(string); ok {
			return t
		}
	case string:
		return v
	}
	return ""
}
