package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPaneTitles_AreDocumentedInUXSpecs enforces that pane title registry values are authoritatively documented.
//
// GIVEN the pane-title registry in code
// WHEN validating UX contract documents
// THEN every pane title must appear in either 06_TUI_MIRROR.md or 07_DEMO_MODE.md.
func TestPaneTitles_AreDocumentedInUXSpecs(t *testing.T) {
	docPaths := []string{
		filepath.Join("..", "..", "docs", "ux", "06_TUI_MIRROR.md"),
		filepath.Join("..", "..", "docs", "ux", "07_DEMO_MODE.md"),
	}

	builder := strings.Builder{}
	for _, docPath := range docPaths {
		content, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("failed to read UX contract doc %s: %v", docPath, err)
		}
		builder.Write(content)
		builder.WriteString("\n")
	}

	allDocs := builder.String()
	for _, title := range sortedPaneTitles() {
		needle := "`" + title + "`"
		if !strings.Contains(allDocs, needle) {
			t.Fatalf("pane title %q is not authoritatively documented in UX specs", title)
		}
	}
}
