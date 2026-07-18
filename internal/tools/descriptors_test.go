package tools

import (
	"strings"
	"testing"
)

func TestRegistryValidateUnknownTool(t *testing.T) {
	r := DefaultRegistry()
	if err := r.Validate("no_such_tool", nil); err == nil {
		t.Error("Validate succeeded for an unknown tool, want an error")
	}
}

func TestRegistryValidateMissingRequiredArg(t *testing.T) {
	r := DefaultRegistry()
	if err := r.Validate("list_dir", map[string]string{}); err == nil {
		t.Error("Validate succeeded for list_dir with no path, want an error (path is required)")
	}
}

func TestRegistryValidateCompleteArgsSucceeds(t *testing.T) {
	r := DefaultRegistry()
	if err := r.Validate("list_dir", map[string]string{"path": "."}); err != nil {
		t.Errorf("Validate(list_dir, {path: .}) = %v, want nil", err)
	}
}

func TestRegistryDescribeMarksRequiredArgs(t *testing.T) {
	r := DefaultRegistry()
	desc := r.Describe("list_dir")
	if desc == "" {
		t.Fatal("Describe(list_dir) = \"\", want a non-empty contract")
	}
	if !strings.Contains(desc, "path") || !strings.Contains(desc, "required") {
		t.Errorf("Describe(list_dir) = %q, want it to name \"path\" as required", desc)
	}
}

func TestRegistryDescribeUnknownToolIsEmpty(t *testing.T) {
	r := DefaultRegistry()
	if desc := r.Describe("no_such_tool"); desc != "" {
		t.Errorf("Describe(no_such_tool) = %q, want empty", desc)
	}
}
