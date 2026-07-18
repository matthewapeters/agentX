package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"

	"agentx/internal/config"
)

type classifyConfigWorld struct {
	dir string
	cfg config.Config
}

// registerClassificationConfigSteps wires the classify/output config steps (CHT-D2).
func registerClassificationConfigSteps(sc *godog.ScenarioContext) {
	w := &classifyConfigWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = classifyConfigWorld{}
		return ctx, nil
	})

	sc.Step(`^a freshly resolved configuration$`, w.freshConfig)
	sc.Step(`^a deployment config with classification retries (\d+) and max widget lines (\d+)$`, w.deploymentWithTunables)
	sc.Step(`^the runtime resolves classification configuration$`, w.resolve)
	sc.Step(`^the effective classification retries is (\d+)$`, w.retriesIs)
	sc.Step(`^the effective clarification options is (\d+)$`, w.clarificationIs)
	sc.Step(`^the effective max widget lines is (\d+)$`, w.maxLinesIs)
}

func (w *classifyConfigWorld) tempPaths() (config.Paths, error) {
	dir, err := os.MkdirTemp("", "agentx-clscfg-")
	if err != nil {
		return config.Paths{}, err
	}
	w.dir = dir
	return config.Paths{
		Deployment: filepath.Join(dir, "deploy", "agentx.toml"),
		Project:    filepath.Join(dir, "project", ".agentx", ".agentx.toml"),
	}, nil
}

func (w *classifyConfigWorld) freshConfig() error {
	paths, err := w.tempPaths()
	if err != nil {
		return err
	}
	w.cfg, _, err = config.Resolve(paths)
	return err
}

func (w *classifyConfigWorld) deploymentWithTunables(retries, maxLines int) error {
	paths, err := w.tempPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.Deployment), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("[agentx.classification]\nretries = %d\n\n[agentx.output]\nmax_widget_lines = %d\n", retries, maxLines)
	if err := os.WriteFile(paths.Deployment, []byte(body), 0o644); err != nil {
		return err
	}
	w.cfg, _, err = config.Resolve(paths)
	return err
}

func (w *classifyConfigWorld) resolve() error { return nil } // resolution happened in the Given

func (w *classifyConfigWorld) retriesIs(want int) error {
	if got := w.cfg.ClassificationRetries(); got != want {
		return fmt.Errorf("classification retries = %d, want %d", got, want)
	}
	return nil
}

func (w *classifyConfigWorld) clarificationIs(want int) error {
	if got := w.cfg.ClarificationOptions(); got != want {
		return fmt.Errorf("clarification options = %d, want %d", got, want)
	}
	return nil
}

func (w *classifyConfigWorld) maxLinesIs(want int) error {
	if got := w.cfg.MaxWidgetLines(); got != want {
		return fmt.Errorf("max widget lines = %d, want %d", got, want)
	}
	return nil
}
