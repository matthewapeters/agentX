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
