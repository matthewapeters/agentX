package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

type startupLogoMode struct {
	inputWidget       bool
	outputWidget      bool
	logsWidget        bool
	contextWidget     bool
	filesystemWidget  bool
	settingsWidget    bool
	layoutTemplate    bool
	dumpDefaultLayout bool
}

func shouldPrintStartupLogo(mode startupLogoMode) bool {
	if mode.inputWidget || mode.outputWidget || mode.logsWidget || mode.contextWidget || mode.filesystemWidget || mode.settingsWidget {
		return false
	}
	if mode.layoutTemplate || mode.dumpDefaultLayout {
		return false
	}
	return true
}

func printStartupLogoForMode(mode startupLogoMode) {
	if !shouldPrintStartupLogo(mode) {
		return
	}
	printStartupLogo()
}

// printStartupLogo writes the startup logo exactly once before any other runtime output.
func printStartupLogo() {
	printStartupLogoToWriters(startupLogoBase64, os.Stdout, os.Stderr)
}

func printStartupLogoToWriters(encoded string, out io.Writer, errOut io.Writer) {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "[AgentX Core] failed to decode startup logo: %v\n", err)
		return
	}
	_, _ = out.Write(decoded)
}
