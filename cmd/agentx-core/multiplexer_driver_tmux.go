package main

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

// TmuxMultiplexerDriver is the default Phase 1 multiplexer backend implementation.
type TmuxMultiplexerDriver struct{}

func NewTmuxMultiplexerDriver() *TmuxMultiplexerDriver {
	return &TmuxMultiplexerDriver{}
}

func (d *TmuxMultiplexerDriver) BackendName() string {
	return defaultMultiplexerBackend
}

func (d *TmuxMultiplexerDriver) Run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func (d *TmuxMultiplexerDriver) RunCombined(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func (d *TmuxMultiplexerDriver) Capture(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (d *TmuxMultiplexerDriver) AttachSession(ctx context.Context, sessionName string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "tmux", "attach-session", "-t", sessionName)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
