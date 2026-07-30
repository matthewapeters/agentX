// Package runtimesteps adds Godog step definitions for the config normalization
// domain (Phase 1c: chat_backend â provider normalization).
package runtimesteps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"agentx/internal/config"
)

// normalizeWorld carries per-scenario state for normalization scenarios.
type normalizeWorld struct {
	cfg        config.Config
	normalized []config.NormalizedKey
}

// InitializeNormalization registers the normalization steps.
func InitializeNormalization(sc *godog.ScenarioContext) {
	w := &normalizeWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		w.cfg = config.Default()
		w.normalized = nil
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		*w = normalizeWorld{}
		return ctx, nil
	})

	// a config with chat_backend "ollama" and no provider
	sc.Step(`^a config with chat_backend "([^"]*)" and no provider$`, w.chatBackendOnly)
	// a config with provider "ollama" and chat_backend "ollama"
	sc.Step(`^a config with provider "([^"]*)" and chat_backend "([^"]*)"$`, w.providerAndChatBackend)
	// a config with no provider and no chat_backend
	sc.Step(`^a config with no provider and no chat_backend$`, w.empty)
	// a config with provider "ollama" and no chat_backend
	sc.Step(`^a config with provider "([^"]*)" and no chat_backend$`, w.providerOnly)
	// the config is normalized
	sc.Step(`^the config is normalized$`, w.normalize)
	// the provider is "ollama"
	sc.Step(`^the provider is "([^"]*)"$`, w.providerIs)
	// a normalization is recorded from "chat_backend" to "provider"
	sc.Step(`^a normalization is recorded from "([^"]*)" to "([^"]*)"$`, w.normalizationRecorded)
	// no normalization is recorded
	sc.Step(`^no normalization is recorded$`, w.noNormalization)
	// the config has deprecated keys
	sc.Step(`^the config has deprecated keys$`, w.hasDeprecated)
	// the config has no deprecated keys
	sc.Step(`^the config has no deprecated keys$`, w.hasNoDeprecated)
}

func (w *normalizeWorld) chatBackendOnly(cb string) error {
	w.cfg = config.Default()
	w.cfg.Agentx.Provider = ""
	w.cfg.Agentx.ChatBackend = cb
	return nil
}

func (w *normalizeWorld) providerAndChatBackend(p, cb string) error {
	w.cfg = config.Default()
	w.cfg.Agentx.Provider = p
	w.cfg.Agentx.ChatBackend = cb
	return nil
}

func (w *normalizeWorld) empty() error {
	w.cfg = config.Default()
	w.cfg.Agentx.Provider = ""
	w.cfg.Agentx.ChatBackend = ""
	return nil
}

func (w *normalizeWorld) providerOnly(p string) error {
	w.cfg = config.Default()
	w.cfg.Agentx.Provider = p
	w.cfg.Agentx.ChatBackend = ""
	return nil
}

func (w *normalizeWorld) normalize() error {
	w.normalized = w.cfg.Normalize()
	return nil
}

func (w *normalizeWorld) providerIs(want string) error {
	if got := w.cfg.Provider(); got != want {
		return fmt.Errorf("Provider = %q, want %q", got, want)
	}
	return nil
}

func (w *normalizeWorld) normalizationRecorded(old, newKey string) error {
	for _, n := range w.normalized {
		if n.Old == old && n.New == newKey {
			return nil
		}
	}
	return fmt.Errorf("expected normalization %s -> %s, got %+v", old, newKey, w.normalized)
}

func (w *normalizeWorld) noNormalization() error {
	if len(w.normalized) != 0 {
		return fmt.Errorf("expected no normalization, got %+v", w.normalized)
	}
	return nil
}

func (w *normalizeWorld) hasDeprecated() error {
	if !w.cfg.IsProviderDeprecated() {
		return fmt.Errorf("expected deprecated keys")
	}
	return nil
}

func (w *normalizeWorld) hasNoDeprecated() error {
	if w.cfg.IsProviderDeprecated() {
		return fmt.Errorf("expected no deprecated keys")
	}
	return nil
}
