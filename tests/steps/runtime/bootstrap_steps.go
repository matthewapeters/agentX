// Package runtimesteps holds Godog step definitions for the runtime behavior
// domain. Step definitions live in importable (non-_test) files so suite
// runners under tests/suites can wire them, per the layout standard in
// docs/implementation/08_go_module_layout.md.
package runtimesteps

import (
	"fmt"

	"github.com/cucumber/godog"

	"agentx/internal/config"
)

// harnessWorld carries per-scenario state for the harness smoke domain.
type harnessWorld struct {
	wired   bool
	ran     bool
	succeed bool
}

// InitializeScenario registers the runtime-domain steps onto a Godog scenario
// context. Suite runners pass this to godog.TestSuite.ScenarioInitializer.
func InitializeScenario(sc *godog.ScenarioContext) {
	registerHarnessSteps(sc)
	registerConfigSteps(sc)
	registerStateSteps(sc)
	registerLifecycleSteps(sc)
	registerEntrypointSteps(sc)
	registerOllamaSteps(sc)
	registerPromptSteps(sc)
	registerPromptCycleSteps(sc)
	registerPromptLoopSteps(sc)
	registerPromptCancelSteps(sc)
	registerModelReadinessSteps(sc)
	registerPromptFilesSteps(sc)
	registerClassificationConfigSteps(sc)
	registerClassificationSteps(sc)
	registerClassifyCycleSteps(sc)
	registerApprovalSteps(sc)
	registerToolCycleSteps(sc)
	registerTransportLifecycleSteps(sc)
	registerContextBreakdownSteps(sc)
	registerContextToggleSteps(sc)
	registerToolContextEnableSteps(sc)
	registerWMPinSteps(sc)
	registerTaskClassifierSteps(sc)
	registerTaskBranchSteps(sc)
	registerTaskSchedulerSteps(sc)
	registerDecomposeAdapterSteps(sc)
	registerConfigValidationSteps(sc)
}

// registerHarnessSteps wires the harness smoke steps.
func registerHarnessSteps(sc *godog.ScenarioContext) {
	w := &harnessWorld{}

	sc.Step(`^the Godog harness is wired$`, w.harnessIsWired)
	sc.Step(`^a (?:unit|functional|integration|e2e) scenario runs$`, w.scenarioRuns)
	sc.Step(`^the harness reports success$`, w.reportsSuccess)
}

func (w *harnessWorld) harnessIsWired() error {
	w.wired = true
	return nil
}

func (w *harnessWorld) scenarioRuns() error {
	if !w.wired {
		return fmt.Errorf("harness not wired before scenario run")
	}
	w.ran = true
	w.succeed = true
	return nil
}

func (w *harnessWorld) reportsSuccess() error {
	if !w.ran || !w.succeed {
		return fmt.Errorf("scenario did not run successfully")
	}
	return nil
}

// configValidationWorld carries per-scenario state for config validation tests.
type configValidationWorld struct {
	cfg config.Config
	err error
}

func (w *configValidationWorld) validConfig(provider string) error {
	w.cfg = config.Config{
		Agentx: config.Agentx{
			Provider: provider,
			Ollama: config.Ollama{
				Host:  "localhost:11434",
				Model: "phi4-mini:3.8b",
			},
		},
	}
	if provider == "llamacpp" {
		w.cfg.Agentx.Llamacpp = config.Llamacpp{
			Host:  "localhost:8080",
			Model: "phi4:latest",
		}
	}
	return nil
}

func (w *configValidationWorld) invalidProvider(provider string) error {
	w.cfg = config.Config{
		Agentx: config.Agentx{
			Provider: provider,
			Ollama: config.Ollama{
				Host:  "localhost:11434",
				Model: "phi4-mini:3.8b",
			},
		},
	}
	return nil
}

func (w *configValidationWorld) ollamaNoModel() error {
	w.cfg = config.Config{
		Agentx: config.Agentx{
			Provider: "ollama",
			Ollama: config.Ollama{
				Host:  "localhost:11434",
				Model: "",
			},
		},
	}
	return nil
}

func (w *configValidationWorld) llamacppNoHost() error {
	w.cfg = config.Config{
		Agentx: config.Agentx{
			Provider: "llamacpp",
			Llamacpp: config.Llamacpp{
				Host:  "",
				Model: "phi4:latest",
			},
		},
	}
	return nil
}

func (w *configValidationWorld) llamacppNoModel() error {
	w.cfg = config.Config{
		Agentx: config.Agentx{
			Provider: "llamacpp",
			Llamacpp: config.Llamacpp{
				Host:  "localhost:8080",
				Model: "",
			},
		},
	}
	return nil
}

func (w *configValidationWorld) validationPasses() error {
	w.err = w.cfg.Validate()
	if w.err != nil {
		return fmt.Errorf("expected validation to pass, got error: %w", w.err)
	}
	return nil
}

func (w *configValidationWorld) validationFails() error {
	w.err = w.cfg.Validate()
	if w.err == nil {
		return fmt.Errorf("expected validation error, got nil")
	}
	return nil
}

// registerConfigValidationSteps wires the config validation steps.
func registerConfigValidationSteps(sc *godog.ScenarioContext) {
	w := &configValidationWorld{}
	sc.Step(`^a valid config with provider "([^"]*)"$`, w.validConfig)
	sc.Step(`^a config with invalid provider "([^"]*)"$`, w.invalidProvider)
	sc.Step(`^a config with provider "ollama" and no model$`, w.ollamaNoModel)
	sc.Step(`^a config with provider "llamacpp" and no host$`, w.llamacppNoHost)
	sc.Step(`^a config with provider "llamacpp" and no model$`, w.llamacppNoModel)
	sc.Step(`^config validation passes$`, w.validationPasses)
	sc.Step(`^config validation fails$`, w.validationFails)
}
