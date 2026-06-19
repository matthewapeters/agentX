package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// MultiplexerDriver is the minimal Phase 1 abstraction seam for core tmux interactions.
type MultiplexerDriver interface {
	BackendName() string
	Run(ctx context.Context, args ...string) error
	RunCombined(ctx context.Context, args ...string) (string, error)
	Capture(ctx context.Context, args ...string) (string, error)
	AttachSession(ctx context.Context, sessionName string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error
}

func resolveMultiplexerBackend(projectDir string) string {
	runtimeConfig := resolveCoreRuntimeConfig(projectDir)
	backend := strings.ToLower(strings.TrimSpace(runtimeConfig.MultiplexerBackend))
	if backend == "" {
		return defaultMultiplexerBackend
	}
	return backend
}

func newMultiplexerDriverFromConfig(projectDir string) (MultiplexerDriver, error) {
	backend := resolveMultiplexerBackend(projectDir)
	switch backend {
	case "", defaultMultiplexerBackend:
		return NewTmuxMultiplexerDriver(), nil
	case "zellij":
		return NewZellijMultiplexerDriver(projectDir), nil
	default:
		return nil, fmt.Errorf("unsupported multiplexer backend: %s", backend)
	}
}

func runtimeMultiplexerDriver(projectDir string) (MultiplexerDriver, error) {
	return newMultiplexerDriverFromConfig(projectDir)
}
