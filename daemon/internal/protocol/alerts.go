package protocol

// The wire shape of the alert settings (B9.4): which of the connected chat and phone channels each
// kind of alert is sent to. The Settings screen reads one answer and writes one body, so a change of
// mind about three alerts is one save and never leaves the choices half done.

// AlertRoute is where one kind of alert goes.
type AlertRoute struct {
	// Event names the kind of alert, in the daemon's own words, such as "agent.stuck".
	Event string `json:"event"`
	// Label is what the screen calls it, such as "An agent is stuck or needs you".
	Label string `json:"label"`
	// Channels are the channel ids it goes to, such as "telegram" and "ntfy". Empty means the alert
	// is off. Never null.
	Channels []string `json:"channels"`
}

// AlertChannel is one place an alert can be sent, and whether it can be used yet.
type AlertChannel struct {
	// ID is the channel's own id, the same as the connection that sends to it.
	ID string `json:"id"`
	// Name is what the screen calls it, such as "Telegram".
	Name string `json:"name"`
	// Connected is true when the connection is set up. A channel that is not can still be chosen, so
	// a choice made before connecting is kept, but nothing is sent until it is.
	Connected bool `json:"connected"`
}

// AlertSettings is the answer to GET and PUT /v1/settings/alerts.
type AlertSettings struct {
	// Routes has one row per kind of alert, in the order the screen shows them. Never null.
	Routes []AlertRoute `json:"routes"`
	// Channels has one entry per place an alert can be sent, in the order the screen shows them.
	// Never null.
	Channels []AlertChannel `json:"channels"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// AlertRouteChoice is one alert's channels as a person chose them.
type AlertRouteChoice struct {
	// Event names the kind of alert.
	Event string `json:"event"`
	// Channels are where it goes. Empty turns the alert off.
	Channels []string `json:"channels"`
}

// SaveAlertSettingsRequest is the body of PUT /v1/settings/alerts. An alert left out keeps the
// channels it had, so a screen may send only what changed.
type SaveAlertSettingsRequest struct {
	// Routes are the alerts whose channels changed.
	Routes []AlertRouteChoice `json:"routes"`
}
