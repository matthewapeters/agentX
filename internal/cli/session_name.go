package cli

import (
	"agentx/internal/config"
	"agentx/internal/session"
)

// NewSessionName mints a session name in AgentX's default adjective-noun style for
// a scripted launcher (`agentx session new-name`), so its names match the ones the
// app generates. It prefers a name not already used by a session on disk; if the
// session root cannot be resolved or read, it falls back to a plain generated name.
func NewSessionName() string {
	paths, err := config.DefaultPaths()
	if err != nil {
		return session.GenerateName()
	}
	name, err := session.NewStore(paths.SessionRoot()).UniqueName()
	if err != nil {
		return session.GenerateName()
	}
	return name
}
