package classify

import (
	"context"
	"strings"
	"testing"

	"agentx/internal/prompting"
)

// TestBuildMessagesEmptyHistoryUnchanged: an empty history string leaves the assembled
// messages exactly as they were before this fix — no regression on the common (cold
// start, or a caller that opts out) case.
func TestBuildMessagesEmptyHistoryUnchanged(t *testing.T) {
	c := New("", 0, nil)
	got := c.buildMessages("do the thing", "")
	want := c.assembler.Assemble("do the thing")
	if len(got) != len(want) {
		t.Fatalf("buildMessages with empty history returned %d messages, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("message %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestBuildMessagesFoldsHistoryBeforeUserMessage reproduces the witty-falcon fix
// directly: a non-empty history digest is folded in as its own system message,
// immediately before the trailing user message, labeled as context rather than
// instructions.
func TestBuildMessagesFoldsHistoryBeforeUserMessage(t *testing.T) {
	c := New("", 0, nil) // "" substitutes DefaultPrompt (New's own contract), so the base
	// Assemble already carries one leading system message; the history message is a SECOND
	// one, inserted right before the trailing user message.
	history := "user: yes, read any suspect files\nagent: let me run cat AGENTS.md"
	msgs := c.buildMessages("proceed with the commands", history)

	if len(msgs) != 3 {
		t.Fatalf("buildMessages returned %d messages, want 3 (default-prompt system + history system + user)", len(msgs))
	}
	if msgs[1].Role != "system" {
		t.Errorf("msgs[1].Role = %q, want %q", msgs[1].Role, "system")
	}
	if !strings.Contains(msgs[1].Content, history) {
		t.Errorf("msgs[1].Content = %q, want it to contain the history digest", msgs[1].Content)
	}
	if !strings.Contains(strings.ToLower(msgs[1].Content), "context only") {
		t.Errorf("msgs[1].Content = %q, want it labeled as context, not instructions", msgs[1].Content)
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" || last.Content != "proceed with the commands" {
		t.Errorf("last message = %+v, want the unchanged user message trailing the history", last)
	}
}

// TestBuildMessagesFoldsHistoryWithSystemPrompt: with a configured system prompt, the
// history message still lands immediately before the user message — insertion point is
// stable regardless of whether a leading system message is present.
func TestBuildMessagesFoldsHistoryWithSystemPrompt(t *testing.T) {
	c := New("You are a classifier.", 0, nil)
	msgs := c.buildMessages("proceed", "agent: let me run cat AGENTS.md")

	if len(msgs) != 3 {
		t.Fatalf("buildMessages returned %d messages, want 3 (configured system + history system + user)", len(msgs))
	}
	if msgs[0].Content != "You are a classifier." {
		t.Errorf("msgs[0] = %+v, want the configured system prompt first", msgs[0])
	}
	if !strings.Contains(msgs[1].Content, "let me run cat AGENTS.md") {
		t.Errorf("msgs[1] = %+v, want the history digest second", msgs[1])
	}
	if msgs[2].Role != "user" || msgs[2].Content != "proceed" {
		t.Errorf("msgs[2] = %+v, want the user message last", msgs[2])
	}
}

// TestClassifyPassesHistoryToChat proves the wiring end to end: Classify's chat call
// actually receives the history-bearing messages buildMessages produces.
func TestClassifyPassesHistoryToChat(t *testing.T) {
	var gotMessages []prompting.Message
	chat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		gotMessages = msgs
		return `{"route": "invoke_planner", "confidence": 0.9, "rationale": "continuation"}`, nil
	}
	c := New("", 0, chat)
	v := c.Classify(context.Background(), "proceed with the commands", "agent: let me run cat AGENTS.md")

	if v.Route != InvokePlanner {
		t.Errorf("Route = %q, want %q", v.Route, InvokePlanner)
	}
	found := false
	for _, m := range gotMessages {
		if strings.Contains(m.Content, "let me run cat AGENTS.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("chat received %+v, want the history digest present in one of the messages", gotMessages)
	}
}
