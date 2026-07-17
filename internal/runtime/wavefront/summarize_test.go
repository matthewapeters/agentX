package wavefront

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNewCondenserSuccessfulChatDisclosesSummary(t *testing.T) {
	var sawFormat json.RawMessage
	var sawFormatSet bool
	chat := func(_ context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error) {
		sawFormat = format
		sawFormatSet = true
		if !strings.Contains(userPrompt, "raw text to condense") {
			t.Errorf("user prompt missing the raw text: %q", userPrompt)
		}
		return "condensed version", nil
	}
	condense := NewCondenser(chat, "")
	got := condense(context.Background(), []string{"top-level goal", "specific question"}, "raw text to condense")

	if !strings.HasPrefix(got, "[summarized from") || !strings.Contains(got, "condensed version") {
		t.Errorf("got = %q, want a disclosed summary", got)
	}
	if !sawFormatSet || sawFormat != nil {
		t.Errorf("format = %v, want nil (schema-free call)", sawFormat)
	}
}

func TestNewCondenserChatErrorFallsBackToTruncate(t *testing.T) {
	chat := func(context.Context, string, string, json.RawMessage) (string, error) {
		return "", errors.New("model unavailable")
	}
	condense := NewCondenser(chat, "")
	text := strings.Repeat("x", OutputSummaryTargetChars+50)
	got := condense(context.Background(), []string{"q"}, text)

	if !strings.Contains(got, "[truncated,") {
		t.Errorf("got = %q, want a disclosed truncation fallback", got)
	}
}

func TestNewCondenserEmptyChatResponseFallsBackToTruncate(t *testing.T) {
	chat := func(context.Context, string, string, json.RawMessage) (string, error) {
		return "   ", nil // whitespace-only: a "successful" call with nothing usable
	}
	condense := NewCondenser(chat, "")
	text := strings.Repeat("x", OutputSummaryTargetChars+50)
	got := condense(context.Background(), []string{"q"}, text)

	if !strings.Contains(got, "[truncated,") {
		t.Errorf("got = %q, want a disclosed truncation fallback for an empty response", got)
	}
}

func TestNewCondenserDefaultsTemplateWhenEmpty(t *testing.T) {
	var sawSystem string
	chat := func(_ context.Context, systemPrompt, _ string, _ json.RawMessage) (string, error) {
		sawSystem = systemPrompt
		return "ok", nil
	}
	condense := NewCondenser(chat, "") // template left empty
	condense(context.Background(), []string{"q"}, "text")

	if !strings.Contains(sawSystem, strconv.Itoa(OutputSummaryTargetChars)) {
		t.Errorf("system prompt did not fall back to DefaultSummaryPromptTemplate: %q", sawSystem)
	}
}

func TestNewCondenserCustomTemplateOverrides(t *testing.T) {
	var sawSystem string
	chat := func(_ context.Context, systemPrompt, _ string, _ json.RawMessage) (string, error) {
		sawSystem = systemPrompt
		return "ok", nil
	}
	condense := NewCondenser(chat, "CUSTOM TEMPLATE with {{target_chars}} chars")
	condense(context.Background(), []string{"q"}, "text")

	if !strings.Contains(sawSystem, "CUSTOM TEMPLATE") {
		t.Errorf("system prompt did not use the custom template: %q", sawSystem)
	}
}

// TestTruncateFindingsIsRuneSafe: a naive byte-index slice can split a multi-byte
// UTF-8 rune at the truncation boundary and produce invalid text — regression guard.
func TestTruncateFindingsIsRuneSafe(t *testing.T) {
	text := strings.Repeat("€", OutputSummaryTargetChars+10) // 3-byte rune in UTF-8
	got := TruncateFindings(text)
	if !utf8.ValidString(got) {
		t.Fatalf("TruncateFindings produced invalid UTF-8: %q", got)
	}
	if !strings.Contains(got, "[truncated,") {
		t.Errorf("TruncateFindings = %q, want a truncation marker", got)
	}
}

func TestTruncateFindingsBelowTargetIsUnchanged(t *testing.T) {
	text := "short text"
	if got := TruncateFindings(text); got != text {
		t.Errorf("TruncateFindings(%q) = %q, want unchanged", text, got)
	}
}
