package main

import "fmt"

type configurationSystemApplet struct{}

func (configurationSystemApplet) ID() string {
	return "configuration"
}

func (configurationSystemApplet) RenderCore(ctx SystemAppletCoreContext) []string {
	return renderConfigurationAppletSection(ctx.Model, ctx.Backend, ctx.OllamaHost, 24, 12, 40)
}

func (configurationSystemApplet) RenderWidget(ctx SystemAppletWidgetContext) []string {
	return renderConfigurationAppletSection(ctx.Model, ctx.Backend, ctx.OllamaHost, 32, 20, 32)
}

func renderConfigurationAppletSection(model string, backend string, ollamaHost string, modelLimit int, backendLimit int, hostLimit int) []string {
	return []string{
		"== CONFIGURATION ==",
		fmt.Sprintf("model: %s", trimSingleLine(model, modelLimit)),
		fmt.Sprintf("backend: %s", trimSingleLine(backend, backendLimit)),
		fmt.Sprintf("ollama_host: %s", trimSingleLine(ollamaHost, hostLimit)),
	}
}
