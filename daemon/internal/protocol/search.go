package protocol

// Search over projects, cards, chats, past sessions, and notes for the command palette and the top
// bar (docs/backend-checklist.md B2.11 and B7.4, docs/backend-inventory.md N23, build-plan task
// 7.10). One request answers every kind at once, each kind in its own list, best match first, so
// the palette draws its Projects, Cards, Chats, Sessions, and Notes groups straight from the
// answer. The session and note kinds are the ones docs/architecture.md section 10 calls for: "a
// search uses SQLite full-text search over `session_events.summary` and card notes".

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
	// Sessions is how many past session events matched.
	Sessions int `json:"sessions"`
	// Notes is how many card notes matched.
	Notes int `json:"notes"`
}

// SessionHit is a line of a card's past that matched a search: one stored event of a card's session
// whose summary holds the words (docs/architecture.md section 10). It is how the palette answers
// "when did we do this before" without opening every card: the hit names the card the work happened
// on, and opening that card's chat shows the event in place.
type SessionHit struct {
	// CardID is the card the session belonged to, which opens it.
	CardID string `json:"cardId"`
	// Key is the card's project and number, such as "api#41".
	Key string `json:"key"`
	// Title is the card's title, so the palette says what the past work was about.
	Title string `json:"title"`
	// Excerpt is the stored summary of the event, clipped short. It is the one line the daemon kept
	// for that moment of the session, and the whole of what a search has to show.
	Excerpt string `json:"excerpt"`
	// ProjectID is the project the card belongs to.
	ProjectID string `json:"projectId"`
	// ProjectName is that project's display name.
	ProjectName string `json:"projectName"`
	// At is when the event happened.
	At Timestamp `json:"at"`
}

// NoteHit is a card note that matched a search: the markdown file the vault keeps for a card, whose
// text holds the words (docs/architecture.md sections 10 and 12). The note itself is read at its own
// route; a search answer carries only the beginning of it, because an answer that carried whole
// notes would cost more than the reading it saves.
type NoteHit struct {
	// CardID is the card the note belongs to, which opens it.
	CardID string `json:"cardId"`
	// Key is the card's project and number, such as "api#41".
	Key string `json:"key"`
	// Title is the card's title, so the palette says which note this is.
	Title string `json:"title"`
	// Path is where the note lives in the vault, relative to the vault root, in the shape
	// `<project>/cards/<number>-<title>.md`.
	Path string `json:"path"`
	// Excerpt is the beginning of the note, clipped short.
	Excerpt string `json:"excerpt"`
	// Author is who last wrote the note: "person" or "agent".
	Author NoteAuthor `json:"author"`
	// ProjectID is the project the card belongs to.
	ProjectID string `json:"projectId"`
	// ProjectName is that project's display name.
	ProjectName string `json:"projectName"`
	// At is when the note was last saved.
	At Timestamp `json:"at"`
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
	// Sessions are the past session events that matched, from every project, newest first among
	// equally good matches.
	Sessions []SessionHit `json:"sessions"`
	// Notes are the card notes that matched, from every project.
	Notes []NoteHit `json:"notes"`
	// Totals are how many of each kind matched before the lists were cut.
	Totals SearchTotals `json:"totals"`
	// ServerTime is the daemon's time when the search ran.
	ServerTime Timestamp `json:"serverTime"`
}
