package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubArtifacts is a minimal in-memory Artifacts stub for tests that need an
// Executor but don't care about artifact persistence itself.
type stubArtifacts struct{}

func (stubArtifacts) Write(data []byte) (string, error)                  { return "artifacts/stub.txt", nil }
func (stubArtifacts) Read(ref string, offset, limit int) ([]byte, error) { return nil, nil }

// failingArtifacts always fails Write, so a test can exercise the branch
// where a successful edit's own artifact persistence fails.
type failingArtifacts struct{}

func (failingArtifacts) Write(data []byte) (string, error) {
	return "", errors.New("artifact store unavailable")
}
func (failingArtifacts) Read(ref string, offset, limit int) ([]byte, error) { return nil, nil }

func newTestExecutor() *Executor {
	return NewExecutor(stubArtifacts{}, 0)
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

var editFileDescriptor = Descriptor{ID: "edit_file", Builtin: "edit_file"}

// GIVEN a file containing exactly one occurrence of old_string
// WHEN edit_file runs
// THEN the file on disk is updated with new_string in its place, and the
// result reports success.
func TestEditFileReplacesUniqueMatch(t *testing.T) {
	path := writeTempFile(t, "hello world\ngoodbye world\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "hello world", "new_string": "hi there",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "hi there\ngoodbye world\n"
	if string(got) != want {
		t.Errorf("file content = %q, want %q", string(got), want)
	}
}

// GIVEN a file that does not contain old_string anywhere
// WHEN edit_file runs
// THEN it fails with an actionable "not found" message — not a bare exit
// code the way the sed-backed implementation did — and the file is left
// unmodified.
func TestEditFileOldStringNotFound(t *testing.T) {
	path := writeTempFile(t, "hello world\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "does not exist", "new_string": "x",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
	if !strings.Contains(res.Preview, "not found") {
		t.Errorf("Preview = %q, want it to explain old_string was not found", res.Preview)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "hello world\n" {
		t.Errorf("file was modified despite a failed edit: %q", string(got))
	}
}

// GIVEN a file containing old_string more than once, with replace_all unset
// WHEN edit_file runs
// THEN it fails with an actionable "not unique" message rather than
// guessing which occurrence to replace, and the file is left unmodified —
// the model gets something concrete to correct against (a more specific
// old_string), unlike sed's opaque exit code.
func TestEditFileAmbiguousMatchWithoutReplaceAll(t *testing.T) {
	path := writeTempFile(t, "foo\nfoo\nfoo\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "foo", "new_string": "bar",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
	if !strings.Contains(res.Preview, "3 times") {
		t.Errorf("Preview = %q, want it to report the match count", res.Preview)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "foo\nfoo\nfoo\n" {
		t.Errorf("file was modified despite an ambiguous, rejected edit: %q", string(got))
	}
}

// GIVEN old_string containing characters that are special in regex/sed
// (., *, (, ), [, ], $, \) but have no special meaning here
// WHEN edit_file runs
// THEN it matches literally, byte-for-byte — no escaping needed. This is
// the entire reason edit_file stopped being sed-backed: proving it here
// directly validates that claim rather than assuming strings.Count/Replace
// behave literally.
func TestEditFileMatchesRegexMetacharactersLiterally(t *testing.T) {
	path := writeTempFile(t, "price := arr[0].Cost * $rate\ndone\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "arr[0].Cost * $rate", "new_string": "arr[1].Cost * $rate",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "price := arr[1].Cost * $rate\ndone\n"
	if string(got) != want {
		t.Errorf("file content = %q, want %q (regex metacharacters must match literally)", string(got), want)
	}
}

// GIVEN an anchor line and new_string containing the anchor plus additional
// lines after it
// WHEN edit_file runs
// THEN a whole new multi-line block is inserted after the anchor — no
// separate "insert" tool needed, exactly the design claim made when this
// tool replaced edit_file's sed backing.
func TestEditFileInsertsMultiLineBlock(t *testing.T) {
	path := writeTempFile(t, "func A() {\n\treturn 1\n}\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path":       path,
		"old_string": "func A() {\n\treturn 1\n}\n",
		"new_string": "func A() {\n\treturn 1\n}\n\nfunc B() {\n\treturn 2\n}\n",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "func A() {\n\treturn 1\n}\n\nfunc B() {\n\treturn 2\n}\n"
	if string(got) != want {
		t.Errorf("file content = %q, want %q (new function inserted after the anchor)", string(got), want)
	}
}

// GIVEN old_string matches exactly once and replace_all is also set
// WHEN edit_file runs
// THEN it behaves identically to the non-replace_all single-match case
// (still reports 1 occurrence replaced) — replace_all being redundant here
// must not change the outcome or the reported count.
func TestEditFileReplaceAllWithSingleMatch(t *testing.T) {
	path := writeTempFile(t, "hello world\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "hello world", "new_string": "hi there", "replace_all": "true",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}
	if !strings.Contains(res.Preview, "replaced 1 occurrence") {
		t.Errorf("Preview = %q, want it to report exactly 1 occurrence", res.Preview)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hi there\n" {
		t.Errorf("file content = %q, want %q", string(got), "hi there\n")
	}
}

// GIVEN old_string that differs from the file's actual content only by
// trailing whitespace
// WHEN edit_file runs
// THEN it reports not-found rather than fuzzy-matching — the match is
// whitespace-literal, exact-or-nothing, never a "close enough" match a
// model could get away with guessing at.
func TestEditFileWhitespaceMismatchIsNotFound(t *testing.T) {
	path := writeTempFile(t, "hello world\n") // no trailing space on the line
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "hello world \n", "new_string": "hi\n", // trailing space before \n
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error — a trailing-whitespace mismatch must not fuzzy-match", res.Status)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "hello world\n" {
		t.Errorf("file was modified despite a whitespace mismatch that should have been rejected: %q", string(got))
	}
}

// GIVEN a file with a specific line to remove, matched (old_string) as that
// line's text plus its trailing newline, with new_string empty
// WHEN edit_file runs
// THEN the line is gone entirely — not replaced with a blank line — and
// every other line is untouched. Deletion is just replacement with "",
// but it's a distinct enough use case (removing a line, an import, a
// block) to deserve its own explicit check rather than being assumed to
// fall out of the replace tests for free.
func TestEditFileRemovesLine(t *testing.T) {
	path := writeTempFile(t, "package main\nimport \"fmt\"\nimport \"os\"\n\nfunc main() {}\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "import \"os\"\n", "new_string": "",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "package main\nimport \"fmt\"\n\nfunc main() {}\n"
	if string(got) != want {
		t.Errorf("file content = %q, want %q (line removed cleanly, no stray blank line)", string(got), want)
	}
}

// GIVEN a file containing old_string more than once, with replace_all set
// WHEN edit_file runs
// THEN every occurrence is replaced.
func TestEditFileReplaceAll(t *testing.T) {
	path := writeTempFile(t, "foo\nfoo\nfoo\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "foo", "new_string": "bar", "replace_all": "true",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "bar\nbar\nbar\n" {
		t.Errorf("file content = %q, want every occurrence replaced", string(got))
	}
}

// GIVEN a path that does not exist
// WHEN edit_file runs
// THEN it fails cleanly instead of panicking or fabricating a file.
func TestEditFileMissingPath(t *testing.T) {
	e := newTestExecutor()
	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": filepath.Join(t.TempDir(), "does-not-exist.txt"), "old_string": "x", "new_string": "y",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
}

// GIVEN an empty old_string
// WHEN edit_file runs
// THEN it is rejected outright rather than falling through to
// strings.Count's "matches everywhere" behavior, which would otherwise
// silently insert new_string at the very start of the file — never what an
// edit call means. The file must be left unmodified.
func TestEditFileEmptyOldStringRejected(t *testing.T) {
	path := writeTempFile(t, "hello world\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "", "new_string": "INSERTED",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
	if !strings.Contains(res.Preview, "empty") {
		t.Errorf("Preview = %q, want it to explain old_string must not be empty", res.Preview)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "hello world\n" {
		t.Errorf("file was modified despite a rejected empty old_string: %q", string(got))
	}
}

// GIVEN a file whose permissions block writing after the read already
// succeeded (e.g. a permission change mid-flight, a read-only mount)
// WHEN edit_file runs
// THEN the write failure is reported as a normal error result, not a panic
// or an unreported crash.
func TestEditFileWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission bits; this test needs a non-root process")
	}
	path := writeTempFile(t, "hello world\n")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) }) // let t.TempDir() clean up afterward
	e := newTestExecutor()

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "hello world", "new_string": "hi there",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (a read-only file's write should fail)", res.Status)
	}
}

// GIVEN a successful edit whose artifact persistence then fails
// WHEN edit_file runs
// THEN the edit itself is still reported as successful (the file was
// genuinely changed — that must not be hidden) but no artifact ref is set,
// rather than the executor panicking or silently swallowing the write it
// already made.
func TestEditFileArtifactWriteFailureStillReportsSuccess(t *testing.T) {
	path := writeTempFile(t, "hello world\n")
	e := NewExecutor(failingArtifacts{}, 0)

	res, err := e.Run(context.Background(), editFileDescriptor, map[string]string{
		"path": path, "old_string": "hello world", "new_string": "hi there",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok — the file edit succeeded even though artifact persistence failed", res.Status)
	}
	if res.Ref != "" {
		t.Errorf("Ref = %q, want empty since the artifact store failed", res.Ref)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hi there\n" {
		t.Errorf("file content = %q, want the edit to have applied despite the artifact failure", string(got))
	}
}
