package main

import "testing"

func TestStartupLogoContract_InteractiveStartupPrintsFirst(t *testing.T) {
	mode := startupLogoMode{}
	if !shouldPrintStartupLogo(mode) {
		t.Fatalf("expected interactive startup to print logo before first interactive output")
	}
}

func TestStartupLogoContract_DumpDefaultLayoutStdoutIsClean(t *testing.T) {
	mode := startupLogoMode{dumpDefaultLayout: true}
	if shouldPrintStartupLogo(mode) {
		t.Fatalf("expected --dump-default-layout stdout path to have no logo preamble")
	}
}

func TestStartupLogoContract_OutputWidgetStdoutIsClean(t *testing.T) {
	mode := startupLogoMode{outputWidget: true}
	if shouldPrintStartupLogo(mode) {
		t.Fatalf("expected output widget mode stdout path to have no logo preamble")
	}
}
