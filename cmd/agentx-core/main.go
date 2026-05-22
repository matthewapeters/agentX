// Package main provides the AgentX Go core orchestrator.
// It manages tmux session/pane lifecycle, supervises Python applets, routes IPC, and exposes a health endpoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var (
		projectDir = flag.String("project-dir", ".", "Project directory for sessions and config")
		username   = flag.String("user", os.Getenv("USER"), "Username for session isolation")
		sessionID  = flag.String("session-id", "", "Session ID; auto-generated if empty")
		attach     = flag.Bool("attach", true, "Attach to tmux session after startup (use -attach=false for headless mode)")
		demo       = flag.Bool("demo", false, "Run DemoMode scaffolding and exit")
		demoStart  = flag.String("demo-start", "", "Demo start selector (test id or 1-based index). Requires -demo")
	)
	flag.Parse()

	if *projectDir == "" {
		*projectDir = "."
	}
	if *username == "" {
		*username = "agentx"
	}

	if *demoStart != "" && !*demo {
		log.Fatalf("--demo-start requires --demo")
	}

	if *demo {
		if err := runDemoMode(os.Stdin, os.Stdout, *demoStart, nil); err != nil {
			log.Fatalf("Failed to initialize demo mode: %v", err)
		}
		return
	}

	// Create root context with cancellation for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for SIGINT/SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\n[AgentX Core] Received signal: %v\n", sig)
		cancel()
	}()

	// Initialize core components.
	cfg := &Config{
		ProjectDir: *projectDir,
		Username:   *username,
		SessionID:  *sessionID,
	}

	core := NewAgentXCore(cfg)

	// Initialize tmux session and panes.
	if err := core.InitializeTmuxSession(ctx); err != nil {
		log.Fatalf("Failed to initialize tmux session: %v", err)
	}
	fmt.Println("[AgentX Core] ✓ tmux session initialized")

	// Start applet supervisor.
	if err := core.StartAppletSupervisor(ctx); err != nil {
		log.Fatalf("Failed to start applet supervisor: %v", err)
	}
	fmt.Println("[AgentX Core] ✓ Applet supervisor started")

	// Start health/status endpoint.
	if err := core.StartHealthEndpoint(ctx); err != nil {
		log.Fatalf("Failed to start health endpoint: %v", err)
	}
	fmt.Println("[AgentX Core] ✓ Health endpoint started")

	if *attach {
		fmt.Printf("[AgentX Core] Attaching to tmux session '%s'...\n", core.tmuxSessionName)
		if err := core.AttachTmuxSession(ctx); err != nil {
			log.Fatalf("Failed to attach tmux session: %v", err)
		}
		fmt.Println("[AgentX Core] tmux client detached; core still running")
	}

	// Wait for context cancellation (via signal or graceful shutdown).
	<-ctx.Done()

	// Graceful shutdown.
	fmt.Println("\n[AgentX Core] Shutting down gracefully...")
	if err := core.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
	fmt.Println("[AgentX Core] ✓ Shutdown complete")
}
