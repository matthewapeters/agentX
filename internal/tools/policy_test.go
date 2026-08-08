package tools

import "testing"

// writeFileDescriptor and editFileDescriptor mirror the registry entries in
// descriptors.go closely enough to exercise Policy's scope-key behavior
// without depending on DefaultRegistry() (these tests care about scoping
// mechanics, not the full descriptor catalog).
func writeFileDescriptor() Descriptor {
	return Descriptor{
		ID: "write_file", Builtin: "write_file", Risk: RiskWrite, RequiresApproval: true,
		Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}, {Name: "content", Kind: KindString, Required: true}},
	}
}

func applyPatchDescriptor() Descriptor {
	return Descriptor{
		ID: "apply_patch", Command: "patch", Argv: []string{"patch", "-p0"}, StdinArg: "patch",
		Risk: RiskWrite, RequiresApproval: true,
		Args: []ArgSpec{{Name: "patch", Kind: KindString, Required: true}},
	}
}

func deleteFileDescriptorForPolicy() Descriptor {
	return Descriptor{
		ID: "delete_file", Builtin: "delete_file", Risk: RiskWrite, RequiresApproval: true,
		Args: []ArgSpec{{Name: "path", Kind: KindPath, Required: true}},
	}
}

func moveFileDescriptorForPolicy() Descriptor {
	return Descriptor{
		ID: "move_file", Builtin: "move_file", Risk: RiskWrite, RequiresApproval: true,
		Args: []ArgSpec{
			{Name: "from", Kind: KindPath, Required: true},
			{Name: "to", Kind: KindPath, Required: true},
		},
	}
}

// GIVEN a global approval for one .txt file's deletion inside the project
// WHEN a different .txt file's deletion inside the same project is evaluated
// THEN it's allowed without a second prompt — delete_file scopes through
// ScopeArgs identically to write_file/edit_file.
func TestPolicyDeleteFileApprovalInsideProjectCoversSameExtension(t *testing.T) {
	p := NewPolicy()
	d := deleteFileDescriptorForPolicy()
	root := "/repo"

	p.Approve(ScopeGlobal, d, map[string]string{"path": "/repo/a.txt"}, root)

	v := p.Evaluate(d, map[string]string{"path": "/repo/b.txt"}, root)
	if v.Decision != Allow {
		t.Fatalf("Evaluate(different .txt file) = %v, want Allow (same verb+extension+in-project scope)", v.Decision)
	}
}

// GIVEN a global approval for a path outside the project
// WHEN a different outside-project path's deletion is evaluated
// THEN it still needs approval — outside the project, delete_file scopes
// per exact path, same as write_file/edit_file.
func TestPolicyDeleteFileApprovalOutsideProjectDoesNotCoverDifferentPath(t *testing.T) {
	p := NewPolicy()
	d := deleteFileDescriptorForPolicy()
	root := "/repo"

	p.Approve(ScopeGlobal, d, map[string]string{"path": "/tmp/a.txt"}, root)

	v := p.Evaluate(d, map[string]string{"path": "/tmp/b.txt"}, root)
	if v.Decision != NeedsApproval {
		t.Fatalf("Evaluate(different outside-project path) = %v, want NeedsApproval", v.Decision)
	}
}

// GIVEN a move whose from/to are both inside the project with the same
// extension
// WHEN the move is approved once
// THEN both the from-scope and the to-scope are covered — approving a move
// requires, and then covers, both of its paths, mirroring apply_patch's
// "approving a patch approves every file it touched" completeness rule
// applied to a two-path call.
func TestPolicyMoveFileApprovalCoversBothFromAndTo(t *testing.T) {
	p := NewPolicy()
	d := moveFileDescriptorForPolicy()
	root := "/repo"

	p.Approve(ScopeGlobal, d, map[string]string{"from": "/repo/old.go", "to": "/repo/new.go"}, root)

	// A different from/to pair, same extension, same project — reuses the
	// scope both paths already established.
	v := p.Evaluate(d, map[string]string{"from": "/repo/a.go", "to": "/repo/b.go"}, root)
	if v.Decision != Allow {
		t.Fatalf("Evaluate(different .go from/to pair) = %v, want Allow", v.Decision)
	}
}

// GIVEN a move whose `to` has a different extension than any previously
// approved move
// WHEN the move is evaluated
// THEN it still needs approval — approving one extension's scope must not
// silently cover a different one just because it shares the same call.
func TestPolicyMoveFileApprovalRequiresBothScopes(t *testing.T) {
	p := NewPolicy()
	d := moveFileDescriptorForPolicy()
	root := "/repo"

	// Only the .go extension has ever been approved.
	p.Approve(ScopeGlobal, d, map[string]string{"from": "/repo/old.go", "to": "/repo/new.go"}, root)

	// This move's `to` is a .md file — a scope never approved.
	v := p.Evaluate(d, map[string]string{"from": "/repo/a.go", "to": "/repo/notes.md"}, root)
	if v.Decision != NeedsApproval {
		t.Fatalf("Evaluate(from=.go approved, to=.md never approved) = %v, want NeedsApproval — approving one scope must not cover an unapproved one", v.Decision)
	}
}

// GIVEN a global approval for one .go file inside the project
// WHEN a different .go file inside the same project is evaluated
// THEN it's allowed without a second prompt — the scope key is verb+extension
// inside the project, not the call's specific path/content.
func TestPolicyApprovalInsideProjectCoversSameExtension(t *testing.T) {
	p := NewPolicy()
	d := writeFileDescriptor()
	root := "/repo"

	p.Approve(ScopeGlobal, d, map[string]string{"path": "/repo/a.go", "content": "package a"}, root)

	v := p.Evaluate(d, map[string]string{"path": "/repo/b.go", "content": "package b"}, root)
	if v.Decision != Allow {
		t.Fatalf("Evaluate(different .go file) = %v, want Allow (same verb+extension+in-project scope)", v.Decision)
	}
}

// GIVEN a global approval for a .go file inside the project
// WHEN a .md file inside the same project is evaluated
// THEN it still needs approval — extension is part of the scope key.
func TestPolicyApprovalInsideProjectDoesNotCoverDifferentExtension(t *testing.T) {
	p := NewPolicy()
	d := writeFileDescriptor()
	root := "/repo"

	p.Approve(ScopeGlobal, d, map[string]string{"path": "/repo/a.go", "content": "package a"}, root)

	v := p.Evaluate(d, map[string]string{"path": "/repo/README.md", "content": "# hi"}, root)
	if v.Decision != NeedsApproval {
		t.Fatalf("Evaluate(different extension) = %v, want NeedsApproval", v.Decision)
	}
}

// GIVEN a global approval for a path outside the project
// WHEN a different path outside the project (same extension) is evaluated
// THEN it still needs approval — outside the project, the scope is per exact
// path, never a blanket "outside project" bucket (the path="/" ext="*" risk).
func TestPolicyApprovalOutsideProjectDoesNotCoverDifferentPath(t *testing.T) {
	p := NewPolicy()
	d := writeFileDescriptor()
	root := "/repo"

	p.Approve(ScopeGlobal, d, map[string]string{"path": "/etc/hosts", "content": "127.0.0.1 x"}, root)

	v := p.Evaluate(d, map[string]string{"path": "/etc/passwd", "content": "root:x"}, root)
	if v.Decision != NeedsApproval {
		t.Fatalf("Evaluate(different outside-project path) = %v, want NeedsApproval", v.Decision)
	}
}

// GIVEN a global approval for a path outside the project
// WHEN the exact same outside-project path is evaluated again
// THEN it's allowed — the outside-project scope key does cover repeats of
// the identical path.
func TestPolicyApprovalOutsideProjectCoversSamePath(t *testing.T) {
	p := NewPolicy()
	d := writeFileDescriptor()
	root := "/repo"

	p.Approve(ScopeGlobal, d, map[string]string{"path": "/etc/hosts", "content": "127.0.0.1 x"}, root)

	v := p.Evaluate(d, map[string]string{"path": "/etc/hosts", "content": "127.0.0.1 y"}, root)
	if v.Decision != Allow {
		t.Fatalf("Evaluate(same outside-project path, different content) = %v, want Allow", v.Decision)
	}
}

// GIVEN a multi-file unified diff touching two files with different
// extensions
// WHEN the patch is approved once
// THEN every file it touched is covered — approving a patch approves the
// whole patch, not just one of its files.
func TestPolicyApprovePatchCoversEveryTouchedFile(t *testing.T) {
	p := NewPolicy()
	d := applyPatchDescriptor()
	root := "/repo"
	patch := "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n" +
		"--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-old\n+new\n"

	p.Approve(ScopeGlobal, d, map[string]string{"patch": patch}, root)

	// A second call to apply_patch touching just one of the same two files
	// (e.g. a follow-up patch limited to main.go) reuses the .go scope key.
	followUp := "--- a/main.go\n+++ b/main.go\n@@ -2 +2 @@\n-x\n+y\n"
	v := p.Evaluate(d, map[string]string{"patch": followUp}, root)
	if v.Decision != Allow {
		t.Fatalf("Evaluate(follow-up patch touching only main.go) = %v, want Allow", v.Decision)
	}

	// And a follow-up limited to README.md also reuses its scope key.
	followUpMd := "--- a/README.md\n+++ b/README.md\n@@ -2 +2 @@\n-x\n+y\n"
	v = p.Evaluate(d, map[string]string{"patch": followUpMd}, root)
	if v.Decision != Allow {
		t.Fatalf("Evaluate(follow-up patch touching only README.md) = %v, want Allow", v.Decision)
	}
}

// GIVEN a global approval recorded with an empty projectRoot (the session's
// cwd fact absent/disabled)
// WHEN the same call is evaluated later with a real projectRoot
// THEN it still needs approval — ClassifyPath's fail-closed "no root ->
// outside" posture means the two scope keys differ (the first run's path was
// classified outside; a real root may now classify it inside), so this is
// not a reuse regression, just documents the fail-closed boundary.
func TestPolicyApprovalWithoutProjectRootDoesNotLeakAcrossRootChange(t *testing.T) {
	p := NewPolicy()
	d := writeFileDescriptor()

	p.Approve(ScopeGlobal, d, map[string]string{"path": "/repo/a.go", "content": "package a"}, "")

	v := p.Evaluate(d, map[string]string{"path": "/repo/b.go", "content": "package b"}, "/repo")
	if v.Decision != NeedsApproval {
		t.Fatalf("Evaluate(same file under a newly-known root) = %v, want NeedsApproval (scope key changed)", v.Decision)
	}
}
