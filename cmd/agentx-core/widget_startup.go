package main

import (
	"io"
	"os"
	"strconv"
	"strings"
)

func consumeWidgetPaneSeed() (height int, width int, ok bool) {
	heightRaw := strings.TrimSpace(os.Getenv("AGENTX_WIDGET_PANE_HEIGHT"))
	widthRaw := strings.TrimSpace(os.Getenv("AGENTX_WIDGET_PANE_WIDTH"))
	if heightRaw == "" || widthRaw == "" {
		return 0, 0, false
	}

	parsedHeight, errHeight := strconv.Atoi(heightRaw)
	parsedWidth, errWidth := strconv.Atoi(widthRaw)
	if errHeight != nil || errWidth != nil || parsedHeight <= 0 || parsedWidth <= 0 {
		return 0, 0, false
	}

	_ = os.Unsetenv("AGENTX_WIDGET_PANE_HEIGHT")
	_ = os.Unsetenv("AGENTX_WIDGET_PANE_WIDTH")
	return parsedHeight, parsedWidth, true
}

func resolveWidgetPaneSizeAtStartup(out io.Writer) (height int, width int) {
	if seededHeight, seededWidth, ok := consumeWidgetPaneSeed(); ok {
		return seededHeight, seededWidth
	}
	return resolveWidgetPaneSizeForWriter(out)
}
