package main

// OutputResponseFormatter defines response-specific formatting hooks for output widget rendering.
//
// Contract:
//   - width is a hard visible-width budget. Implementations must not assume callers can render
//     content wider than width without truncation.
//   - FormatCollapsedPreview should treat empty or whitespace-only raw content as empty preview
//     semantics used by the default path ("none").
//   - ANSI is allowed in returned strings only if implementations ensure visible-width stability;
//     the default formatter returns plain text consistent with existing wrapping/preview helpers.
//   - Both methods must be deterministic and idempotent for identical (raw, width) inputs.
type OutputResponseFormatter interface {
	FormatResponse(raw string, width int) []string
	FormatCollapsedPreview(raw string, width int) string
}

type plainTextOutputFormatter struct{}

func (f plainTextOutputFormatter) FormatResponse(raw string, width int) []string {
	return wrapOutputWidgetContent(raw, width)
}

func (f plainTextOutputFormatter) FormatCollapsedPreview(raw string, width int) string {
	return renderOutputWidgetCollapsedPreview(raw, width)
}

// PlainTextOutputFormatter returns the default formatter that preserves current plain-text behavior.
func PlainTextOutputFormatter() OutputResponseFormatter {
	return plainTextOutputFormatter{}
}

// DefaultOutputResponseFormatter returns the default response formatter.
func DefaultOutputResponseFormatter() OutputResponseFormatter {
	return PlainTextOutputFormatter()
}
