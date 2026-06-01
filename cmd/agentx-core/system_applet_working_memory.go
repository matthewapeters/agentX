package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type workingMemorySystemApplet struct{}

func (workingMemorySystemApplet) ID() string {
	return "working-memory"
}

func (workingMemorySystemApplet) RenderCore(ctx SystemAppletCoreContext) []string {
	return renderWorkingMemoryAppletSection(ctx.SessionDir, ctx.SessionID)
}

func (workingMemorySystemApplet) RenderWidget(ctx SystemAppletWidgetContext) []string {
	return renderWorkingMemoryAppletSection(ctx.SessionDir, ctx.SessionID)
}

type workingMemoryFactSnapshot struct {
	Owner   string `json:"owner"`
	Key     string `json:"key"`
	Value   any    `json:"value"`
	Enabled bool   `json:"enabled"`
}

type workingMemoryFactLine struct {
	owner   string
	key     string
	value   any
	enabled bool
}

func renderWorkingMemoryAppletSection(sessionDir string, sessionID string) []string {
	lines := []string{"== WORKING MEMORY =="}
	if trimmedSessionID := strings.TrimSpace(sessionID); trimmedSessionID != "" {
		lines = append(lines, fmt.Sprintf("session_id: %s", trimSingleLine(trimmedSessionID, 40)))
	}

	facts := loadWorkingMemoryFacts(sessionDir)
	lines = append(lines, fmt.Sprintf("fact_count: %d", len(facts)))
	if len(facts) == 0 {
		lines = append(lines, "No facts stored yet.")
		return lines
	}

	enabledCount := 0
	lines = append(lines, "facts:")
	for index, fact := range facts {
		if fact.enabled {
			enabledCount++
		}
		status := "disabled"
		if fact.enabled {
			status = "enabled"
		}
		icon := "🤖"
		if fact.owner == "user" {
			icon = "👤"
		}
		lines = append(lines, fmt.Sprintf("  %d. %s %s [%s] = %s", index+1, icon, trimSingleLine(fact.key, 40), status, trimSingleLine(formatWorkingMemoryValue(fact.value), 80)))
	}
	lines = append(lines, fmt.Sprintf("enabled_fact_count: %d", enabledCount))
	return lines
}

func loadWorkingMemoryFacts(sessionDir string) []workingMemoryFactLine {
	trimmedSessionDir := strings.TrimSpace(sessionDir)
	if trimmedSessionDir == "" {
		return []workingMemoryFactLine{}
	}
	target := filepath.Join(trimmedSessionDir, "working_memory.json")
	raw, err := os.ReadFile(target)
	if err != nil {
		return []workingMemoryFactLine{}
	}

	var payload map[string]workingMemoryFactSnapshot
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []workingMemoryFactLine{}
	}

	facts := make([]workingMemoryFactLine, 0, len(payload))
	for compoundKey, snapshot := range payload {
		owner := strings.ToLower(strings.TrimSpace(snapshot.Owner))
		key := strings.TrimSpace(snapshot.Key)
		if owner != "user" && owner != "agent" {
			if strings.HasPrefix(strings.ToLower(compoundKey), "agent:") {
				owner = "agent"
			} else {
				owner = "user"
			}
		}
		if key == "" {
			if _, suffix, ok := strings.Cut(compoundKey, ":"); ok {
				key = suffix
			}
		}
		facts = append(facts, workingMemoryFactLine{owner: owner, key: key, value: snapshot.Value, enabled: snapshot.Enabled})
	}

	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].owner != facts[j].owner {
			return facts[i].owner == "user"
		}
		if facts[i].key != facts[j].key {
			return facts[i].key < facts[j].key
		}
		return formatWorkingMemoryValue(facts[i].value) < formatWorkingMemoryValue(facts[j].value)
	})

	return facts
}

func formatWorkingMemoryValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return "null"
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(encoded)
	}
}