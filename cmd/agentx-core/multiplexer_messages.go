package main

import "fmt"

func multiplexerAttachHint(backendName string, sessionName string) string {
	switch backendName {
	case "zellij":
		return fmt.Sprintf("zellij attach %s", sessionName)
	default:
		return fmt.Sprintf("tmux attach -t %s", sessionName)
	}
}

func multiplexerSupportsDemoSplit(backendName string) bool {
	return backendName == defaultMultiplexerBackend
}
