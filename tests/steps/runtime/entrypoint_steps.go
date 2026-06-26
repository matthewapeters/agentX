package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/app"
	"agentx/internal/cli"
	"agentx/internal/config"
	"agentx/internal/runtime"
)

type entrypointWorld struct {
	dir      string
	opts     app.Options
	cmd      cli.Command
	parseErr error
	orc      *runtime.Orchestrator
	runErr   error
}

// registerEntrypointSteps wires the composition/entrypoint steps (CHT-A6).
func registerEntrypointSteps(sc *godog.ScenarioContext) {
	w := &entrypointWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = entrypointWorld{}
		return ctx, err
	})

	sc.Step(`^the arguments "([^"]*)" are parsed$`, w.parseArgs)
	sc.Step(`^the parsed command requests version output$`, w.requestsVersion)
	sc.Step(`^app options with a temp config selecting model "([^"]*)"$`, w.appOptions)
	sc.Step(`^the app builds the runtime$`, w.build)
	sc.Step(`^the built orchestrator has an active session$`, w.hasSession)
	sc.Step(`^the orchestrator active model is "([^"]*)"$`, w.activeModelIs)
	sc.Step(`^the app runs until the context is canceled$`, w.runUntilCanceled)
	sc.Step(`^the run returns without error$`, w.runNoError)
}

func (w *entrypointWorld) parseArgs(args string) error {
	w.cmd, w.parseErr = cli.Parse(strings.Fields(args))
	return w.parseErr
}

func (w *entrypointWorld) requestsVersion() error {
	if !w.cmd.ShowVersion {
		return fmt.Errorf("expected ShowVersion to be true")
	}
	return nil
}

func (w *entrypointWorld) appOptions(model string) error {
	dir, err := os.MkdirTemp("", "agentx-app-")
	if err != nil {
		return err
	}
	w.dir = dir
	deployment := filepath.Join(dir, "cfg", "agentx.toml")
	if err := os.MkdirAll(filepath.Dir(deployment), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(deployment, []byte(fmt.Sprintf("ollama_model = %q\n", model)), 0o644); err != nil {
		return err
	}
	w.opts = app.Options{
		Paths: &config.Paths{
			Deployment: deployment,
			Project:    filepath.Join(dir, "proj", ".agentx", ".agentx.toml"),
		},
		SessionRoot: filepath.Join(dir, "sessions"),
	}
	return nil
}

func (w *entrypointWorld) build() error {
	orc, err := app.Build(w.opts)
	if err != nil {
		return err
	}
	w.orc = orc
	return nil
}

func (w *entrypointWorld) hasSession() error {
	if w.orc.Session().ID == "" {
		return fmt.Errorf("orchestrator has no active session")
	}
	return nil
}

func (w *entrypointWorld) activeModelIs(want string) error {
	if got := w.orc.Settings().OllamaModel; got != want {
		return fmt.Errorf("active model = %q, want %q", got, want)
	}
	return nil
}

func (w *entrypointWorld) runUntilCanceled() error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // request shutdown immediately; Run builds, serves, then drains.
	w.runErr = app.Run(ctx, w.opts)
	return nil
}

func (w *entrypointWorld) runNoError() error {
	return w.runErr
}
