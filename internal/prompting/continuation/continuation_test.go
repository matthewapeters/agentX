package continuation

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectRealSessionExamples reproduces the exact two responses from session
// amber-quartz that silently ended a turn instead of continuing.
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

// TestDetectOnlyLastSentence: a "let me" phrase earlier in the response — not the
// final sentence — must not trigger. Only the model's last word counts as a stated
// forward intent.
func TestDetectOnlyLastSentence(t *testing.T) {
	response := "Let me explain what I found. The bug is in the retry loop, which never terminates."
	if _, _, ok := Detect(response); ok {
		t.Error("an earlier 'let me' aside must not trigger when the final sentence doesn't restate it")
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
