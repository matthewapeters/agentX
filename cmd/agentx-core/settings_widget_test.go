package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCycleSettingValue_ConstrainedOptions(t *testing.T) {
	field := settingsField{Section: "agentx", Key: "chat_backend", Kind: settingsFieldEnum, Options: []string{"ollama", "echo"}}

	next, ok := cycleSettingValue(field, "ollama", 1)
	if !ok || next != "echo" {
		t.Fatalf("expected forward cycle to echo, got ok=%v value=%q", ok, next)
	}
	prev, ok := cycleSettingValue(field, "ollama", -1)
	if !ok || prev != "echo" {
		t.Fatalf("expected backward wrap cycle to echo, got ok=%v value=%q", ok, prev)
	}
}

func TestUpdateAgentXTomlScalar_ReplacesExistingKey(t *testing.T) {
	initial := strings.Join([]string{
		"[agentx]",
		"chat_backend = \"ollama\"",
		"chat_runtime = \"go\"",
	}, "\n")
	_, configPath := createWidgetTestConfigProject(t, initial)

	if err := updateAgentXTomlScalar(configPath, "agentx", "chat_backend", "echo", settingsFieldEnum); err != nil {
		t.Fatalf("updateAgentXTomlScalar returned error: %v", err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "chat_backend = \"echo\"") {
		t.Fatalf("expected updated backend value, got:\n%s", text)
	}
	if strings.Contains(text, "chat_backend = \"ollama\"") {
		t.Fatalf("expected old backend value to be replaced, got:\n%s", text)
	}
}

func TestUpdateAgentXTomlScalar_AppendsMissingSection(t *testing.T) {
	_, configPath := createWidgetTestConfigProject(t, "[agentx]\nchat_backend = \"ollama\"\n")

	if err := updateAgentXTomlScalar(configPath, "terminal", "exec_mode", "confirm", settingsFieldEnum); err != nil {
		t.Fatalf("updateAgentXTomlScalar returned error: %v", err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "[terminal]") || !strings.Contains(text, "exec_mode = \"confirm\"") {
		t.Fatalf("expected missing section and key append, got:\n%s", text)
	}
}

func TestSettingsWidgetStateReload_UsesApprovedFieldDefaults(t *testing.T) {
	projectDir := t.TempDir()
	state := &settingsWidgetState{
		projectDir: projectDir,
		configPath: filepath.Join(projectDir, "agentx.toml"),
		fields: []settingsField{
			{Section: "agentx", Key: "chat_backend", Kind: settingsFieldEnum, Options: []string{"ollama", "echo"}},
		},
		values: map[string]string{},
	}

	if err := state.reload(); err != nil {
		t.Fatalf("reload returned error: %v", err)
	}
	if got := state.values["agentx.chat_backend"]; got != "ollama" {
		t.Fatalf("expected default constrained value ollama, got %q", got)
	}
}

func TestNormalizeSettingsWidgetControlCommand(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "colon help", raw: ":help", want: "help"},
		{name: "question alias", raw: ":?", want: "help"},
		{name: "colon quit", raw: ":q", want: "q"},
		{name: "exit alias", raw: ":exit", want: "quit"},
		{name: "up maps to k", raw: ":up", want: "k"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeSettingsWidgetControlCommand(tc.raw); got != tc.want {
				t.Fatalf("normalizeSettingsWidgetControlCommand(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestRunSettingsWidgetCommand_QuitTokenStopsLoop(t *testing.T) {
	projectDir := t.TempDir()
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
	})

	exitCode, output := runHeadlessWidgetCommandScript(t, "q\n", func(in io.Reader, out io.Writer) int {
		return runSettingsWidgetCommand("", in, out)
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d output=%q", exitCode, output)
	}
	if !strings.Contains(output, "SYSTEM SETTINGS") {
		t.Fatalf("expected settings frame output, got %q", output)
	}
}
