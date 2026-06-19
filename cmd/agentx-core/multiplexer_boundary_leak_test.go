package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var multiplexerBoundaryLeakPattern = regexp.MustCompile(`exec\.Command(?:Context)?\s*\(\s*(?:ctx\s*,\s*)?"tmux"`)
var zellijBoundaryLeakPattern = regexp.MustCompile(`exec\.Command(?:Context)?\s*\(\s*(?:ctx\s*,\s*)?"zellij"`)

// GIVEN the multiplexer boundary is being introduced incrementally
// WHEN structural checks scan Go runtime files for direct tmux execution
// THEN only explicit temporary exemptions may contain direct tmux exec calls.
func TestMultiplexerBoundaryLeak_DirectTmuxExecRestrictedToApprovedFiles(t *testing.T) {
	goCoreDir := locateGoCoreDir(t)

	approvedTemporaryExemptions := map[string]bool{
		// Canonical backend implementation is expected to execute tmux directly.
		"multiplexer_driver_tmux.go": true,
	}

	dirEntries, err := os.ReadDir(goCoreDir)
	if err != nil {
		t.Fatalf("failed reading %s: %v", goCoreDir, err)
	}

	violations := make([]string, 0)
	for _, entry := range dirEntries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(goCoreDir, name)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("failed reading %s: %v", path, readErr)
		}

		matches := multiplexerBoundaryLeakPattern.FindAllStringIndex(string(data), -1)
		if len(matches) == 0 {
			continue
		}
		if approvedTemporaryExemptions[name] {
			continue
		}

		violations = append(violations, name)
	}

	if len(violations) > 0 {
		t.Fatalf("direct tmux exec leaked outside approved files: %s", strings.Join(violations, ", "))
	}
}

// GIVEN the multiplexer boundary is being introduced incrementally
// WHEN structural checks scan Go runtime files for direct tmux driver construction
// THEN only explicit temporary exemptions may construct the tmux backend directly.
func TestMultiplexerBoundaryLeak_DirectTmuxDriverConstructionRestrictedToApprovedFiles(t *testing.T) {
	goCoreDir := locateGoCoreDir(t)

	approvedTemporaryExemptions := map[string]bool{
		// Backend constructor declaration file and canonical backend implementation.
		"multiplexer_driver_tmux.go": true,
		// Canonical factory seam for backend resolution from config.
		"multiplexer_driver.go": true,
		// Core constructor default and nil-driver fallback for dependency injection path.
		"core.go": true,
	}

	dirEntries, err := os.ReadDir(goCoreDir)
	if err != nil {
		t.Fatalf("failed reading %s: %v", goCoreDir, err)
	}

	violations := make([]string, 0)
	for _, entry := range dirEntries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(goCoreDir, name)
		hasDirectCtorCall, parseErr := hasDirectTmuxDriverConstructorCall(path)
		if parseErr != nil {
			t.Fatalf("failed parsing %s: %v", path, parseErr)
		}
		if !hasDirectCtorCall {
			continue
		}
		if approvedTemporaryExemptions[name] {
			continue
		}

		violations = append(violations, name)
	}

	if len(violations) > 0 {
		t.Fatalf("direct NewTmuxMultiplexerDriver usage leaked outside approved files: %s", strings.Join(violations, ", "))
	}
}

func TestMultiplexerBoundaryLeak_DirectZellijExecRestrictedToApprovedFiles(t *testing.T) {
	goCoreDir := locateGoCoreDir(t)

	approvedTemporaryExemptions := map[string]bool{
		"multiplexer_driver_zellij.go": true,
	}

	dirEntries, err := os.ReadDir(goCoreDir)
	if err != nil {
		t.Fatalf("failed reading %s: %v", goCoreDir, err)
	}

	violations := make([]string, 0)
	for _, entry := range dirEntries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(goCoreDir, name)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("failed reading %s: %v", path, readErr)
		}

		matches := zellijBoundaryLeakPattern.FindAllStringIndex(string(data), -1)
		if len(matches) == 0 {
			continue
		}
		if approvedTemporaryExemptions[name] {
			continue
		}

		violations = append(violations, name)
	}

	if len(violations) > 0 {
		t.Fatalf("direct zellij exec leaked outside approved files: %s", strings.Join(violations, ", "))
	}
}

func TestMultiplexerBoundaryLeak_DirectZellijDriverConstructionRestrictedToApprovedFiles(t *testing.T) {
	goCoreDir := locateGoCoreDir(t)

	approvedTemporaryExemptions := map[string]bool{
		"multiplexer_driver_zellij.go": true,
		"multiplexer_driver.go":        true,
		"core.go":                      true,
	}

	dirEntries, err := os.ReadDir(goCoreDir)
	if err != nil {
		t.Fatalf("failed reading %s: %v", goCoreDir, err)
	}

	violations := make([]string, 0)
	for _, entry := range dirEntries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(goCoreDir, name)
		hasDirectCtorCall, parseErr := hasDirectDriverConstructorCall(path, "NewZellijMultiplexerDriver")
		if parseErr != nil {
			t.Fatalf("failed parsing %s: %v", path, parseErr)
		}
		if !hasDirectCtorCall {
			continue
		}
		if approvedTemporaryExemptions[name] {
			continue
		}

		violations = append(violations, name)
	}

	if len(violations) > 0 {
		t.Fatalf("direct NewZellijMultiplexerDriver usage leaked outside approved files: %s", strings.Join(violations, ", "))
	}
}

func hasDirectTmuxDriverConstructorCall(path string) (bool, error) {
	return hasDirectDriverConstructorCall(path, "NewTmuxMultiplexerDriver")
}

func hasDirectDriverConstructorCall(path string, constructorName string) (bool, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return false, err
	}

	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if found {
			return false
		}

		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}

		if ident.Name == constructorName {
			found = true
			return false
		}

		return true
	})

	return found, nil
}

func locateGoCoreDir(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if filepath.Base(wd) == "agentx-core" {
		return wd
	}

	candidate := wd
	for {
		goCorePath := filepath.Join(candidate, "cmd", "agentx-core")
		if _, statErr := os.Stat(filepath.Join(goCorePath, "go.mod")); statErr == nil {
			return goCorePath
		}
		next := filepath.Dir(candidate)
		if next == candidate {
			t.Fatalf("unable to locate cmd/agentx-core from %s", wd)
		}
		candidate = next
	}
}
