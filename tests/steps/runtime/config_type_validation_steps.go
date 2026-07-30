// Package runtimesteps adds Godog step definitions for the config validation
// domain (Phase 1c: type-appropriate validation).
package runtimesteps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/llm/provider/validation"
)

// typeValidateWorld carries per-scenario state for type-validation scenarios.
type typeValidateWorld struct {
	err *validation.Error
}

// InitializeTypeValidation registers the type-validation steps.
func InitializeTypeValidation(sc *godog.ScenarioContext) {
	w := &typeValidateWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.err = nil
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		*w = typeValidateWorld{}
		return ctx, nil
	})

	// int: a value "42" validated as an integer with range [0, 100]
	sc.Step(`^a value "([^"]*)" validated as an integer with range \[(\d+), (\d+)\]$`, w.validatedAsInt)
	// bool: a value "true" validated as a boolean
	sc.Step(`^a value "([^"]*)" validated as a boolean$`, w.validatedAsBool)
	// enum: a value "ollama" validated as an enum with ["ollama", "llamacpp"]
	sc.Step(`^a value "([^"]*)" validated as an enum with \["([^"]*)", "([^"]*)"\]$`, w.validatedAsEnum)
	// color: a value "cyan" validated as a color
	sc.Step(`^a value "([^"]*)" validated as a color$`, w.validatedAsColor)
	// host: a value "localhost:11434" validated as a host
	sc.Step(`^a value "([^"]*)" validated as a host$`, w.validatedAsHost)
	// model: a value "phi4:latest" validated as a model name
	sc.Step(`^a value "([^"]*)" validated as a model name$`, w.validatedAsModel)
	// non-empty: a value "" validated as a non-empty string
	sc.Step(`^a value "([^"]*)" validated as a non-empty string$`, w.validatedAsNonEmpty)
	// assertions
	sc.Step(`^the value is valid$`, w.isValid)
	sc.Step(`^the value is rejected with "([^"]*)"$`, w.isRejectedWith)
	// the validation runs (no-op: validation was already applied in Given)
	sc.Step(`^the validation runs$`, w.validationRuns)
}

func (w *typeValidateWorld) validatedAsInt(s, minS, maxS string) error {
	min, max := 0, 100
	if minS != "" {
		_, _ = fmt.Sscanf(minS, "%d", &min)
	}
	if maxS != "" {
		_, _ = fmt.Sscanf(maxS, "%d", &max)
	}
	w.err = validation.ValidateInt(s, min, max)
	return nil
}

func (w *typeValidateWorld) validatedAsBool(s string) error {
	w.err = validation.ValidateBool(s)
	return nil
}

func (w *typeValidateWorld) validatedAsEnum(s, a, b string) error {
	w.err = validation.ValidateEnum(s, []string{a, b})
	return nil
}

func (w *typeValidateWorld) validatedAsColor(s string) error {
	w.err = validation.ValidateColor(s)
	return nil
}

func (w *typeValidateWorld) validatedAsHost(s string) error {
	w.err = validation.ValidateHost(s)
	return nil
}

func (w *typeValidateWorld) validatedAsModel(s string) error {
	w.err = validation.ValidateModelName(s)
	return nil
}

func (w *typeValidateWorld) validatedAsNonEmpty(s string) error {
	w.err = validation.ValidateNonEmpty(s)
	return nil
}

func (w *typeValidateWorld) validationRuns() error {
	// no-op: the validation was already applied in the Given step
	return nil
}

func (w *typeValidateWorld) isValid() error {
	if w.err != nil {
		return fmt.Errorf("expected valid, got error: %v", w.err)
	}
	return nil
}

func (w *typeValidateWorld) isRejectedWith(reason string) error {
	if w.err == nil {
		return fmt.Errorf("expected rejection with %q, got nil", reason)
	}
	if !strings.Contains(w.err.Message, reason) {
		return fmt.Errorf("error message = %q, want to contain %q", w.err.Message, reason)
	}
	return nil
}
