package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type filesSystemApplet struct{}

func (filesSystemApplet) ID() string {
	return "files"
}

func (filesSystemApplet) RenderCore(ctx SystemAppletCoreContext) []string {
	return renderFilesAppletSection(ctx.ProjectDir, 64, true)
}

func (filesSystemApplet) RenderWidget(ctx SystemAppletWidgetContext) []string {
	return renderFilesAppletSection(ctx.ProjectDir, 40, false)
}

func renderFilesAppletSection(projectDir string, projectDirLimit int, includePreview bool) []string {
	entries := safeListDir(projectDir)
	lines := []string{
		"== FILES ==",
		fmt.Sprintf("project_dir: %s", trimSingleLine(projectDir, projectDirLimit)),
		fmt.Sprintf("entry_count: %d", len(entries)),
	}
	if !includePreview {
		return lines
	}

	lines = append(lines, "preview:")
	if len(entries) == 0 {
		return append(lines, "- none")
	}

	for _, name := range entries[:minInt(3, len(entries))] {
		entryPath := filepath.Join(projectDir, name)
		entryType := "other"
		if info, err := os.Stat(entryPath); err == nil {
			if info.IsDir() {
				entryType = "dir"
			} else {
				entryType = "file"
			}
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", entryType, trimSingleLine(name, 36)))
	}
	return lines
}
