package executor

import (
	"os"
	"path/filepath"
	"testing"

	"agentx/internal/tools"
)

// TestVerifyReadToolsByOutput: a read's effect is its output — a successful list_dir on a
// directory verifies (the mellow-meadow first-leaf kill), an empty read does not.
func TestVerifyReadToolsByOutput(t *testing.T) {
	v := FSVerifier{}
	read := tools.Descriptor{ID: "list_dir", Risk: tools.RiskRead}

	ok := tools.Result{Status: "ok", Exit: 0, Bytes: 120, Preview: "cmd/ internal/ docs/"}
	if !v.Verify(read, map[string]string{"path": "/some/dir"}, ok) {
		t.Error("successful directory listing must verify (was the phantom bug)")
	}
	empty := tools.Result{Status: "ok", Exit: 0, Bytes: 0, Preview: ""}
	if v.Verify(read, map[string]string{"path": "/some/dir"}, empty) {
		t.Error("a read that returned nothing proved nothing")
	}
	failed := tools.Result{Status: "error", Exit: 1, Bytes: 10, Preview: "denied"}
	if v.Verify(read, map[string]string{"path": "/some/dir"}, failed) {
		t.Error("a failed run never verifies")
	}
}

// TestVerifyWriteToolsByTarget: the original write semantics are unchanged — the named
// file target must exist and be non-empty.
func TestVerifyWriteToolsByTarget(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(f, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := FSVerifier{}
	write := tools.Descriptor{ID: "write_file", Risk: tools.RiskWrite}
	ok := tools.Result{Status: "ok", Exit: 0}

	if !v.Verify(write, map[string]string{"path": f}, ok) {
		t.Error("landed write must verify")
	}
	if v.Verify(write, map[string]string{"path": filepath.Join(dir, "missing.txt")}, ok) {
		t.Error("write whose target never landed must not verify")
	}
	if v.Verify(write, map[string]string{"path": dir}, ok) {
		t.Error("directory target must not verify a write")
	}
}
