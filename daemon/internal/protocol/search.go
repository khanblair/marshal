package protocol

// Search over projects, cards, and chats for the command palette and the top bar
// (docs/backend-checklist.md B2.11, docs/backend-inventory.md N23). One request answers every
// kind at once, each kind in its own list, best match first, so the palette draws its Projects,
// Cards, and Chats groups straight from the answer. Past sessions and notes join in Phase 7.

// SearchHitsPerKind is the most hits of one kind that a search answer holds. The totals say how
// many there were before the cut.
const SearchHitsPerKind = 8

// MaxSearchQueryChars is the longest query, in characters, that a search accepts.
const MaxSearchQueryChars = 200

// ProjectHit is a project that matched a search, by its name or its folder.
type ProjectHit struct {
	// ProjectID is the project's short id, which opens it.
	ProjectID string `json:"projectId"`
	// Name is the project's display name, which the palette shows.
	Name string `json:"name"`
	// Path is the top folder of the repository on the machine that runs the daemon.
	Path string `json:"path"`
	// Language is what the daemon found in the repository, which the palette shows as the hint.
	Language string `json:"language"`
}

// CardHit is a card that matched a search, by its number, its title, or its description.
type CardHit struct {
	// CardID is the card's opaque id, which routes and the `card:<id>` topic use.
	CardID string `json:"cardId"`
	// Key is the card's project and number, such as "api#41", which the app names cards by.
	Key string `json:"key"`
	// Number is the card's number in its project, which the palette shows as "#41".
	Number int `json:"number"`
	// Title is the card's title.
	Title string `json:"title"`
	// State is the card's column, which picks the palette's icon and its color.
	State CardState `json:"state"`
	// ProjectID is the project the card belongs to.
	ProjectID string `json:"projectId"`
	// ProjectName is that project's display name, shown next to the number because one list mixes
	// the cards of every project.
	ProjectName string `json:"projectName"`
}

// ChatHit is a project chat that matched a search, by its title. Only chats in the main list
// are searched; archived chats stay behind the Archived toggle.
type ChatHit struct {
	// ChatID is the chat's opaque id, which opens it.
	ChatID string `json:"chatId"`
	// Title is the chat's name.
	Title string `json:"title"`
	// ProjectID is the project the chat belongs to.
	ProjectID string `json:"projectId"`
	// ProjectName is that project's display name.
	ProjectName string `json:"projectName"`
	// LastActiveAt is when the chat last had a message.
	LastActiveAt Timestamp `json:"lastActiveAt"`
}

// SearchTotals says how many things of each kind matched, before each list was cut to
// SearchHitsPerKind.
type SearchTotals struct {
	// Projects is how many projects matched.
	Projects int `json:"projects"`
	// Cards is how many cards matched.
	Cards int `json:"cards"`
	// Chats is how many chats matched.
	Chats int `json:"chats"`
}

// SearchSnapshot is the answer to GET /v1/search?q=. Each list is best match first and holds at
// most SearchHitsPerKind hits. An empty query matches nothing, since the palette lists its own
// commands until something is typed. The lists are never null.
type SearchSnapshot struct {
	// Query is the query the daemon searched for: the one sent, trimmed, with each run of spaces
	// made one. A client typing ahead drops an answer whose query is not its latest.
	Query string `json:"query"`
	// Projects are the projects that matched.
	Projects []ProjectHit `json:"projects"`
	// Cards are the cards that matched, from every project.
	Cards []CardHit `json:"cards"`
	// Chats are the chats that matched, from every project.
	Chats []ChatHit `json:"chats"`
	// Totals are how many of each kind matched before the lists were cut.
	Totals SearchTotals `json:"totals"`
	// ServerTime is the daemon's time when the search ran.
	ServerTime Timestamp `json:"serverTime"`
}
