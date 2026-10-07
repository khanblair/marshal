package protocol

import "time"

// The wire shape of a connection: a service Marshal has been set up with, and the last time it was
// tested (docs/architecture.md section 18, build-plan 6.11, B6.7, N18). One shape serves every
// kind - a model provider, GitHub, Trello, a calendar, a chat service - because section 18 gives
// them all the same connect, test, save-the-result, cool-down shape.

// IntegrationStatus is whether a connection is set up, as a screen shows it. There are three
// values and no more, on purpose: an installation that is under way is a fact about the browser a
// person is in, not about Marshal, and a daemon that stored "installing" would have to guess when
// that stopped being true. A screen that needs to show work in progress keeps that in its own
// state and never sends it here (the ruling is recorded in the Phase 6 report).
type IntegrationStatus string

const (
	// IntegrationStatusConnected means the connection is set up: a key or an install is stored.
	IntegrationStatusConnected IntegrationStatus = "connected"
	// IntegrationStatusNone means nothing is stored yet, so the connection cannot be used.
	IntegrationStatusNone IntegrationStatus = "none"
	// IntegrationStatusError means something is stored and the last test found it does not work.
	IntegrationStatusError IntegrationStatus = "error"
)

// IntegrationStatusValues lists every status. It is the three words the settings screen already
// reads for an integration row (`apps/web/src/mock/settings-types.ts`).
func IntegrationStatusValues() []IntegrationStatus {
	return []IntegrationStatus{
		IntegrationStatusConnected, IntegrationStatusNone, IntegrationStatusError,
	}
}

// Valid reports whether s is an integration status.
func (s IntegrationStatus) Valid() bool {
	for _, v := range IntegrationStatusValues() {
		if v == s {
			return true
		}
	}
	return false
}

// Integration is one connection as a screen sees it. The words shown to a person (a name, an icon,
// the sentence under it) are the app's, and are built from this: the wire carries the id, the kind,
// the status, and the last test's own answer.
type Integration struct {
	// ID is the connection's own id, and the id its keychain entry and its `integrations` row are
	// filed under: "github", "trello", "anthropic".
	ID string `json:"id"`
	// Kind is the sort of connection this is: "provider", "github", "calendar". It is the kind
	// its connection test is filed under.
	Kind string `json:"kind"`
	// Status says whether it is set up, and whether the last test found it working. The field is
	// called Status in Go and `st` on the wire because `st` is the name the screens already read.
	Status IntegrationStatus `json:"st"`
	// Detail is one plain sentence saying what is set up, such as "GitHub App installed on 3
	// repositories". It is empty for a connection nothing is stored for.
	Detail string `json:"detail"`
	// Target is where a chat connection sends its notices: the Discord channel id, the Telegram chat
	// id, or the ntfy topic. It is not a secret, so a screen can show it back. Empty for every other
	// connection, and for one nothing is stored for.
	Target string `json:"target,omitempty"`
	// LastTest is the result of the last connection test of this connection, or nil when it has
	// never been tested. It is the same shape a provider's test answers with, so one screen shows
	// both kinds of result.
	LastTest *TestResult `json:"lastTest,omitempty"`
}

// IntegrationList is the answer to GET /v1/integrations: every connection Marshal can be set up
// with, whether or not it is, so the screen can show the ones that are not connected yet.
type IntegrationList struct {
	// Integrations has one entry per connection Marshal knows, in the order the screen shows them.
	// Never null.
	Integrations []Integration `json:"integrations"`
	// ServerTime is the daemon's time when the answer was made, so a client counts a cooldown or
	// an age from it rather than from its own clock.
	ServerTime Timestamp `json:"serverTime"`
}

// NewIntegrationList makes an answer stamped with the daemon's time. A nil list becomes an empty
// one, so the JSON has [] and never null.
func NewIntegrationList(integrations []Integration, now time.Time) IntegrationList {
	out := make([]Integration, len(integrations))
	copy(out, integrations)
	return IntegrationList{Integrations: out, ServerTime: NewTimestamp(now)}
}

// SaveGitHubRequest is the body of the call that saves the GitHub App's connection (B6.1). The
// private key and the webhook secret are written to the keychain and never come back from any
// route; the two ids are written to the connection's own row.
type SaveGitHubRequest struct {
	// AppID is the App's own numeric id, from its settings page on GitHub.
	AppID int64 `json:"appId"`
	// InstallationID is the numeric id of the App's installation on the owner's account or
	// organization. One App can be installed in more than one place, and this is the one Marshal
	// acts as.
	InstallationID int64 `json:"installationId"`
	// PrivateKey is the App's private key, in PEM form, exactly as GitHub generated it.
	PrivateKey string `json:"privateKey"`
	// WebhookSecret is the secret the App's deliveries are signed with. Every delivery is checked
	// against it before its body is read (B6.1).
	WebhookSecret string `json:"webhookSecret"`
}

// SaveTrelloRequest is the body of the call that saves the Trello connection (B8.2). It is the
// connection's own shape rather than GitHub's: Trello has no App, so what it needs is the API key
// and token that stand for the person, the board Marshal watches, and the two things a delivery
// needs to be believed - the webhook secret and the callback URL the delivery was signed for.
//
// The token and the webhook secret are written to the keychain and never come back from any route;
// the key, the board, and the callback URL are written to the connection's own row.
type SaveTrelloRequest struct {
	// APIKey is the Trello API key, the public half of the credential.
	APIKey string `json:"apiKey"`
	// Token is the Trello token the key is used with. It is the secret half, and is the same one a
	// person copies from Trello's own "generate a token" page.
	Token string `json:"token"`
	// ProjectID is the Marshal project this board is linked to. One project links to one board
	// (docs/marshal-product-scope.md section 19.2), so the sync is one board in and one project out,
	// with no guessing about which card belongs where.
	ProjectID string `json:"projectId"`
	// BoardID is the board Marshal watches: the one cards are read from and written to.
	BoardID string `json:"boardId"`
	// NewCardListID is the board list a new Trello card is imported from: a card added there becomes
	// a Marshal card in ProjectID. Empty means Trello never creates a Marshal card, which is the
	// honest state of a connection that only reads.
	NewCardListID string `json:"newCardListId,omitempty"`
	// WebhookSecret is what Trello signs a delivery to this connection with. It is the `secret`
	// chosen when the webhook is made.
	WebhookSecret string `json:"webhookSecret"`
	// CallbackURL is the address the webhook was made for, exactly as it was registered with
	// Trello. Trello signs a delivery over its body followed by this URL, so Marshal cannot check a
	// signature without knowing it, and it cannot be guessed from the request.
	CallbackURL string `json:"callbackUrl"`
}

// SaveGoogleCalendarRequest is the body that saves the Google Calendar OAuth client (B8.3). This
// stores the client only; the token comes later, through the consent flow AuthorizeURL starts.
type SaveGoogleCalendarRequest struct {
	// ClientID is the OAuth client's id, from the Google Cloud console.
	ClientID string `json:"clientId"`
	// ClientSecret is the OAuth client's secret.
	ClientSecret string `json:"clientSecret"`
}

// AuthorizeURL is the answer to the call that starts a Google OAuth consent flow: the address a
// person opens in their own browser to grant Marshal read-only access.
type AuthorizeURL struct {
	URL string `json:"url"`
}

// SaveGmailRequest is the body that saves the Gmail connection (B8.3): which label to watch, and
// which Marshal project a labeled email becomes a card in. It carries no client of its own -
// Gmail shares the OAuth client and the token Google Calendar's own consent already granted.
type SaveGmailRequest struct {
	// Label is the Gmail label a person adds to an email to have Marshal turn it into a card.
	Label string `json:"label"`
	// ProjectID is the Marshal project a labeled email becomes a card in.
	ProjectID string `json:"projectId"`
}

// GitHubConnectState is where the GitHub sign-in is. A screen draws one panel for each value.
type GitHubConnectState string

const (
	// GitHubConnectStateIdle means nothing is connected and no sign-in is under way.
	GitHubConnectStateIdle GitHubConnectState = "idle"
	// GitHubConnectStatePending means a code was issued and the person has not approved it on GitHub yet.
	GitHubConnectStatePending GitHubConnectState = "pending"
	// GitHubConnectStateNeedsInstall means the person signed in but the Marshal GitHub App is not
	// installed on any of their accounts, so there is nothing for Marshal to see yet.
	GitHubConnectStateNeedsInstall GitHubConnectState = "needs_install"
	// GitHubConnectStateConnected means GitHub is connected, by a sign-in or by a pasted token.
	GitHubConnectStateConnected GitHubConnectState = "connected"
	// GitHubConnectStateDenied means the person refused the code on GitHub.
	GitHubConnectStateDenied GitHubConnectState = "denied"
	// GitHubConnectStateExpired means the code ran out before it was approved.
	GitHubConnectStateExpired GitHubConnectState = "expired"
	// GitHubConnectStateFailed means the sign-in could not be finished, with the reason in Message.
	GitHubConnectStateFailed GitHubConnectState = "failed"
)

// GitHubConnectStateValues lists every state of the GitHub sign-in.
func GitHubConnectStateValues() []GitHubConnectState {
	return []GitHubConnectState{
		GitHubConnectStateIdle, GitHubConnectStatePending, GitHubConnectStateNeedsInstall,
		GitHubConnectStateConnected, GitHubConnectStateDenied, GitHubConnectStateExpired,
		GitHubConnectStateFailed,
	}
}

// GitHubInstallation is one account the Marshal GitHub App is installed on.
type GitHubInstallation struct {
	// Account is the user or organization login.
	Account string `json:"account"`
	// Kind is "user" or "organization".
	Kind string `json:"kind"`
	// AllRepositories is true when the installation sees every repository the account owns.
	AllRepositories bool `json:"allRepositories"`
}

// GitHubConnect is the answer to every call that starts, reads, or ends the GitHub sign-in. One
// shape serves all its states; a field that does not apply to the state is left out.
type GitHubConnect struct {
	State GitHubConnectState `json:"state"`
	// Mode is how GitHub is connected once it is: "oauth" for the sign-in, "token" for a pasted one.
	Mode string `json:"mode,omitempty"`
	// UserCode is the short code the person types on GitHub. Set while pending.
	UserCode string `json:"userCode,omitempty"`
	// VerificationURI is the GitHub page the code is typed on. Set while pending.
	VerificationURI string `json:"verificationUri,omitempty"`
	// ExpiresAt is when the pending code stops working.
	ExpiresAt *Timestamp `json:"expiresAt,omitempty"`
	// Login is the GitHub user Marshal is signed in as, once known.
	Login string `json:"login,omitempty"`
	// InstallURL is the page that installs the Marshal GitHub App on an account or organization. Set
	// while needs_install, and when connected through a sign-in, for "add another account".
	InstallURL string `json:"installUrl,omitempty"`
	// Installations lists the accounts the App is installed on. Never null; empty for a token.
	Installations []GitHubInstallation `json:"installations"`
	// Message is one plain sentence for the person, set when the state needs explaining.
	Message string `json:"message,omitempty"`
}

// SaveGitHubTokenRequest is the body of the calls that save or test a pasted GitHub token.
type SaveGitHubTokenRequest struct {
	// Token is a GitHub personal access token. It is written to the keychain and never comes back.
	Token string `json:"token"`
}
