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
		demo       = flag.Bool("demo", false, "Run DemoMode interactive harness")
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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigChan
			fmt.Printf("\n[AgentX Demo] Received signal: %v\n", sig)
			cancel()
		}()

		cfg := &Config{
			ProjectDir: *projectDir,
			Username:   *username,
			SessionID:  *sessionID,
		}

		core := NewAgentXCore(cfg)
		if err := core.InitializeTmuxSession(ctx); err != nil {
			log.Fatalf("Failed to initialize demo tmux session: %v", err)
		}
		fmt.Printf("[AgentX Demo] Live TUI session initialized: %s\n", core.tmuxSessionName)
		fmt.Printf("[AgentX Demo] Attach in another terminal with: tmux attach -t %s\n", core.tmuxSessionName)

		if err := core.StartAppletSupervisor(ctx); err != nil {
			log.Fatalf("Failed to start demo applet supervisor: %v", err)
		}
		if err := core.StartHealthEndpoint(ctx); err != nil {
			log.Fatalf("Failed to start demo health endpoint: %v", err)
		}

		demoRunner := func(testCase DemoTestCase) (string, error) {
			inputLine := demoPromptForTestCase(testCase)
			response, _, err := core.HandleInputLine(ctx, inputLine)
			if err != nil {
				return "", err
			}
			if response == "" {
				return "ok", nil
			}
			return response, nil
		}

		demoConfig := DemoRuntimeConfig{
			ProjectDir:      *projectDir,
			Username:        *username,
			SessionID:       *sessionID,
			TmuxSessionName: core.tmuxSessionName,
		}
		demoErr := runDemoModeWithConfig(os.Stdin, os.Stdout, *demoStart, demoRunner, demoConfig)

		shutdownErr := core.Shutdown(ctx)
		if demoErr != nil {
			log.Fatalf("Failed to complete demo mode: %v", demoErr)
		}
		if shutdownErr != nil {
			log.Printf("Shutdown error: %v", shutdownErr)
		}
		fmt.Println("[AgentX Demo] ✓ Demo session complete")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\n[AgentX Core] Received signal: %v\n", sig)
		cancel()
	}()

	cfg := &Config{
		ProjectDir: *projectDir,
		Username:   *username,
		SessionID:  *sessionID,
	}

	core, err := startAgentXCore(ctx, cfg, *attach)
	if err != nil {
		log.Fatalf("Failed to start AgentX core: %v", err)
	}

	<-ctx.Done()

	fmt.Println("\n[AgentX Core] Shutting down gracefully...")
	if err := core.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
	fmt.Println("[AgentX Core] ✓ Shutdown complete")
}

func startAgentXCore(ctx context.Context, cfg *Config, attach bool) (*AgentXCore, error) {
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(ctx); err != nil {
		return nil, err
	}
	fmt.Println("[AgentX Core] ✓ tmux session initialized")

	if err := core.StartAppletSupervisor(ctx); err != nil {
		return nil, err
	}
	fmt.Println("[AgentX Core] ✓ Applet supervisor started")

	if err := core.StartHealthEndpoint(ctx); err != nil {
		return nil, err
	}
	fmt.Println("[AgentX Core] ✓ Health endpoint started")

	if attach {
		fmt.Printf("[AgentX Core] Attaching to tmux session '%s'...\n", core.tmuxSessionName)
		if err := core.AttachTmuxSession(ctx); err != nil {
			return nil, err
		}
		fmt.Println("[AgentX Core] tmux client detached; core still running")
	}

	return core, nil
}
