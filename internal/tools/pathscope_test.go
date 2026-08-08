package tools

import "testing"

// GIVEN a relative path and a project root
// WHEN ClassifyPath resolves it
// THEN it's classified inside the project, with the extension derived and
// lowercased.
func TestClassifyPathInsideRelative(t *testing.T) {
	scope := ClassifyPath("internal/foo.GO", "/repo")
	if !scope.InsideProject {
		t.Errorf("InsideProject = false, want true")
	}
	if scope.Ext != ".go" {
		t.Errorf("Ext = %q, want %q (lowercased)", scope.Ext, ".go")
	}
	if scope.Resolved != "/repo/internal/foo.GO" {
		t.Errorf("Resolved = %q, want %q", scope.Resolved, "/repo/internal/foo.GO")
	}
}

// GIVEN an absolute path that happens to live under the project root
// WHEN ClassifyPath resolves it
// THEN it's still classified inside — absolute vs. relative input doesn't
// change the classification, only how it's joined.
func TestClassifyPathInsideAbsolute(t *testing.T) {
	scope := ClassifyPath("/repo/internal/foo.go", "/repo")
	if !scope.InsideProject {
		t.Errorf("InsideProject = false, want true")
	}
}

// GIVEN an absolute path outside the project root
// WHEN ClassifyPath resolves it
// THEN it's classified outside.
func TestClassifyPathOutsideAbsolute(t *testing.T) {
	scope := ClassifyPath("/etc/hosts", "/repo")
	if scope.InsideProject {
		t.Error("InsideProject = true, want false")
	}
	if scope.Resolved != "/etc/hosts" {
		t.Errorf("Resolved = %q, want %q", scope.Resolved, "/etc/hosts")
	}
	if scope.Ext != "" {
		t.Errorf("Ext = %q, want empty (no extension)", scope.Ext)
	}
}

// GIVEN a relative path that traverses upward out of the project root via
// ".." (the security-critical case — a naive string-prefix check would
// misclassify this as inside, since the raw string starts with the project
// root's own components before the traversal)
// WHEN ClassifyPath resolves it
// THEN it's correctly classified outside, because resolution uses
// Clean+Rel, not string prefix matching.
func TestClassifyPathTraversalEscapesRoot(t *testing.T) {
	scope := ClassifyPath("../../etc/passwd", "/repo/internal")
	if scope.InsideProject {
		t.Error("InsideProject = true, want false — a \"..\" traversal must not be classified as inside")
	}
}

// GIVEN an empty projectRoot (the session's cwd fact absent, disabled, or
// deleted)
// WHEN ClassifyPath resolves any path, even one that looks project-shaped
// THEN it always classifies outside — the conservative, fail-closed default
// when the boundary can't be determined at all.
func TestClassifyPathEmptyProjectRootFailsClosed(t *testing.T) {
	scope := ClassifyPath("internal/foo.go", "")
	if scope.InsideProject {
		t.Error("InsideProject = true, want false when projectRoot is empty (must fail closed)")
	}
}

// GIVEN a path with no extension
// WHEN ClassifyPath resolves it
// THEN Ext is the empty string, not a placeholder like "*" or ".".
func TestClassifyPathNoExtension(t *testing.T) {
	scope := ClassifyPath("Makefile", "/repo")
	if scope.Ext != "" {
		t.Errorf("Ext = %q, want empty for an extensionless file", scope.Ext)
	}
}

// GIVEN a path that resolves to exactly the project root itself
// WHEN ClassifyPath resolves it
// THEN it's classified inside (rel == "."), not outside.
func TestClassifyPathEqualToRoot(t *testing.T) {
	scope := ClassifyPath("/repo", "/repo")
	if !scope.InsideProject {
		t.Error("InsideProject = false, want true when the path equals the project root")
	}
}

// GIVEN a write_file call targeting a .go file inside the project
// WHEN ScopeArgs computes the approval scope
// THEN it returns exactly one scope map keyed by extension only — no path,
// no content — so a different .go file's call produces the identical map.
func TestScopeArgsWriteFileInsideProject(t *testing.T) {
	d := Descriptor{ID: "write_file"}
	got := d.ScopeArgs(map[string]string{"path": "internal/foo.go", "content": "package foo"}, "/repo")
	if len(got) != 1 {
		t.Fatalf("ScopeArgs = %v, want 1 entry", got)
	}
	want := map[string]string{"ext": ".go"}
	if got[0]["ext"] != want["ext"] || got[0]["path"] != "" {
		t.Errorf("ScopeArgs[0] = %v, want %v (no path key for an inside-project scope)", got[0], want)
	}

	// A different file, same extension, different content: identical scope.
	got2 := d.ScopeArgs(map[string]string{"path": "cmd/bar.go", "content": "package bar; totally different"}, "/repo")
	if got[0]["ext"] != got2[0]["ext"] || got[0]["path"] != got2[0]["path"] {
		t.Errorf("two different .go files produced different scopes: %v vs %v, want identical", got[0], got2[0])
	}
}

// GIVEN an edit_file call targeting a path outside the project
// WHEN ScopeArgs computes the approval scope
// THEN the resolved path is part of the key, so a different outside-project
// path is a different scope — never one blanket "outside project" bucket.
func TestScopeArgsOutsideProjectIncludesPath(t *testing.T) {
	d := Descriptor{ID: "edit_file"}
	got := d.ScopeArgs(map[string]string{"path": "/etc/hosts", "old_string": "a", "new_string": "b"}, "/repo")
	if len(got) != 1 || got[0]["path"] != "/etc/hosts" {
		t.Fatalf("ScopeArgs = %v, want [{ext: \"\", path: /etc/hosts}]", got)
	}

	got2 := d.ScopeArgs(map[string]string{"path": "/etc/shadow", "old_string": "a", "new_string": "b"}, "/repo")
	if got[0]["path"] == got2[0]["path"] {
		t.Error("two different outside-project paths produced the same scope key — a single approval must not cover both")
	}
}

// GIVEN a tool this scoping change doesn't touch (e.g. read_file)
// WHEN ScopeArgs is called
// THEN it returns the call's own args unchanged, in a single-element slice —
// today's exact per-call behavior, untouched.
func TestScopeArgsUnscopedToolUnchanged(t *testing.T) {
	d := Descriptor{ID: "read_file"}
	args := map[string]string{"path": "internal/foo.go"}
	got := d.ScopeArgs(args, "/repo")
	if len(got) != 1 || got[0]["path"] != "internal/foo.go" {
		t.Fatalf("ScopeArgs = %v, want the original args unchanged", got)
	}
}
