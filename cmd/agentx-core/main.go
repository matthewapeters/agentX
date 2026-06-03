// Package main provides the AgentX Go core orchestrator.
// It manages tmux session/pane lifecycle, supervises pane applets, routes IPC, and exposes a health endpoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	var (
		projectDir            = flag.String("project-dir", ".", "Project directory for sessions and config")
		username              = flag.String("user", os.Getenv("USER"), "Username for session isolation")
		sessionID             = flag.String("session-id", "", "Session ID; auto-generated if empty")
		inputWidget           = flag.Bool("input-widget", false, "Run native Go input widget mode over stdin/stdout")
		outputWidget          = flag.Bool("output-widget", false, "Run native Go output widget mode over stdout")
		logsWidget            = flag.Bool("logs-widget", false, "Run native Go logs widget mode over stdout")
		contextWidget         = flag.Bool("context-widget", false, "Run native Go context widget mode over stdout")
		filesystemWidget      = flag.Bool("filesystem-widget", false, "Run native Go filesystem widget mode over stdin/stdout")
		settingsWidget        = flag.Bool("settings-widget", false, "Run native Go system settings widget mode over stdin/stdout")
		coreHTTP              = flag.String("core-http", strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP")), "Core HTTP base URL for widget/bridge modes")
		layout                = flag.String("layout", "", "tmuxp layout file to apply after core windows are created (defaults to .agentx/layouts/default-layout.yaml)")
		layoutFile            = flag.String("layout-file", "", "Legacy alias for --layout")
		layoutTemplate        = flag.String("layout-template", "", "Write a starter tmuxp layout template to this file and exit")
		dumpDefaultLayoutPath = flag.String("dump-default-layout", "", "Write the built-in default tmuxp layout to a file path, or '-' for stdout")
		startupMode           = flag.String("startup-mode", resolveStartupModeDefault(), "Startup topology mode: default|visible-windows")
		attach                = flag.Bool("attach", true, "Attach to tmux session after startup (use -attach=false for headless mode)")
		demo                  = flag.Bool("demo", false, "Run DemoMode with a split tmux controller and live core session")
		demoDefault           = flag.Bool("default", false, "Use default frame-based startup topology (works for normal and demo startup)")
		demoWindowed          = flag.Bool("windowed", false, "Use windowed startup topology (works for normal and demo startup)")
		demoHeadless          = flag.Bool("demo-headless", false, "Run DemoMode without the split-pane controller (internal)")
		demoController        = flag.Bool("demo-controller", false, "Run the split-pane DemoMode controller pane (internal)")
		demoSplit             = flag.Bool("demo-split", false, "Enable split-view controller behavior (internal)")
		demoStoriesFile       = flag.String("demo-stories-file", "", "Stories board file path for split-view demo mode (internal)")
		demoStart             = flag.String("demo-start", "", "Demo start selector (test id or 1-based index). Requires a demo mode flag")
		demoCoreSession       = flag.String("demo-core-session", "", "Live core tmux session used by the DemoMode controller (internal)")
		healthAddr            = flag.String("health-addr", "", "Health endpoint address override for internal controller/runtime wiring")
	)
	flag.Parse()

	if *projectDir == "" {
		*projectDir = "."
	}
	resolvedStartupMode, ok := normalizeStartupMode(*startupMode)
	if !ok {
		log.Fatalf("invalid --startup-mode value %q (expected default or visible-windows)", strings.TrimSpace(*startupMode))
	}
	resolvedStartupMode, err := resolveStartupModeSelection(*demoDefault, *demoWindowed, resolvedStartupMode)
	if err != nil {
		log.Fatalf("%v", err)
	}

	demoStartupMode, err := resolveDemoStartupMode(*demoDefault, *demoWindowed, resolvedStartupMode)
	if err != nil {
		log.Fatalf("%v", err)
	}
	if *username == "" {
		*username = "agentx"
	}

	if *demoStart != "" && !(*demo || *demoHeadless || *demoController) {
		log.Fatalf("--demo-start requires a demo mode flag")
	}

	if *inputWidget {
		exitCode := runInputWidgetCommand(strings.TrimSpace(*coreHTTP), os.Stdin, os.Stdout)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}

	if *outputWidget {
		exitCode := runOutputWidgetCommand(strings.TrimSpace(*coreHTTP), os.Stdin, os.Stdout)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}

	if *logsWidget {
		exitCode := runLogsWidgetCommand(strings.TrimSpace(*coreHTTP), os.Stdout)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}

	if *contextWidget {
		exitCode := runContextWidgetCommandWithInput(strings.TrimSpace(*coreHTTP), os.Stdin, os.Stdout)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}

	if *filesystemWidget {
		exitCode := runFilesystemWidgetCommand(strings.TrimSpace(*coreHTTP), os.Stdin, os.Stdout)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}

	if *settingsWidget {
		exitCode := runSettingsWidgetCommand(strings.TrimSpace(*coreHTTP), os.Stdin, os.Stdout)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}

	if strings.TrimSpace(*layoutTemplate) != "" && strings.TrimSpace(*dumpDefaultLayoutPath) != "" {
		log.Fatalf("--layout-template and --dump-default-layout cannot be used together")
	}

	if strings.TrimSpace(*dumpDefaultLayoutPath) != "" {
		if err := dumpDefaultLayout(strings.TrimSpace(*dumpDefaultLayoutPath), os.Stdout); err != nil {
			log.Fatalf("Failed to dump default layout: %v", err)
		}
		if strings.TrimSpace(*dumpDefaultLayoutPath) != "-" {
			fmt.Printf("[AgentX Core] Wrote default tmuxp layout: %s\n", strings.TrimSpace(*dumpDefaultLayoutPath))
		}
		return
	}

	if strings.TrimSpace(*layoutTemplate) != "" {
		if err := writeTmuxpLayoutTemplate(strings.TrimSpace(*layoutTemplate)); err != nil {
			log.Fatalf("Failed to write layout template: %v", err)
		}
		fmt.Printf("[AgentX Core] Wrote tmuxp layout template: %s\n", strings.TrimSpace(*layoutTemplate))
		return
	}

	resolvedLayoutFile, err := resolveLayoutFileSelection(strings.TrimSpace(*layout), strings.TrimSpace(*layoutFile))
	if err != nil {
		log.Fatalf("%v", err)
	}
	if resolvedLayoutFile == "" {
		resolvedLayoutFile, err = ensureDefaultLayoutFile(*projectDir)
		if err != nil {
			log.Fatalf("Failed to materialize default layout file: %v", err)
		}
	}

	if err := validateRuntimePrerequisites(); err != nil {
		log.Fatalf("Missing runtime prerequisite: %v", err)
	}

	if *demoHeadless {
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
			ProjectDir:  *projectDir,
			Username:    *username,
			SessionID:   *sessionID,
			LayoutFile:  resolvedLayoutFile,
			StartupMode: demoStartupMode,
		}

		core := NewAgentXCore(cfg)
		core.SetShutdownProvider(cancel)
		if err := core.InitializeTmuxSession(ctx); err != nil {
			log.Fatalf("Failed to initialize demo tmux session: %v", err)
		}
		if err := core.PrepareHealthEndpoint(); err != nil {
			log.Fatalf("Failed to prepare demo health endpoint: %v", err)
		}
		fmt.Printf("[AgentX Demo] Live TUI session initialized: %s\n", core.tmuxSessionName)
		fmt.Printf("[AgentX Demo] Attach in another terminal with: tmux attach -t %s\n", core.tmuxSessionName)

		if err := core.StartAppletSupervisor(ctx); err != nil {
			log.Fatalf("Failed to start demo applet supervisor: %v", err)
		}
		if err := core.StartHealthEndpoint(ctx); err != nil {
			log.Fatalf("Failed to start demo health endpoint: %v", err)
		}

		demoConfig := DemoRuntimeConfig{
			ProjectDir:      *projectDir,
			Username:        *username,
			SessionID:       core.SessionID,
			TmuxSessionName: core.tmuxSessionName,
			HealthAddr:      core.healthAddr,
			StartupMode:     demoStartupMode,
		}

		demoRunner := func(testCase DemoTestCase) (string, error) {
			return runDemoUXUseCase(ctx, demoConfig, testCase)
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

	if *demoController {
		if strings.TrimSpace(*demoCoreSession) == "" {
			log.Fatalf("--demo-controller requires --demo-core-session")
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigChan
			fmt.Printf("\n[AgentX Demo] Received signal: %v\n", sig)
			cancel()
		}()

		runtimeConfig := DemoRuntimeConfig{
			ProjectDir:      *projectDir,
			Username:        *username,
			SessionID:       *sessionID,
			TmuxSessionName: *demoCoreSession,
			HealthAddr:      strings.TrimSpace(*healthAddr),
			StartupMode:     demoStartupMode,
			SplitView:       *demoSplit,
			StoriesFilePath: strings.TrimSpace(*demoStoriesFile),
		}
		if err := waitForDemoHealthEndpoint(ctx, runtimeConfig.HealthURL()); err != nil {
			log.Fatalf("Demo controller could not reach live core: %v", err)
		}

		demoRunner := func(testCase DemoTestCase) (string, error) {
			response, err := runDemoUXUseCase(ctx, runtimeConfig, testCase)
			if err != nil {
				return "", err
			}

			if strings.TrimSpace(testCase.ID) == "e2e-003" && strings.EqualFold(strings.TrimSpace(response), "quit") {
				if killErr := runTmux(context.Background(), "kill-session", "-t", runtimeConfig.TmuxSessionName); killErr != nil && !isTmuxMissingSessionError(killErr) {
					return "", fmt.Errorf("failed to close live core session after final shutdown test: %w", killErr)
				}
			}

			return response, nil
		}
		if err := runDemoModeWithConfigAndContext(ctx, os.Stdin, os.Stdout, *demoStart, demoRunner, runtimeConfig); err != nil {
			if closeErr := closeCurrentTmuxSession(context.Background()); closeErr != nil {
				log.Printf("[AgentX Demo] Warning: failed to close demo session from controller error path: %v", closeErr)
			}
			if errors.Is(err, context.Canceled) {
				fmt.Println("[AgentX Demo] Controller interrupted; demo session closed")
				return
			}
			log.Fatalf("Demo controller failed: %v", err)
		}
		if err := closeCurrentTmuxSession(ctx); err != nil {
			log.Printf("[AgentX Demo] Warning: failed to close demo session from controller: %v", err)
		}
		fmt.Println("[AgentX Demo] ✓ Controller complete")
		return
	}

	if *demo {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Suppress core runtime log interleaving while tmux split demo is attached.
		// Demo UX output is rendered by the controller pane and explicit demo prints.
		originalLogWriter := log.Writer()
		log.SetOutput(io.Discard)
		defer log.SetOutput(originalLogWriter)

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigChan
			fmt.Printf("\n[AgentX Demo] Received signal: %v\n", sig)
			cancel()
		}()

		cfg := &Config{
			ProjectDir:  *projectDir,
			Username:    *username,
			SessionID:   *sessionID,
			LayoutFile:  resolvedLayoutFile,
			StartupMode: demoStartupMode,
		}

		core, err := startAgentXCore(ctx, cancel, cfg, false)
		if err != nil {
			log.Fatalf("Failed to start live demo core: %v", err)
		}

		demoErr := runDemoSplitMode(ctx, cfg, core, *demoStart)
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
		ProjectDir:  *projectDir,
		Username:    *username,
		SessionID:   *sessionID,
		LayoutFile:  resolvedLayoutFile,
		StartupMode: resolvedStartupMode,
	}

	core, err := startAgentXCore(ctx, cancel, cfg, *attach)
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

func validateRuntimePrerequisites() error {
	for _, binary := range []string{"tmux", "tmuxp"} {
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Errorf("%s not found in PATH", binary)
		}
	}

	if output, err := exec.Command("tmux", "-V").CombinedOutput(); err != nil {
		return fmt.Errorf("tmux probe failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("tmuxp", "--version").CombinedOutput(); err != nil {
		return fmt.Errorf("tmuxp probe failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func resolveLayoutFileSelection(layout string, legacyLayoutFile string) (string, error) {
	if layout == "" && legacyLayoutFile == "" {
		return "", nil
	}
	if layout != "" && legacyLayoutFile != "" && layout != legacyLayoutFile {
		return "", fmt.Errorf("--layout and --layout-file conflict; provide only one")
	}
	if layout != "" {
		return layout, nil
	}
	return legacyLayoutFile, nil
}

func resolveDemoStartupMode(demoDefault, demoWindowed bool, fallback string) (string, error) {
	return resolveStartupModeSelection(demoDefault, demoWindowed, fallback)
}

func resolveStartupModeSelection(defaultFlag, windowedFlag bool, fallback string) (string, error) {
	if defaultFlag && windowedFlag {
		return "", fmt.Errorf("--default and --windowed cannot be used together")
	}
	if windowedFlag {
		return visibleWindowsStartupMode, nil
	}
	if defaultFlag {
		return defaultStartupMode, nil
	}
	if mode, ok := normalizeStartupMode(fallback); ok {
		return mode, nil
	}
	return defaultStartupMode, nil
}

func startAgentXCore(ctx context.Context, cancel context.CancelFunc, cfg *Config, attach bool) (*AgentXCore, error) {
	core := NewAgentXCore(cfg)
	core.SetShutdownProvider(cancel)

	if err := core.InitializeTmuxSession(ctx); err != nil {
		return nil, err
	}
	fmt.Println("[AgentX Core] ✓ tmux session initialized")

	if err := core.PrepareHealthEndpoint(); err != nil {
		return nil, err
	}

	if err := core.StartAppletSupervisor(ctx); err != nil {
		return nil, err
	}
	fmt.Println("[AgentX Core] ✓ Applet supervisor started")

	if err := core.RunStartupBootstrap(ctx); err != nil {
		log.Printf("[AgentX Core] Startup bootstrap warning: %v", err)
	}

	if err := core.FocusInputPane(ctx); err != nil {
		return nil, err
	}

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
