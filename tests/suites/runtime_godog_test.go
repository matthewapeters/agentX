// Package suites holds Godog suite runners. Each runner is tag-scoped so the
// Makefile targets (go-test-unit/integration/functional/e2e) can select a slice
// of the behavior corpus. See docs/implementation/07_test_and_documentation_contract.md
// for the required tag scheme and docs/implementation/08_go_module_layout.md for
// the suite layout.
package suites

import (
	"testing"

	"github.com/cucumber/godog"

	runtimesteps "agentx/tests/steps/runtime"
)

// featurePaths points at the shared feature corpus relative to tests/suites.
var featurePaths = []string{"../features"}

// runTagged runs every registered scenario initializer against the feature
// corpus, filtered to the given tag expression, and fails the test on a
// non-zero Godog status.
func runTagged(t *testing.T, tags string) {
	t.Helper()

	suite := godog.TestSuite{
		Name:                "agentx",
		ScenarioInitializer: runtimesteps.InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
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
