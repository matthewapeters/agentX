package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestShouldPrintStartupLogo_InteractiveDefault(t *testing.T) {
	if !shouldPrintStartupLogo(startupLogoMode{}) {
		t.Fatalf("expected startup logo for default interactive startup path")
	}
}

func TestShouldPrintStartupLogo_DisabledForWidgetModes(t *testing.T) {
	tests := []startupLogoMode{
		{inputWidget: true},
		{outputWidget: true},
		{logsWidget: true},
		{contextWidget: true},
		{filesystemWidget: true},
		{settingsWidget: true},
	}

	for _, tc := range tests {
		if shouldPrintStartupLogo(tc) {
			t.Fatalf("expected startup logo suppressed for widget mode: %#v", tc)
		}
	}
}

func TestShouldPrintStartupLogo_DisabledForUtilityModes(t *testing.T) {
	if shouldPrintStartupLogo(startupLogoMode{dumpDefaultLayout: true}) {
		t.Fatalf("expected startup logo suppressed for dump-default-layout mode")
	}
	if shouldPrintStartupLogo(startupLogoMode{layoutTemplate: true}) {
		t.Fatalf("expected startup logo suppressed for layout-template mode")
	}
}

func TestPrintStartupLogoToWriters_Success(t *testing.T) {
	payload := []byte("LOGO\n")
	encoded := base64.StdEncoding.EncodeToString(payload)
	var out bytes.Buffer
	var errOut bytes.Buffer

	printStartupLogoToWriters(encoded, &out, &errOut)

	if out.String() != string(payload) {
		t.Fatalf("unexpected logo output: got %q want %q", out.String(), string(payload))
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output on success, got %q", errOut.String())
	}
}

func TestPrintStartupLogoToWriters_FailureSafeInvalidBase64(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	printStartupLogoToWriters("%%%not_base64%%%", &out, &errOut)

	if out.Len() != 0 {
		t.Fatalf("expected no stdout logo bytes on decode failure, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), "failed to decode startup logo") {
		t.Fatalf("expected decode failure warning, got %q", errOut.String())
	}
}

func TestPrintStartupLogoToWriters_NilWritersDoNotPanic(t *testing.T) {
	printStartupLogoToWriters("%%%not_base64%%%", nil, nil)
}
