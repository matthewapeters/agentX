package main

import (
	"fmt"
	"sort"
)

const (
	PaneTitleOutput        = "output"
	PaneTitleSystem        = "system"
	PaneTitleInput         = "input"
	PaneTitleLogs          = "logs"
	PaneTitleStores        = "stores"
	PaneTitleTestControler = "testControler"
	PaneTitleLiveCore      = "liveCore"
)

var allowedPaneTitles = map[string]struct{}{
	PaneTitleOutput:        {},
	PaneTitleSystem:        {},
	PaneTitleInput:         {},
	PaneTitleLogs:          {},
	PaneTitleStores:        {},
	PaneTitleTestControler: {},
	PaneTitleLiveCore:      {},
}

func validatePaneTitle(title string) error {
	if _, ok := allowedPaneTitles[title]; ok {
		return nil
	}
	return fmt.Errorf("unsupported pane title %q (must be documented in UX contract)", title)
}

func sortedPaneTitles() []string {
	titles := make([]string, 0, len(allowedPaneTitles))
	for title := range allowedPaneTitles {
		titles = append(titles, title)
	}
	sort.Strings(titles)
	return titles
}
