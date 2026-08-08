package tools

import "testing"

// GIVEN the default registry
// WHEN grep_files/delete_file/move_file are looked up
// THEN each is present with the shape its behavior doc describes — a
// registration regression test, since DefaultRegistry() otherwise has no
// direct coverage of what it actually registers.
func TestDefaultRegistryIncludesNewFilesystemTools(t *testing.T) {
	r := DefaultRegistry()

	grep, ok := r.Lookup("grep_files")
	if !ok {
		t.Fatal(`Lookup("grep_files") = false, want the tool registered`)
	}
	if grep.Builtin != "grep_files" {
		t.Errorf("grep_files.Builtin = %q, want %q", grep.Builtin, "grep_files")
	}
	if grep.Risk != RiskRead {
		t.Errorf("grep_files.Risk = %q, want RiskRead — it never writes", grep.Risk)
	}
	if grep.RequiresApproval {
		t.Error("grep_files.RequiresApproval = true, want false — a read-only search needs no approval")
	}

	del, ok := r.Lookup("delete_file")
	if !ok {
		t.Fatal(`Lookup("delete_file") = false, want the tool registered`)
	}
	if del.Builtin != "delete_file" {
		t.Errorf("delete_file.Builtin = %q, want %q", del.Builtin, "delete_file")
	}
	if del.Risk != RiskWrite || !del.RequiresApproval {
		t.Errorf("delete_file Risk/RequiresApproval = %q/%v, want RiskWrite/true — same tier as write_file/edit_file", del.Risk, del.RequiresApproval)
	}

	mv, ok := r.Lookup("move_file")
	if !ok {
		t.Fatal(`Lookup("move_file") = false, want the tool registered`)
	}
	if mv.Builtin != "move_file" {
		t.Errorf("move_file.Builtin = %q, want %q", mv.Builtin, "move_file")
	}
	if mv.Risk != RiskWrite || !mv.RequiresApproval {
		t.Errorf("move_file Risk/RequiresApproval = %q/%v, want RiskWrite/true", mv.Risk, mv.RequiresApproval)
	}
	wantArgs := map[string]bool{"from": true, "to": true}
	if len(mv.Args) != 2 {
		t.Fatalf("move_file.Args = %+v, want exactly from/to", mv.Args)
	}
	for _, a := range mv.Args {
		if !wantArgs[a.Name] {
			t.Errorf("move_file.Args has unexpected arg %q", a.Name)
		}
		if !a.Required {
			t.Errorf("move_file.Args[%q].Required = false, want true", a.Name)
		}
	}
}

// GIVEN the default registry
// WHEN the read-only-only view is requested (Available(true))
// THEN grep_files appears in it and delete_file/move_file do not — Risk
// classification actually gates the read-only tool list, not just a label.
func TestDefaultRegistryReadOnlyViewIncludesGrepFilesOnly(t *testing.T) {
	r := DefaultRegistry()
	readOnly := r.Available(true)

	seen := make(map[string]bool, len(readOnly))
	for _, d := range readOnly {
		seen[d.ID] = true
	}
	if !seen["grep_files"] {
		t.Error(`Available(true) does not include "grep_files", want it present (RiskRead)`)
	}
	if seen["delete_file"] {
		t.Error(`Available(true) includes "delete_file", want it excluded (RiskWrite)`)
	}
	if seen["move_file"] {
		t.Error(`Available(true) includes "move_file", want it excluded (RiskWrite)`)
	}
}
