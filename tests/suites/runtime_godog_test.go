// Package suites holds Godog suite runners. Each runner is tag-scoped so the
// Makefile targets (go-test-unit/integration/functional/e2e) can select a slice
// of the behavior corpus. See docs/implementation/07_test_and_documentation_contract.md
// for the required tag scheme and docs/implementation/08_go_module_layout.md for
// the suite layout.
package suites

import (
	"testing"

	"github.com/cucumber/godog"

	executorsteps "agentx/tests/steps/executor"
	llmsteps "agentx/tests/steps/llm"
	promptingsteps "agentx/tests/steps/prompting"
	runtimesteps "agentx/tests/steps/runtime"
	sessionsteps "agentx/tests/steps/session"
	surfacesteps "agentx/tests/steps/surfaces"
	toolsteps "agentx/tests/steps/tools"
	transportsteps "agentx/tests/steps/transport"
)

// featurePaths points at the shared feature corpus relative to tests/suites.
var featurePaths = []string{"../features"}

// initializeAll aggregates every behavior domain's step registrations into one
// scenario initializer. Add a line here when a new tests/steps/<domain> package
// is introduced.
func initializeAll(sc *godog.ScenarioContext) {
	executorsteps.InitializeScenario(sc)
	llmsteps.InitializeScenario(sc)
	promptingsteps.InitializeScenario(sc)
	runtimesteps.InitializeScenario(sc)
	sessionsteps.InitializeScenario(sc)
	surfacesteps.InitializeScenario(sc)
	toolsteps.InitializeScenario(sc)
	transportsteps.InitializeScenario(sc)
}

// runTagged runs every registered scenario initializer against the feature
// corpus, filtered to the given tag expression, and fails the test on a
// non-zero Godog status.
func runTagged(t *testing.T, tags string) {
	t.Helper()

	suite := godog.TestSuite{
		Name:                "agentx",
		ScenarioInitializer: initializeAll,
		Options: &godog.Options{
			// "progress" (one char per step) not "pretty" (full per-step render):
			// on a failing/spinning scenario the pretty formatter's output balloons
			// and OOM-kills anything buffering it. See docs note on suite output.
			Format:   "progress",
			Paths:    featurePaths,
			Tags:     tags,
			Strict:   true,
			TestingT: t,
		},
	}

	if status := suite.Run(); status != 0 {
		t.Fatalf("godog suite %q returned non-zero status %d", tags, status)
	}
}

func TestUnit(t *testing.T)        { runTagged(t, "@unit") }
func TestIntegration(t *testing.T) { runTagged(t, "@integration") }
func TestFunctional(t *testing.T)  { runTagged(t, "@functional") }
func TestE2E(t *testing.T)         { runTagged(t, "@e2e") }
