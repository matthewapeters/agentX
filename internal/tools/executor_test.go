package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
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
var grepFilesDescriptor = Descriptor{ID: "grep_files", Builtin: "grep_files"}
var deleteFileDescriptor = Descriptor{ID: "delete_file", Builtin: "delete_file"}
var moveFileDescriptor = Descriptor{ID: "move_file", Builtin: "move_file"}

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

// GIVEN a target path whose parent directory already exists
// WHEN write_file runs
// THEN it writes the file normally — the new MkdirAll call must not change
// this, the common case.
func TestWriteFileExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), writeFileDescriptor(), map[string]string{
		"path": path, "content": "hello\n",
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
	if string(got) != "hello\n" {
		t.Errorf("file content = %q, want %q", string(got), "hello\n")
	}
}

// GIVEN a target path whose immediate parent directory does not exist yet
// WHEN write_file runs
// THEN it creates that directory and writes the file — a brand-new
// subdirectory (e.g. a new Go package's first file) is an ordinary
// write_file target, not an error case; a bare os.WriteFile fails outright
// here with no way for the caller to recover, since no other builtin
// creates directories either (the bug a live tidal_two.md test run
// surfaced: write_file failed on internal/runtime/tidal/render.go because
// internal/runtime/tidal/ didn't exist yet).
func TestWriteFileCreatesMissingParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newpkg", "render.go")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), writeFileDescriptor(), map[string]string{
		"path": path, "content": "package newpkg\n",
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
	if string(got) != "package newpkg\n" {
		t.Errorf("file content = %q, want %q", string(got), "package newpkg\n")
	}
}

// GIVEN a target path several directory levels deep, none of which exist yet
// WHEN write_file runs
// THEN it creates the entire missing chain (MkdirAll, not just Mkdir) and
// writes the file.
func TestWriteFileCreatesNestedMissingDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "deep.txt")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), writeFileDescriptor(), map[string]string{
		"path": path, "content": "deep\n",
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
	if string(got) != "deep\n" {
		t.Errorf("file content = %q, want %q", string(got), "deep\n")
	}
}

// GIVEN a target path whose parent directory component already exists as a
// regular FILE, not a directory
// WHEN write_file runs
// THEN MkdirAll fails and that failure is reported as the result's error,
// not a panic or a silent no-op — the one new failure branch this change
// added.
func TestWriteFileParentPathIsARegularFile(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	path := filepath.Join(blocker, "out.txt")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), writeFileDescriptor(), map[string]string{
		"path": path, "content": "hello\n",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (MkdirAll must fail when a path component is a regular file)", res.Status)
	}
	if res.Stderr == "" {
		t.Error("Stderr = \"\", want the MkdirAll failure reason")
	}
}

// GIVEN a target path with a relative (non-nested) filename — Dir() is "."
// WHEN write_file runs
// THEN it still writes successfully; MkdirAll(".") must not be attempted in
// a way that errors on an existing/no-op current directory.
func TestWriteFileRelativeFilenameInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	e := newTestExecutor()
	res, err := e.Run(context.Background(), writeFileDescriptor(), map[string]string{
		"path": "out.txt", "content": "cwd\n",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "cwd\n" {
		t.Errorf("file content = %q, want %q", string(got), "cwd\n")
	}
}

// --- grep_files ---------------------------------------------------------

// GIVEN a single file containing exactly one line matching pattern
// WHEN grep_files runs with path set to that file
// THEN it returns one "path:line: text" match, searching only that file.
func TestGrepFilesSingleFileMatch(t *testing.T) {
	path := writeTempFile(t, "hello world\ngoodbye world\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": path, "pattern": "^hello",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}
	want := path + ":1: hello world"
	if res.Preview != want {
		t.Errorf("Preview = %q, want %q", res.Preview, want)
	}
	if res.Lines != 1 {
		t.Errorf("Lines = %d, want 1", res.Lines)
	}
}

// GIVEN a directory tree with matches spread across more than one file
// WHEN grep_files runs with path set to the directory root
// THEN it walks recursively and returns matches from every file.
func TestGrepFilesDirectoryTreeMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("func Foo() {}\n"), 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.go"), []byte("func Bar() {}\nfunc FooBar() {}\n"), 0o644); err != nil {
		t.Fatalf("write b.go: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": dir, "pattern": `^func Foo\(\)`,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}
	if res.Lines != 1 {
		t.Fatalf("Lines = %d, want 1 (only a.go's line matches ^func Foo(), not FooBar())", res.Lines)
	}
	if !strings.Contains(res.Preview, "a.go:1:") {
		t.Errorf("Preview = %q, want it to include a.go's match", res.Preview)
	}
}

// GIVEN more matching lines than max_results
// WHEN grep_files runs with max_results set
// THEN exactly that many matches are returned, not the full match set.
func TestGrepFilesMaxResultsTruncates(t *testing.T) {
	var lines []string
	for range 10 {
		lines = append(lines, "target line")
	}
	path := writeTempFile(t, strings.Join(lines, "\n")+"\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": path, "pattern": "target", "max_results": "3",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Lines != 3 {
		t.Fatalf("Lines = %d, want 3 (max_results must cap the match count)", res.Lines)
	}
}

// GIVEN a file that looks binary (a NUL byte in its first bytes)
// WHEN grep_files walks over it as part of a directory search
// THEN it is skipped without error and without appearing in the results —
// matches from other files in the same directory still come back.
func TestGrepFilesSkipsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "text.txt"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatalf("write text.txt: %v", err)
	}
	binary := append([]byte("needle\x00"), make([]byte, 100)...)
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), binary, 0o644); err != nil {
		t.Fatalf("write blob.bin: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": dir, "pattern": "needle",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Lines != 1 {
		t.Fatalf("Lines = %d, want 1 (blob.bin's NUL byte must exclude it, even though it also contains \"needle\")", res.Lines)
	}
	if !strings.Contains(res.Preview, "text.txt") {
		t.Errorf("Preview = %q, want the text.txt match", res.Preview)
	}
}

// GIVEN a file the process cannot read (permission denied)
// WHEN grep_files walks over it as part of a directory search
// THEN it is skipped without error; matches from every other readable file
// in the tree still come back.
func TestGrepFilesSkipsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission bits; this test needs a non-root process")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatalf("write readable.txt: %v", err)
	}
	blocked := filepath.Join(dir, "blocked.txt")
	if err := os.WriteFile(blocked, []byte("needle blocked\n"), 0o644); err != nil {
		t.Fatalf("write blocked.txt: %v", err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatalf("chmod unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o644) })
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": dir, "pattern": "needle",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Lines != 1 {
		t.Fatalf("Lines = %d, want 1 (blocked.txt must be skipped, not fatal to the search)", res.Lines)
	}
	if !strings.Contains(res.Preview, "readable.txt") {
		t.Errorf("Preview = %q, want the readable.txt match", res.Preview)
	}
}

// GIVEN a pattern that is not valid RE2 regexp syntax
// WHEN grep_files runs
// THEN it fails with the compile error as the result's failure reason, not
// a panic.
func TestGrepFilesInvalidPattern(t *testing.T) {
	path := writeTempFile(t, "hello\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": path, "pattern": "(unclosed",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
}

// GIVEN a path that does not exist
// WHEN grep_files runs
// THEN it fails with a clear error, not a panic.
func TestGrepFilesNonexistentPath(t *testing.T) {
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": filepath.Join(t.TempDir(), "does-not-exist"), "pattern": "x",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
}

// GIVEN a directory tree with a vendored subdirectory (e.g. node_modules)
// containing a matching line
// WHEN grep_files walks the tree
// THEN it excludes that subdirectory — the same exclusion list tree already
// applies, shared as one Go slice (excludedDirs), not a second hand-copied
// list.
func TestGrepFilesExcludesVendoredDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.go"), []byte("needle real\n"), 0o644); err != nil {
		t.Fatalf("write real.go: %v", err)
	}
	nm := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nm, "vendored.js"), []byte("needle vendored\n"), 0o644); err != nil {
		t.Fatalf("write vendored.js: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": dir, "pattern": "needle",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Lines != 1 {
		t.Fatalf("Lines = %d, want 1 (node_modules must be excluded)", res.Lines)
	}
	if strings.Contains(res.Preview, "vendored") {
		t.Errorf("Preview = %q, must not include the excluded node_modules match", res.Preview)
	}
}

// --- delete_file ----------------------------------------------------------

// GIVEN a path naming an existing regular file
// WHEN delete_file runs
// THEN the file is removed from disk.
func TestDeleteFileRemovesExistingFile(t *testing.T) {
	path := writeTempFile(t, "bye\n")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), deleteFileDescriptor, map[string]string{"path": path})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists after delete_file, stat err = %v", err)
	}
}

// GIVEN a path naming an existing directory
// WHEN delete_file runs
// THEN it fails without removing anything — directories are categorically
// out of scope for this tool.
func TestDeleteFileRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "subdir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), deleteFileDescriptor, map[string]string{"path": target})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (a directory must be refused)", res.Status)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("directory was removed despite the refusal: stat err = %v", err)
	}
}

// GIVEN a symlink whose target is a directory
// WHEN delete_file runs on the symlink's own path
// THEN the symlink itself is removed (classified by Lstat, not by resolving
// the target) — its target directory is left untouched.
func TestDeleteFileRemovesSymlinkToDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "realdir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), deleteFileDescriptor, map[string]string{"path": link})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok — removing a symlink itself must succeed even though its target is a directory (stderr: %s)", res.Status, res.Stderr)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("symlink still exists after delete_file, lstat err = %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target directory was affected by removing the symlink: stat err = %v", err)
	}
}

// GIVEN a path that does not exist
// WHEN delete_file runs
// THEN it fails with a clear error, not a silent no-op.
func TestDeleteFileNonexistentPath(t *testing.T) {
	e := newTestExecutor()

	res, err := e.Run(context.Background(), deleteFileDescriptor, map[string]string{
		"path": filepath.Join(t.TempDir(), "does-not-exist"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
}

// GIVEN a file whose containing directory denies write permission (removal
// requires write+execute on the parent directory, not on the file itself)
// WHEN delete_file runs
// THEN it fails with the permission error as the result's failure reason.
func TestDeleteFilePermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission bits; this test needs a non-root process")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // let t.TempDir() clean up afterward
	e := newTestExecutor()

	res, err := e.Run(context.Background(), deleteFileDescriptor, map[string]string{"path": path})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (removal requires write on the parent directory)", res.Status)
	}
}

// --- move_file --------------------------------------------------------

// GIVEN an existing file and a destination in the same directory that does
// not yet exist
// WHEN move_file runs
// THEN the file is renamed: gone from `from`, present at `to`.
func TestMoveFileRenamesWithinSameDirectory(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(from, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	to := filepath.Join(dir, "new.txt")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), moveFileDescriptor, map[string]string{"from": from, "to": to})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("source still exists after move_file, stat err = %v", err)
	}
	got, err := os.ReadFile(to)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(got) != "content\n" {
		t.Errorf("destination content = %q, want %q", string(got), "content\n")
	}
}

// GIVEN a destination whose parent directory does not exist yet
// WHEN move_file runs
// THEN that directory tree is created first, then the file is moved into
// it — the same missing-parent-directory fix write_file received.
func TestMoveFileCreatesMissingDestinationDirectory(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(from, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	to := filepath.Join(dir, "newpkg", "new.txt")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), moveFileDescriptor, map[string]string{"from": from, "to": to})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}
	got, err := os.ReadFile(to)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(got) != "content\n" {
		t.Errorf("destination content = %q, want %q", string(got), "content\n")
	}
}

// GIVEN a destination that already exists
// WHEN move_file runs
// THEN it fails without moving anything — the source is left in place and
// the pre-existing destination is left untouched, never silently
// overwritten.
func TestMoveFileRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(from, []byte("source\n"), 0o644); err != nil {
		t.Fatalf("write from: %v", err)
	}
	to := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(to, []byte("preexisting\n"), 0o644); err != nil {
		t.Fatalf("write to: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), moveFileDescriptor, map[string]string{"from": from, "to": to})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (must refuse to overwrite an existing destination)", res.Status)
	}
	gotFrom, err := os.ReadFile(from)
	if err != nil || string(gotFrom) != "source\n" {
		t.Errorf("source content = %q, err=%v — want unchanged", string(gotFrom), err)
	}
	gotTo, err := os.ReadFile(to)
	if err != nil || string(gotTo) != "preexisting\n" {
		t.Errorf("destination content = %q, err=%v — want unchanged", string(gotTo), err)
	}
}

// GIVEN `from` names a directory, not a regular file
// WHEN move_file runs
// THEN it fails without moving anything — directories are out of scope for
// this tool, same as delete_file.
func TestMoveFileRefusesDirectorySource(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "subdir")
	if err := os.Mkdir(from, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	to := filepath.Join(dir, "renamed")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), moveFileDescriptor, map[string]string{"from": from, "to": to})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (a directory source must be refused)", res.Status)
	}
	if _, err := os.Stat(from); err != nil {
		t.Errorf("source directory was affected despite the refusal: stat err = %v", err)
	}
}

// GIVEN `from` does not exist
// WHEN move_file runs
// THEN it fails with a clear error.
func TestMoveFileNonexistentSource(t *testing.T) {
	dir := t.TempDir()
	e := newTestExecutor()

	res, err := e.Run(context.Background(), moveFileDescriptor, map[string]string{
		"from": filepath.Join(dir, "does-not-exist"), "to": filepath.Join(dir, "new.txt"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
}

// GIVEN a destination whose parent path component already exists as a
// regular FILE, not a directory
// WHEN move_file runs
// THEN MkdirAll fails and that failure is reported as the result's error —
// the same failure shape write_file's equivalent test covers.
func TestMoveFileDestinationParentPathIsARegularFile(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(from, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write from: %v", err)
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	to := filepath.Join(blocker, "new.txt")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), moveFileDescriptor, map[string]string{"from": from, "to": to})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (MkdirAll must fail when a path component is a regular file)", res.Status)
	}
}

// GIVEN a source whose containing directory denies write permission (a
// rename requires removing the entry from the source directory, same as
// delete_file's removal requirement)
// WHEN move_file runs
// THEN os.Rename fails and that failure is reported as the result's error.
func TestMoveFileRenamePermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission bits; this test needs a non-root process")
	}
	srcDir := t.TempDir()
	from := filepath.Join(srcDir, "old.txt")
	if err := os.WriteFile(from, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write from: %v", err)
	}
	if err := os.Chmod(srcDir, 0o555); err != nil {
		t.Fatalf("chmod source dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(srcDir, 0o755) }) // let t.TempDir() clean up afterward
	to := filepath.Join(t.TempDir(), "new.txt")
	e := newTestExecutor()

	res, err := e.Run(context.Background(), moveFileDescriptor, map[string]string{"from": from, "to": to})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (rename requires write on the source directory)", res.Status)
	}
}

// GIVEN a directory search where the total match count reaches max_results
// partway through the walk (not on the very first file)
// WHEN grep_files runs
// THEN it stops walking further files (filepath.SkipAll) instead of
// continuing to scan files whose matches could never be returned.
func TestGrepFilesStopsWalkOnceMaxResultsReached(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle\nneedle\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("needle\nneedle\n"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": dir, "pattern": "needle", "max_results": "2",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Lines != 2 {
		t.Fatalf("Lines = %d, want 2 (max_results must cap the total across the whole directory walk)", res.Lines)
	}
}

// GIVEN a subdirectory the process cannot read (permission denied on the
// directory itself, so WalkDir cannot even list its contents)
// WHEN grep_files walks the tree
// THEN that subdirectory is skipped without error; matches from every other
// readable file in the tree still come back.
func TestGrepFilesSkipsUnreadableSubdirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission bits; this test needs a non-root process")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatalf("write readable.txt: %v", err)
	}
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir blocked: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "inside.txt"), []byte("needle inside\n"), 0o644); err != nil {
		t.Fatalf("write inside.txt: %v", err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatalf("chmod unreadable dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) }) // let t.TempDir() clean up afterward
	e := newTestExecutor()

	res, err := e.Run(context.Background(), grepFilesDescriptor, map[string]string{
		"path": dir, "pattern": "needle",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Lines != 1 {
		t.Fatalf("Lines = %d, want 1 (the unreadable subdirectory must be skipped, not fatal to the search)", res.Lines)
	}
	if !strings.Contains(res.Preview, "readable.txt") {
		t.Errorf("Preview = %q, want the readable.txt match", res.Preview)
	}
}

var gitDescriptor = Descriptor{ID: "git", Builtin: "git", TimeoutSeconds: 10}

// initTestRepo creates a fresh git repository in a temp dir with a local
// (not global) user.name/email, so `git commit` works in a sandboxed test
// environment without touching the machine's real git config.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")
	return dir
}

func gitArgs(t *testing.T, path string, argv ...string) map[string]string {
	t.Helper()
	raw, err := json.Marshal(argv)
	if err != nil {
		t.Fatalf("marshal argv: %v", err)
	}
	return map[string]string{"path": path, "args": string(raw)}
}

// GIVEN a fresh git repository and a new file
// WHEN the git builtin runs "add" then "commit -m ..." as two separate calls
// THEN the file lands in the repository's history — proving the tool can
// actually mutate git state, not just read it (unlike the existing
// read-only git_status tool).
func TestGitAddThenCommitCreatesHistory(t *testing.T) {
	dir := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write hello.txt: %v", err)
	}
	e := newTestExecutor()

	res, err := e.Run(context.Background(), gitDescriptor, gitArgs(t, dir, "add", "hello.txt"))
	if err != nil {
		t.Fatalf("Run(add): %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("add Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	res, err = e.Run(context.Background(), gitDescriptor, gitArgs(t, dir, "commit", "-m", "add hello"))
	if err != nil {
		t.Fatalf("Run(commit): %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("commit Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	log := exec.Command("git", "-C", dir, "log", "--oneline")
	out, err := log.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), "add hello") {
		t.Errorf("git log = %q, want a commit titled %q", out, "add hello")
	}
}

// GIVEN a git repository
// WHEN the git builtin runs a subcommand with no bearing on read/write
// (e.g. branch -D on a nonexistent branch) that no other AgentX git tool
// exposes
// THEN it reaches real git and fails with git's own error — proving no
// AgentX-side subcommand allowlist stands between the model and arbitrary
// git functionality, per the explicit "no restrictions" product decision.
func TestGitRunsArbitrarySubcommandNotJustStatus(t *testing.T) {
	dir := initTestRepo(t)
	e := newTestExecutor()

	res, err := e.Run(context.Background(), gitDescriptor, gitArgs(t, dir, "branch", "-D", "does-not-exist"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (git itself rejects deleting a nonexistent branch)", res.Status)
	}
	if !strings.Contains(res.Stderr, "does-not-exist") {
		t.Errorf("Stderr = %q, want git's own branch-not-found message", res.Stderr)
	}
}

// GIVEN a git repository
// WHEN the git builtin runs a read-only subcommand (log) against an empty
// history
// THEN it reports the real nonzero exit git itself returns — the tool
// reports git's outcome faithfully rather than swallowing it.
func TestGitReportsNonZeroExit(t *testing.T) {
	dir := initTestRepo(t)
	e := newTestExecutor()

	res, err := e.Run(context.Background(), gitDescriptor, gitArgs(t, dir, "log"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error (git log on a repo with no commits exits nonzero)", res.Status)
	}
}

// GIVEN args that is not valid JSON
// WHEN the git builtin runs
// THEN it fails cleanly instead of passing a garbage token straight to argv.
func TestGitInvalidJSONArgsFails(t *testing.T) {
	dir := initTestRepo(t)
	e := newTestExecutor()

	res, err := e.Run(context.Background(), gitDescriptor, map[string]string{
		"path": dir, "args": "not valid json",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
}

// GIVEN args that decodes to an empty JSON array
// WHEN the git builtin runs
// THEN it fails cleanly rather than invoking bare `git -C path` with no
// subcommand.
func TestGitEmptyArgsArrayFails(t *testing.T) {
	dir := initTestRepo(t)
	e := newTestExecutor()

	res, err := e.Run(context.Background(), gitDescriptor, map[string]string{
		"path": dir, "args": "[]",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("Status = %q, want error", res.Status)
	}
}

// GIVEN a commit message containing spaces and punctuation
// WHEN the git builtin runs "commit" with that message as one JSON array
// element
// THEN the full message survives intact in history — proving the JSON-array
// argv shape (not a shell string) correctly carries a multi-word argument
// without splitting or quoting corruption.
func TestGitCommitMessageWithSpacesSurvivesIntact(t *testing.T) {
	dir := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write f.txt: %v", err)
	}
	e := newTestExecutor()
	if _, err := e.Run(context.Background(), gitDescriptor, gitArgs(t, dir, "add", "f.txt")); err != nil {
		t.Fatalf("Run(add): %v", err)
	}

	const msg = `fix: handle "quoted" edge cases & spaces`
	res, err := e.Run(context.Background(), gitDescriptor, gitArgs(t, dir, "commit", "-m", msg))
	if err != nil {
		t.Fatalf("Run(commit): %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (stderr: %s)", res.Status, res.Stderr)
	}

	log := exec.Command("git", "-C", dir, "log", "-1", "--pretty=%B")
	out, err := log.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), msg) {
		t.Errorf("commit message = %q, want it to contain %q intact", out, msg)
	}
}
