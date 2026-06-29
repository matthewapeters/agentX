// Package runtimesteps holds Godog step definitions for the runtime behavior
// domain. Step definitions live in importable (non-_test) files so suite
// runners under tests/suites can wire them, per the layout standard in
// docs/implementation/08_go_module_layout.md.
package runtimesteps

import (
	"fmt"

	"github.com/cucumber/godog"
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
	registerPromptCancelSteps(sc)
	registerModelReadinessSteps(sc)
	registerPromptFilesSteps(sc)
	registerClassificationConfigSteps(sc)
	registerClassificationSteps(sc)
	registerClassifyCycleSteps(sc)
	registerApprovalSteps(sc)
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
