package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type configurationSystemApplet struct{}

func (configurationSystemApplet) ID() string {
	return "configuration"
}

func (configurationSystemApplet) RenderCore(ctx SystemAppletCoreContext) []string {
	return renderConfigurationAppletSection(ctx.ProjectDir, ctx.Model, ctx.Backend, ctx.OllamaHost, 24, 12, 40)
}

func (configurationSystemApplet) RenderWidget(ctx SystemAppletWidgetContext) []string {
	return renderConfigurationAppletSection(ctx.ProjectDir, ctx.Model, ctx.Backend, ctx.OllamaHost, 32, 20, 32)
}

func renderConfigurationAppletSection(projectDir string, model string, backend string, ollamaHost string, modelLimit int, backendLimit int, hostLimit int) []string {
	lines := []string{
		"== CONFIGURATION ==",
		fmtKV("config_file", resolveAgentXTomlPath(projectDir)),
		fmtKV("effective_model", trimSingleLine(model, modelLimit)),
		fmtKV("effective_backend", trimSingleLine(backend, backendLimit)),
		fmtKV("effective_ollama_host", trimSingleLine(ollamaHost, hostLimit)),
	}

	sections := loadAgentXTomlSections(projectDir)
	if len(sections) == 0 {
		return append(lines, "agentx.toml: not found or contains no scalar settings")
	}

	sectionNames := make([]string, 0, len(sections))
	for section := range sections {
		sectionNames = append(sectionNames, section)
	}
	sort.Strings(sectionNames)

	for _, section := range sectionNames {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("[%s]", section))
		for _, setting := range sections[section] {
			lines = append(lines, fmtKV(setting.Key, setting.Value))
		}
	}

	return lines
}

type configurationSetting struct {
	Key   string
	Value string
}

func resolveAgentXTomlPath(projectDir string) string {
	root := strings.TrimSpace(projectDir)
	if root == "" {
		root = "."
	}
	return filepath.Join(root, "agentx.toml")
}

func loadAgentXTomlSections(projectDir string) map[string][]configurationSetting {
	configPath := resolveAgentXTomlPath(projectDir)
	file, err := os.Open(configPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	sections := map[string][]configurationSetting{}
	currentSection := "root"

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if currentSection == "" {
				currentSection = "root"
			}
			continue
		}
		key, value, ok := parseTomlKeyValue(line)
		if !ok {
			continue
		}
		sections[currentSection] = append(sections[currentSection], configurationSetting{Key: key, Value: trimSingleLine(value, 96)})
	}

	return sections
}
