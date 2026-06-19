package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppletPortAllocator_SequentialAllocation(t *testing.T) {
	alloc, err := NewAppletPortAllocator(19100, 19109)
	if err != nil {
		t.Fatalf("unexpected error creating allocator: %v", err)
	}
	for want := 19100; want <= 19109; want++ {
		got, err := alloc.Next()
		if err != nil {
			t.Fatalf("unexpected error at port %d: %v", want, err)
		}
		if got != want {
			t.Fatalf("expected port %d, got %d", want, got)
		}
	}
}

func TestAppletPortAllocator_ExhaustsRange(t *testing.T) {
	alloc, err := NewAppletPortAllocator(19200, 19201)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := alloc.Next(); err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if _, err := alloc.Next(); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if _, err := alloc.Next(); err == nil {
		t.Fatal("expected error after range exhausted, got nil")
	}
}

func TestAppletPortAllocator_Remaining(t *testing.T) {
	alloc, err := NewAppletPortAllocator(19300, 19304)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := alloc.Remaining(); got != 5 {
		t.Fatalf("expected 5 remaining, got %d", got)
	}
	_, _ = alloc.Next()
	if got := alloc.Remaining(); got != 4 {
		t.Fatalf("expected 4 remaining after one allocation, got %d", got)
	}
}

func TestAppletPortAllocator_InvalidRange(t *testing.T) {
	if _, err := NewAppletPortAllocator(0, 100); err == nil {
		t.Fatal("expected error for start=0")
	}
	if _, err := NewAppletPortAllocator(200, 100); err == nil {
		t.Fatal("expected error for end < start")
	}
}

func TestDefaultCoreRuntimeConfig_HasPortRange(t *testing.T) {
	cfg := defaultCoreRuntimeConfig()
	if cfg.AppletPortRangeStart != defaultAppletPortRangeStart {
		t.Fatalf("expected AppletPortRangeStart=%d, got %d", defaultAppletPortRangeStart, cfg.AppletPortRangeStart)
	}
	if cfg.AppletPortRangeEnd != defaultAppletPortRangeEnd {
		t.Fatalf("expected AppletPortRangeEnd=%d, got %d", defaultAppletPortRangeEnd, cfg.AppletPortRangeEnd)
	}
}

func TestApplyAgentXTomlRuntimeConfig_ParsesPortRange(t *testing.T) {
	dir := t.TempDir()
	toml := "[agentx]\napplet_api_port_range_start = 15000\napplet_api_port_range_end = 15099\n"
	if err := os.WriteFile(filepath.Join(dir, "agentx.toml"), []byte(toml), 0o644); err != nil {
		t.Fatalf("failed to write test toml: %v", err)
	}
	cfg := defaultCoreRuntimeConfig()
	applyAgentXTomlRuntimeConfig(dir, &cfg)
	if cfg.AppletPortRangeStart != 15000 {
		t.Fatalf("expected AppletPortRangeStart=15000, got %d", cfg.AppletPortRangeStart)
	}
	if cfg.AppletPortRangeEnd != 15099 {
		t.Fatalf("expected AppletPortRangeEnd=15099, got %d", cfg.AppletPortRangeEnd)
	}
}

