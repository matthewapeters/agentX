package continuation

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectRealSessionExamples reproduces exact responses from sessions that
// silently ended a turn instead of continuing: amber-quartz (intent restated as the
// final sentence) and eager-otter (intent stated as the FIRST sentence, with
// everything after it a narrated fenced-code pseudo-tool-call rather than real
// content — the naive last-sentence split lands inside that fence and finds nothing).
func TestDetectRealSessionExamples(t *testing.T) {
	cases := []struct {
		response string
		wantVerb string
	}{
		{
			"The investigation results are mostly redundant (same directory listing repeated). Let me examine the actual source code to find a critical issue.",
			"examine",
		},
		{
			"Let me dig into the actual source code to find concrete issues rather than relying on directory listings alone.",
			"dig",
		},
		{
			"Let me read the key documentation files to understand the project.\n\n```bash\ncat /home/mpeters/Projects/agentX/README.md\n```</think>\n\nReading README.md first — it's usually the primary entry point:\n\n```bash\nhead -200 /home/mpeters/Projects/agentX/README.md\n```",
			"read",
		},
	}
	for _, c := range cases {
		verb, sentence, ok := Detect(c.response)
		if !ok {
			t.Fatalf("Detect(%q) = not found, want verb %q", c.response, c.wantVerb)
		}
		if verb != c.wantVerb {
			t.Errorf("Detect(%q) verb = %q, want %q", c.response, verb, c.wantVerb)
		}
		if sentence == "" {
			t.Error("sentence should not be empty when ok")
		}
	}
}

func TestDetectShouldIAndShallI(t *testing.T) {
	if verb, _, ok := Detect("Should I check the config file for this?"); !ok || verb != "check" {
		t.Errorf("Detect(should I) = verb=%q ok=%v, want check/true", verb, ok)
	}
	if verb, _, ok := Detect("Shall I investigate the executor module?"); !ok || verb != "investigate" {
		t.Errorf("Detect(shall I) = verb=%q ok=%v, want investigate/true", verb, ok)
	}
}

// TestDetectFirstSentenceAlsoTriggers: eager-otter showed a stated intent can be the
// model's OPENING sentence, with nothing that follows ever restating or fulfilling
// it (see TestDetectRealSessionExamples). So a "let me" in the first sentence must
// trigger even when the last sentence doesn't restate it.
func TestDetectFirstSentenceAlsoTriggers(t *testing.T) {
	response := "Let me explain what I found. The bug is in the retry loop, which never terminates."
	verb, _, ok := Detect(response)
	if !ok || verb != "explain" {
		t.Errorf("Detect(%q) = verb=%q ok=%v, want explain/true", response, verb, ok)
	}
}

// TestDetectMiddleSentenceStillDoesNotTrigger: a "let me" phrase that is neither the
// first nor the last sentence — a true rhetorical aside buried in the middle — must
// still not trigger.
func TestDetectMiddleSentenceStillDoesNotTrigger(t *testing.T) {
	response := "Here's my plan. Let me explain what I found first. The bug is in the retry loop, which never terminates."
	if _, _, ok := Detect(response); ok {
		t.Error("a 'let me' aside in the middle sentence must not trigger when neither the first nor last sentence restates it")
	}
}

func TestDetectNoTrigger(t *testing.T) {
	if _, _, ok := Detect("Here is the answer to your question."); ok {
		t.Error("a plain answer must not trigger")
	}
}

func TestDetectEmptyResponse(t *testing.T) {
	if _, _, ok := Detect(""); ok {
		t.Error("an empty response must not trigger")
	}
}

func TestLoadVerbsParsesAndSkipsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verbs.md")
	content := "# a comment\n\nexamine\nDIG\n  check  \n# another comment\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	verbs, err := LoadVerbs(path)
	if err != nil {
		t.Fatalf("LoadVerbs: %v", err)
	}
	for _, want := range []string{"examine", "dig", "check"} {
		if !verbs[want] {
			t.Errorf("verbs missing %q: %v", want, verbs)
		}
	}
	if len(verbs) != 3 {
		t.Errorf("verbs = %v, want exactly 3 entries", verbs)
	}
}

func TestLoadVerbsMissingFileIsEmptyNotError(t *testing.T) {
	verbs, err := LoadVerbs(filepath.Join(t.TempDir(), "does-not-exist.md"))
	if err != nil {
		t.Fatalf("LoadVerbs on a missing file returned an error: %v", err)
	}
	if len(verbs) != 0 {
		t.Errorf("verbs = %v, want empty", verbs)
	}
}

func TestAppendVerbThenLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "verbs.md")
	if err := os.WriteFile(path, []byte("examine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendVerb(path, "Revise"); err != nil {
		t.Fatalf("AppendVerb: %v", err)
	}
	verbs, err := LoadVerbs(path)
	if err != nil {
		t.Fatalf("LoadVerbs: %v", err)
	}
	if !verbs["examine"] || !verbs["revise"] {
		t.Errorf("verbs = %v, want both examine and revise (lowercased)", verbs)
	}
}

func TestAppendVerbCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new-verbs.md")
	if err := AppendVerb(path, "explore"); err != nil {
		t.Fatalf("AppendVerb on a missing file: %v", err)
	}
	verbs, err := LoadVerbs(path)
	if err != nil {
		t.Fatalf("LoadVerbs: %v", err)
	}
	if !verbs["explore"] {
		t.Errorf("verbs = %v, want explore", verbs)
	}
}
