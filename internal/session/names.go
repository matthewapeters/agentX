package session

import "math/rand"

// Adjective-noun word lists for default human-readable session names.
var (
	participials = []string{
		"alarming", "amazing", "annoying", "boring", "calming", "challenging",
		"confusing", "delightful", "depressing", "disappointing", "discouraging",
		"dismaying", "disturbing", "embarrassing", "exciting", "exhausting",
		"frightening", "frustrating", "heartbreaking", "impressive", "inspiring",
		"intriguing", "motivating", "overwhelming", "puzzling", "reassuring",
		"refreshing", "remarkable", "satisfying", "shocking", "stunning",
		"terrifying", "tiresome", "unforgettable", "upsetting", "wonderful",
		"comforting", "devastating", "enchanting", "entertaining",
		"exasperating", "fascinating", "humiliating", "interesting", "surprising",
		"thrilling",
	}

	adjectives = []string{
		"absent", "active", "acute", "adventurous", "aggressive", "alert",
		"ambitious", "angry", "anxious", "apprehensive", "arrogant", "ashamed",
		"astonished", "attractive", "available", "awake", "aware", "awesome",
		"awkward", "balanced", "beautiful", "bitter", "bold", "brave", "bright",
		"broken", "busy", "calm", "careful", "capable", "caring", "cautious",
		"charming", "cheerful", "clean", "clear", "clever", "comfortable",
		"confident", "conscious", "cool", "corrupt", "creative", "critical",
		"curious", "cute", "dangerous", "dark", "dead", "deafening", "delicate",
		"determined", "difficult", "direct", "dirty", "disgusted", "distinct",
		"distant", "diverse", "dominant", "dry", "dull", "eager", "early", "easy",
		"elated", "elderly", "electric", "embarrassed", "emotional", "empty",
		"enchanted", "energetic", "enormous", "enthusiastic", "equal", "even",
		"evil", "excited", "exhausted", "elegant", "exclusive", "expensive",
		"expert", "exciting", "exotic", "fair", "false", "famous", "fantastic",
		"fast", "fearless", "feared", "fertile", "fervent", "few", "fine", "firm",
		"fit", "fluent", "focused", "fond", "formal", "fortunate", "fragrant",
		"free", "fresh", "friendly", "frugal", "funny", "generous", "gentle",
		"genuine", "glad", "gleaming", "global", "graceful", "grand", "great",
		"greedy", "green", "grey", "gross", "guilty", "happy", "hard", "harmless",
		"healthy", "heavy", "helpful", "hidden", "high", "hilarious", "historic",
		"hostile", "huge", "human", "hungry", "ideal", "ignorant", "illustrious",
		"impartial", "important", "impressed", "incomplete", "indebted",
		"indifferent", "inevitable", "innocent", "intelligent", "interesting",
		"internal", "international", "invisible", "irate", "isolated", "itchy",
		"joyful", "keen", "kind", "large", "lazy", "leading", "legal",
		"legendary", "liberal", "light", "likely", "limited", "lively", "logical",
		"lonely", "long", "loud", "lovely", "loyal", "lucky", "mad", "magic",
		"major", "mature", "mean", "medical", "mental", "merry", "messy", "mild",
		"mighty", "minor", "minute", "miraculous", "modern", "moody", "monstrous",
		"morose", "motivated", "naive", "narrow", "natural", "nearby", "neat",
		"necessary", "negative", "neither", "nerveless", "neutral", "nice",
		"noisy", "normal", "notable", "noticeable", "obedient", "obsessed",
		"obvious", "odd", "old", "open", "outgoing", "painful", "pale",
		"parallel", "partial", "passive", "patient", "peaceful", "perfect",
		"personal", "petty", "phenomenal", "plain", "pleasant", "plastic",
		"playful", "poetic", "polished", "popular", "portly", "positive",
		"powerful", "precise", "proud", "punctual", "pure", "purple", "quiet",
		"rapid", "rare", "rash", "raw", "ready", "real", "recent", "red",
		"regular", "relaxed", "relevant", "reliable", "remarkable", "remote",
		"renowned", "repentant", "resilient", "rich", "righteous", "rigid",
		"ritualistic", "robust", "round", "royal", "rude", "rustic", "sad",
		"safe", "salty", "scared", "scarce", "searching", "seasoned", "severe",
		"shallow", "sharp", "sheer", "short", "shy", "silent", "sinful", "slim",
		"slow", "smooth", "snappy", "soft", "solemn", "solid", "sorrowful",
		"sorry", "southern", "sparkling", "sparse", "special", "specific",
		"spicy", "spirited", "splendid", "stable", "static", "steady", "stern",
		"sticky", "strange", "strong", "stubborn", "studious", "subtle", "sudden",
		"superb", "sweet", "swift", "tall", "tangy", "targeted", "tasty",
		"temporary", "terrible", "thankful", "theatrical", "thin", "thoughtful",
		"tight", "timely", "tiny", "touching", "traditional", "transparent",
		"tragic", "treasured", "trendy", "tricky", "true", "trustworthy", "twin",
		"typical", "ultimate", "unanimous", "understated", "unequal", "unique",
		"universal", "unknown", "unlikely", "unusual", "upbeat", "urgent",
		"useful", "utilitarian", "utopian", "vague", "valid", "valuable",
		"varied", "vast", "vegetable", "vigorous", "violent", "virtual",
		"visible", "vivacious", "voluntary", "voracious", "warm", "weak",
		"wealthy", "wholesome", "wicked", "wide", "wild", "wise", "witty",
		"wrong", "zealous",
	}
	nouns = []string{
		"abacus", "apple", "badger", "banana", "banana", "book",
		"bookcase", "bridge", "butterfly", "camera", "candle", "canyon",
		"car", "castle", "catfish", "dagger", "daisy", "desert",
		"diamond", "dinosaur", "dog", "dolphin", "eagle", "eagle",
		"eggplant", "elephant", "elephant", "engine", "falcon", "falcon",
		"fireplace", "fish", "fishnet", "fountain", "garden", "garden",
		"gardenia", "garlic", "giraffe", "guitar", "hammer", "harbor",
		"harp", "harping", "house", "ice", "iceberg", "igloo",
		"ironclad", "island", "island", "ivory", "jacket", "jelly",
		"jellyfish", "jigsaw", "jungle", "jungle", "kangaroo", "kangaroo",
		"kayak", "kettle", "keyboard", "kitchen", "kiwi", "lamp",
		"lemon", "lemon", "lemonade", "lion", "lion", "lizard",
		"mango", "maple", "moon", "mountain", "mountain", "newspaper",
		"nightingale", "noodle", "note", "notebook", "nurse", "ocean",
		"ocean", "octopus", "orange", "orange", "owl", "panda",
		"parrot", "parrot", "pencil", "penguin", "pepper", "piano",
		"quail", "quartz", "queen", "quiche", "quill", "quilt",
		"quilt", "rabbit", "rabbit", "rabbit", "raspberry", "rooster",
		"sailboat", "snake", "star", "strawberry", "sunset", "sunset",
		"tiger", "tiger", "toaster", "tomato", "tree", "turtle",
		"ugli", "umbrella", "umbrella", "unicorn", "universe", "van",
		"velvet", "violin", "violin", "vulture", "walrus", "waterfall",
		"watermelon", "whale", "wheat", "window", "wolf", "xenopus",
		"xigua", "xylophone", "xylophone", "xylophone", "yacht", "yacht",
		"yarn", "yellow", "yellow", "yucca", "zebra", "zebra",
		"zebra", "zoo", "zucchini", "airplane", "antelope", "apple",
		"cactus", "falcon", "fishnet", "guitar", "honey", "honeycomb",
		"jacket", "jacket", "mango", "mushroom", "nightingale", "nightlight",
		"olive", "rabbit", "river", "sandwich", "tornado", "unicorn",
		"vinegar", "violin", "yak", "zebra",
	}
)

// defaultNamer returns a random adjective-noun name (for example "brave-running-otter").
func defaultNamer() string {
	return adjectives[rand.Intn(len(adjectives))] +
		"-" + participials[rand.Intn(len(participials))] +
		"-" + nouns[rand.Intn(len(nouns))]
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
