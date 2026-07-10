package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPlansWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	plans, err := s.Plans(id.ID)
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}

	tree := &PlanTree{
		RootID: "task-1",
		Goal:   "review the project",
		Nodes: map[string]*PlanTreeNode{
			"task-1": {ID: "task-1", Goal: "review the project", Kind: "step", Status: "decomposed", Children: []string{"task-1-1"}},
			"task-1-1": {
				ID: "task-1-1", Goal: "list files", Kind: "task", Status: "done", Type: "command",
				Command: "ls -la", ResultText: "total 0", ResultOutcome: "executed", ResultRef: "artifacts/000000000001.txt",
			},
		},
	}
	if err := plans.Write(tree.RootID, tree); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(dir, id.ID, "plans", "task-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got PlanTree
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.RootID != "task-1" || got.Goal != "review the project" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got.Nodes))
	}
	leaf := got.Nodes["task-1-1"]
	if leaf == nil || leaf.Command != "ls -la" || leaf.ResultRef != "artifacts/000000000001.txt" {
		t.Fatalf("leaf node round-trip mismatch: %+v", leaf)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind after rename: err=%v", err)
	}
}

func TestPlansWriteAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	id, err := s.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	plans, err := s.Plans(id.ID)
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}

	tree := &PlanTree{RootID: "task-1", Goal: "first", Nodes: map[string]*PlanTreeNode{}}
	if err := plans.Write(tree.RootID, tree); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	tree.Goal = "second"
	tree.Nodes["task-1-1"] = &PlanTreeNode{ID: "task-1-1", Goal: "child", Kind: "task", Status: "done"}
	if err := plans.Write(tree.RootID, tree); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	path := filepath.Join(dir, id.ID, "plans", "task-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got PlanTree
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Goal != "second" || len(got.Nodes) != 1 {
		t.Fatalf("second write did not fully replace first: %+v", got)
	}
}
