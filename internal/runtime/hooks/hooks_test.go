package hooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// GIVEN an empty Registry (Orchestrator.Start's default, no hooks configured)
// WHEN RunSync/RunAsync are called
// THEN both are no-ops and RunSync returns no error.
func TestEmptyRegistryIsNoop(t *testing.T) {
	r := NewRegistry()
	turn := &Turn{Prompt: "hi"}
	if err := r.RunSync(context.Background(), turn); err != nil {
		t.Fatalf("RunSync on empty registry: %v", err)
	}
	r.RunAsync(context.Background(), *turn) // must not block or panic
}

// GIVEN two sync hooks registered in order A, then B
// WHEN RunSync is called
// THEN A observes the turn before B, and B sees A's mutation — a serial chain,
// not independent snapshots.
func TestSyncHooksRunInRegistrationOrder(t *testing.T) {
	var order []string
	a := syncFunc(func(_ context.Context, turn *Turn) error {
		order = append(order, "a")
		turn.Prompt += "-a"
		return nil
	})
	b := syncFunc(func(_ context.Context, turn *Turn) error {
		order = append(order, "b")
		if turn.Prompt != "hi-a" {
			t.Errorf("hook B saw Prompt = %q, want hook A's mutation to be visible", turn.Prompt)
		}
		return nil
	})
	r := NewRegistry()
	r.RegisterSync(a)
	r.RegisterSync(b)

	turn := &Turn{Prompt: "hi"}
	if err := r.RunSync(context.Background(), turn); err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("order = %v, want [a b]", order)
	}
}

// GIVEN a sync hook that returns an error
// WHEN RunSync is called
// THEN the error propagates and any later hook in the chain does not run.
func TestSyncHookErrorAbortsChain(t *testing.T) {
	wantErr := errors.New("boom")
	ran := false
	r := NewRegistry()
	r.RegisterSync(syncFunc(func(context.Context, *Turn) error { return wantErr }))
	r.RegisterSync(syncFunc(func(context.Context, *Turn) error { ran = true; return nil }))

	err := r.RunSync(context.Background(), &Turn{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunSync err = %v, want %v", err, wantErr)
	}
	if ran {
		t.Fatal("a hook after the erroring one ran; the chain should have aborted")
	}
}

// GIVEN an async hook
// WHEN RunAsync is called with a turn, then the caller mutates its own copy
// THEN the hook's snapshot is unaffected — Go value semantics on Turn give the
// copy-not-reference guarantee the design relies on.
func TestAsyncHookGetsValueCopy(t *testing.T) {
	seen := make(chan string, 1)
	r := NewRegistry()
	r.RegisterAsync(asyncFunc(func(_ context.Context, turn Turn) {
		seen <- turn.Prompt
	}))

	turn := Turn{Prompt: "original"}
	r.RunAsync(context.Background(), turn)
	turn.Prompt = "mutated-after-dispatch" // must not affect what the hook already received

	if got := <-seen; got != "original" {
		t.Fatalf("async hook saw Prompt = %q, want %q", got, "original")
	}
}

// GIVEN a fresh context
// WHEN SpawnDepth/CanSpawn/NextSpawnContext are used to track recursive
// loop-spawn nesting
// THEN depth starts at 0, CanSpawn respects the max, and NextSpawnContext
// increments depth for the child context without mutating the parent's.
func TestSpawnDepthGuardrail(t *testing.T) {
	ctx := context.Background()
	if d := SpawnDepth(ctx); d != 0 {
		t.Fatalf("SpawnDepth(root) = %d, want 0", d)
	}
	if !CanSpawn(ctx, 1) {
		t.Fatal("CanSpawn(depth 0, max 1) = false, want true")
	}

	child := NextSpawnContext(ctx)
	if d := SpawnDepth(child); d != 1 {
		t.Fatalf("SpawnDepth(child) = %d, want 1", d)
	}
	if SpawnDepth(ctx) != 0 {
		t.Fatal("NextSpawnContext mutated the parent context's depth")
	}
	if CanSpawn(child, 1) {
		t.Fatal("CanSpawn(depth 1, max 1) = true, want false")
	}

	grandchild := NextSpawnContext(child)
	if d := SpawnDepth(grandchild); d != 2 {
		t.Fatalf("SpawnDepth(grandchild) = %d, want 2", d)
	}
}

// GIVEN no HooksConfigPath (the shipped default)
// WHEN LoadConfig is called with an empty path
// THEN it returns no configs and no error — the "missing file ⇒ empty"
// convention shared with tools.LoadBlacklist/LoadApprovals.
func TestLoadConfigEmptyPath(t *testing.T) {
	configs, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig(\"\") err = %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("LoadConfig(\"\") = %v, want none", configs)
	}
}

// GIVEN a path that does not exist on disk
// WHEN LoadConfig is called
// THEN it returns no configs and no error (same missing-file convention).
func TestLoadConfigMissingFile(t *testing.T) {
	configs, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("LoadConfig(missing) err = %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("LoadConfig(missing) = %v, want none", configs)
	}
}

// GIVEN a hooks config file naming an unregistered hook
// WHEN Build resolves it against Available
// THEN it fails loudly rather than silently running with fewer hooks than
// configured (a config typo should not fail silently).
func TestBuildUnknownHookErrors(t *testing.T) {
	_, err := Build([]Config{{Name: "does-not-exist"}})
	if err == nil {
		t.Fatal("Build with an unregistered hook name succeeded, want an error")
	}
}

// GIVEN a factory registered in Available under Config.Async = true
// WHEN Build resolves a config that names it but the factory's return value
// only implements SyncHook, not AsyncHook
// THEN Build fails loudly rather than silently registering it as the wrong kind.
func TestBuildAsyncMismatchErrors(t *testing.T) {
	Available["sync-only"] = func(map[string]any) (any, error) {
		return syncFunc(func(context.Context, *Turn) error { return nil }), nil
	}
	defer delete(Available, "sync-only")

	_, err := Build([]Config{{Name: "sync-only", Async: true}})
	if err == nil {
		t.Fatal("Build with a sync-only factory configured async succeeded, want an error")
	}
}

// GIVEN a hooks config file on disk registering one sync and one async hook
// WHEN LoadConfig then Build run against it
// THEN the resulting Registry actually invokes both.
func TestLoadConfigAndBuildEndToEnd(t *testing.T) {
	var syncRan bool
	asyncRan := make(chan struct{})
	Available["e2e-sync"] = func(map[string]any) (any, error) {
		return syncFunc(func(context.Context, *Turn) error { syncRan = true; return nil }), nil
	}
	Available["e2e-async"] = func(map[string]any) (any, error) {
		return asyncFunc(func(context.Context, Turn) { close(asyncRan) }), nil
	}
	defer delete(Available, "e2e-sync")
	defer delete(Available, "e2e-async")

	path := filepath.Join(t.TempDir(), "hooks.toml")
	content := "[[hook]]\nname = \"e2e-sync\"\n\n[[hook]]\nname = \"e2e-async\"\nasync = true\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	configs, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	reg, err := Build(configs)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if err := reg.RunSync(context.Background(), &Turn{}); err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	reg.RunAsync(context.Background(), Turn{})
	<-asyncRan // the configured async hook itself closes this — no separate sentinel to race against it

	if !syncRan {
		t.Error("configured sync hook never ran")
	}
}

// syncFunc adapts a function to SyncHook, mirroring tools.ApproverFunc's pattern.
type syncFunc func(ctx context.Context, turn *Turn) error

func (f syncFunc) Run(ctx context.Context, turn *Turn) error { return f(ctx, turn) }

// asyncFunc adapts a function to AsyncHook.
type asyncFunc func(ctx context.Context, turn Turn)

func (f asyncFunc) Run(ctx context.Context, turn Turn) { f(ctx, turn) }
