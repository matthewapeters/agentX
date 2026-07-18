package classify

import (
	"context"
	"testing"

	"agentx/internal/prompting"
)

// TestClassifyGroundsInFacts: a Classifier with Facts set folds a working-memory
// message between the system prompt and the user message — the fix for the
// ungrounded classification path (quiet-frustrating-maple: "what is the agentX
// project written in?" resolved to respond_directly with no project/cwd fact in
// sight, so the model guessed instead of routing to investigation).
func TestClassifyGroundsInFacts(t *testing.T) {
	var sawMsgs []prompting.Message
	chat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		sawMsgs = msgs
		return `{"route":"invoke_planner","confidence":0.9,"rationale":"project question"}`, nil
	}
	c := New("SYSTEM PROMPT", 0, chat)
	c.Facts = func() []prompting.Fact {
		return []prompting.Fact{{Key: "project", Value: "agentX"}, {Key: "cwd", Value: "/Projects/agentX"}}
	}

	v := c.Classify(context.Background(), "what is the agentX project written in?")
	if v.Route != InvokePlanner {
		t.Fatalf("Classify route = %q, want invoke_planner", v.Route)
	}
	if len(sawMsgs) != 3 {
		t.Fatalf("messages = %+v, want 3 (system prompt, facts system, user)", sawMsgs)
	}
	if sawMsgs[0].Content != "SYSTEM PROMPT" {
		t.Errorf("msg[0] = %+v, want the classification system prompt first", sawMsgs[0])
	}
	if sawMsgs[1].Role != "system" || sawMsgs[1].Content == "" {
		t.Errorf("msg[1] = %+v, want a facts system message", sawMsgs[1])
	}
	if sawMsgs[2].Role != "user" || sawMsgs[2].Content != "what is the agentX project written in?" {
		t.Errorf("msg[2] = %+v, want the user message last, unchanged", sawMsgs[2])
	}
}

// TestClassifyUngroundedWithoutFacts: Facts unset (nil) behaves exactly as
// before — no grounding message, no ripple to callers that don't set it.
func TestClassifyUngroundedWithoutFacts(t *testing.T) {
	var sawMsgs []prompting.Message
	chat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		sawMsgs = msgs
		return `{"route":"respond_directly","confidence":0.5,"rationale":"n/a"}`, nil
	}
	c := New("SYSTEM PROMPT", 0, chat)

	c.Classify(context.Background(), "hello")
	if len(sawMsgs) != 2 {
		t.Fatalf("messages = %+v, want 2 (system prompt, user) with Facts unset", sawMsgs)
	}
}

// TestInsertFactsNoLeadingSystem covers insertFacts's "no leading system message"
// path directly (New always produces one, so Classify alone can't exercise it).
func TestInsertFactsNoLeadingSystem(t *testing.T) {
	msgs := []prompting.Message{{Role: "user", Content: "hi"}}
	out := insertFacts(msgs, func() []prompting.Fact {
		return []prompting.Fact{{Key: "cwd", Value: "/tmp"}}
	})
	if len(out) != 2 || out[0].Role != "system" || out[1].Role != "user" {
		t.Fatalf("insertFacts = %+v, want [system, user]", out)
	}
}
