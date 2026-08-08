package tools

import (
	"reflect"
	"sort"
	"testing"
)

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// GIVEN a plain (non-git-prefixed) single-file unified diff, matching how
// apply_patch's `patch -p0` actually expects paths to be written
// WHEN parsePatchPaths parses it
// THEN it extracts exactly that one path.
func TestParsePatchPathsSingleFile(t *testing.T) {
	patch := "--- foo.go\n+++ foo.go\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	paths, ok := parsePatchPaths(patch)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got := sortedCopy(paths); !reflect.DeepEqual(got, []string{"foo.go"}) {
		t.Errorf("paths = %v, want [foo.go]", got)
	}
}

// GIVEN a diff touching multiple files, each with its own --- /+++ header
// pair
// WHEN parsePatchPaths parses it
// THEN it extracts every distinct file, deduplicated.
func TestParsePatchPathsMultiFile(t *testing.T) {
	patch := "--- a.go\n+++ a.go\n@@ -1 +1 @@\n-x\n+y\n" +
		"--- b.go\n+++ b.go\n@@ -1 +1 @@\n-x\n+y\n"
	paths, ok := parsePatchPaths(patch)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got := sortedCopy(paths); !reflect.DeepEqual(got, []string{"a.go", "b.go"}) {
		t.Errorf("paths = %v, want [a.go b.go]", got)
	}
}

// GIVEN a diff using git's "diff --git a/X b/Y" header form
// WHEN parsePatchPaths parses it
// THEN both sides are extracted (redundant with the ---/+++ lines that
// normally follow, but harmless — dedup collapses the overlap).
func TestParsePatchPathsGitHeaderForm(t *testing.T) {
	patch := "diff --git a/foo.go b/foo.go\nindex abc..def 100644\n--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n-x\n+y\n"
	paths, ok := parsePatchPaths(patch)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	// a/foo.go and b/foo.go are literal, distinct strings here (no prefix
	// stripping — see parsePatchPaths' doc comment on why), so both survive.
	want := []string{"a/foo.go", "b/foo.go"}
	if got := sortedCopy(paths); !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

// GIVEN a diff creating a new file, where the "before" side is /dev/null
// WHEN parsePatchPaths parses it
// THEN /dev/null is skipped and only the real target path survives.
func TestParsePatchPathsSkipsDevNull(t *testing.T) {
	patch := "--- /dev/null\n+++ new_file.go\n@@ -0,0 +1,1 @@\n+package foo\n"
	paths, ok := parsePatchPaths(patch)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !reflect.DeepEqual(paths, []string{"new_file.go"}) {
		t.Errorf("paths = %v, want [new_file.go]", paths)
	}
}

// GIVEN a header line whose path is followed by a tab and a timestamp, the
// standard unified-diff form some tools emit
// WHEN parsePatchPaths parses it
// THEN the timestamp is stripped and only the path survives.
func TestParsePatchPathsStripsTrailingTimestamp(t *testing.T) {
	patch := "--- foo.go\t2024-01-01 12:00:00.000000000 +0000\n+++ foo.go\t2024-01-01 12:00:01.000000000 +0000\n@@ -1 +1 @@\n-x\n+y\n"
	paths, ok := parsePatchPaths(patch)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !reflect.DeepEqual(paths, []string{"foo.go"}) {
		t.Errorf("paths = %v, want [foo.go] (timestamp stripped)", paths)
	}
}

// GIVEN text with no recognizable diff header at all
// WHEN parsePatchPaths parses it
// THEN ok is false — the caller must fall back to a safe default, not
// silently grant a coarser scope than could actually be proven.
func TestParsePatchPathsUnparseableFallsBack(t *testing.T) {
	paths, ok := parsePatchPaths("this is not a diff at all, just prose\nwith several\nlines\n")
	if ok {
		t.Errorf("ok = true, paths = %v, want false for unparseable input", paths)
	}
}

// GIVEN an apply_patch call whose diff touches two different files
// WHEN ScopeArgs computes the approval scope
// THEN it returns one scope entry per file, so approving the patch approves
// every file it touched.
func TestScopeArgsApplyPatchMultiFile(t *testing.T) {
	d := Descriptor{ID: "apply_patch"}
	patch := "--- a.go\n+++ a.go\n@@ -1 +1 @@\n-x\n+y\n" +
		"--- b.py\n+++ b.py\n@@ -1 +1 @@\n-x\n+y\n"
	got := d.ScopeArgs(map[string]string{"patch": patch}, "/repo")
	if len(got) != 2 {
		t.Fatalf("ScopeArgs = %v, want 2 entries (one per touched file)", got)
	}
	exts := map[string]bool{got[0]["ext"]: true, got[1]["ext"]: true}
	if !exts[".go"] || !exts[".py"] {
		t.Errorf("ScopeArgs extensions = %v, want .go and .py", exts)
	}
}

// GIVEN an apply_patch call whose patch text can't be parsed at all
// WHEN ScopeArgs computes the approval scope
// THEN it falls back to the call's own raw args (today's exact per-call
// behavior) rather than silently granting some coarser, unproven scope.
func TestScopeArgsApplyPatchUnparseableFallsBackToRawArgs(t *testing.T) {
	d := Descriptor{ID: "apply_patch"}
	args := map[string]string{"patch": "not a diff"}
	got := d.ScopeArgs(args, "/repo")
	if len(got) != 1 || got[0]["patch"] != "not a diff" {
		t.Fatalf("ScopeArgs = %v, want the original args unchanged", got)
	}
}
