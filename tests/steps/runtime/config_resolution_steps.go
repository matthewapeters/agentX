package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/cucumber/godog"

	"agentx/internal/config"
)

// configWorld carries per-scenario state for the config-resolution domain
// (CHT-A1). Paths point inside a per-scenario temp dir so tests never touch the
// real user config.
type configWorld struct {
	dir    string
	paths  config.Paths
	cfg    config.Config
	source config.Source
}

// registerConfigSteps wires the config-resolution steps onto the scenario context.
func registerConfigSteps(sc *godog.ScenarioContext) {
	w := &configWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		dir, err := os.MkdirTemp("", "agentx-cfg-")
		if err != nil {
			return ctx, err
		}
		w.dir = dir
		w.paths = config.Paths{
			Deployment: filepath.Join(dir, "deploy", "agentx.toml"),
			Project:    filepath.Join(dir, "project", ".agentx", ".agentx.toml"),
		}
		w.cfg = config.Config{}
		w.source = ""
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		return ctx, err
	})

	sc.Step(`^a deployment config with ollama_model "([^"]*)"$`, w.aDeploymentConfigWithModel)
	sc.Step(`^a deployment config with markdown_renderer "([^"]*)"$`, w.aDeploymentConfigWithRenderer)
	sc.Step(`^the effective markdown renderer is "([^"]*)"$`, w.effectiveRendererIs)
	sc.Step(`^no deployment config exists$`, w.noDeploymentConfig)
	sc.Step(`^no project config exists$`, w.noProjectConfig)
	sc.Step(`^a project config with ollama_model "([^"]*)"$`, w.aProjectConfigWithModel)
	sc.Step(`^the runtime resolves configuration$`, w.resolve)
	sc.Step(`^the effective ollama_model is "([^"]*)"$`, w.effectiveModelIs)
	sc.Step(`^the effective ollama_model is the built-in default$`, w.effectiveModelIsDefault)
	sc.Step(`^the config source is "([^"]*)"$`, w.sourceIs)
	sc.Step(`^a deployment config file is created$`, w.deploymentFileCreated)
	sc.Step(`^the seeded deployment config contains ollama_model "([^"]*)"$`, w.seededDeploymentContainsModel)
}

func (w *configWorld) aDeploymentConfigWithModel(model string) error {
	return writeModelConfig(w.paths.Deployment, model)
}

func (w *configWorld) aProjectConfigWithModel(model string) error {
	return writeModelConfig(w.paths.Project, model)
}

func (w *configWorld) aDeploymentConfigWithRenderer(mode string) error {
	if err := os.MkdirAll(filepath.Dir(w.paths.Deployment), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("[agentx.output]\nmarkdown_renderer = %q\n", mode)
	return os.WriteFile(w.paths.Deployment, []byte(body), 0o644)
}

func (w *configWorld) effectiveRendererIs(want string) error {
	if got := w.cfg.MarkdownRenderer(); got != want {
		return fmt.Errorf("effective markdown renderer = %q, want %q", got, want)
	}
	return nil
}

func (w *configWorld) noDeploymentConfig() error {
	if fileExists(w.paths.Deployment) {
		return fmt.Errorf("expected no deployment config at %s", w.paths.Deployment)
	}
	return nil
}

func (w *configWorld) noProjectConfig() error {
	if fileExists(w.paths.Project) {
		return fmt.Errorf("expected no project config at %s", w.paths.Project)
	}
	return nil
}

func (w *configWorld) resolve() error {
	cfg, source, err := config.Resolve(w.paths)
	if err != nil {
		return err
	}
	w.cfg = cfg
	w.source = source
	return nil
}

func (w *configWorld) effectiveModelIs(want string) error {
	if w.cfg.OllamaModel() != want {
		return fmt.Errorf("effective ollama_model = %q, want %q", w.cfg.OllamaModel(), want)
	}
	return nil
}

func (w *configWorld) effectiveModelIsDefault() error {
	return w.effectiveModelIs(config.Default().OllamaModel())
}

func (w *configWorld) sourceIs(want string) error {
	if string(w.source) != want {
		return fmt.Errorf("config source = %q, want %q", w.source, want)
	}
	return nil
}

func (w *configWorld) deploymentFileCreated() error {
	if !fileExists(w.paths.Deployment) {
		return fmt.Errorf("expected seeded deployment config at %s", w.paths.Deployment)
	}
	return nil
}

func (w *configWorld) seededDeploymentContainsModel(want string) error {
	var got config.Config
	if _, err := toml.DecodeFile(w.paths.Deployment, &got); err != nil {
		return fmt.Errorf("read seeded deployment config: %w", err)
	}
	if got.OllamaModel() != want {
		return fmt.Errorf("seeded ollama_model = %q, want %q", got.OllamaModel(), want)
	}
	return nil
}

// writeModelConfig writes a minimal agentx.toml setting only the active model
// under the [agentx.ollama] table.
func writeModelConfig(path, model string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("[agentx.ollama]\nmodel = %q\n", model)
	return os.WriteFile(path, []byte(body), 0o644)
}

// fileExists reports whether path is an existing regular file. Local helper so the
// steps package does not depend on config internals.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
