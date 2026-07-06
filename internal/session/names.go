package session

import "math/rand"

// Adjective-noun word lists for default human-readable session names.
var (
	adjectives = []string{
		"amber", "brave", "calm", "clever", "eager", "gentle", "jolly",
		"keen", "lively", "mellow", "nimble", "proud", "quiet", "swift",
		"tidy", "vivid", "witty", "zesty",
	}
	nouns = []string{
		"otter", "falcon", "willow", "harbor", "cedar", "lantern", "pebble",
		"meadow", "comet", "ember", "quartz", "raven", "summit", "thicket",
		"beacon", "marten", "cove", "fjord",
	}
)

// defaultNamer returns a random adjective-noun name (for example "brave-otter").
func defaultNamer() string {
	return adjectives[rand.Intn(len(adjectives))] + "-" + nouns[rand.Intn(len(nouns))]
}

// GenerateName returns a random adjective-noun session name in AgentX's default
// style, with no uniqueness guarantee. It lets callers outside the store — such as
// the `agentx session new-name` helper — mint a name in the same vocabulary the
// runtime uses, so a scripted launcher's names match the app's.
func GenerateName() string { return defaultNamer() }

// UniqueName returns a default-style name that is not already in use by a session
// under this store's root (suffixing "-2", "-3", … on collision). It is the
// store-aware counterpart to GenerateName, used to pre-mint a launcher's session
// name that will not clash with a session already on disk.
func (s *Store) UniqueName() (string, error) { return s.uniqueName(defaultNamer) }
