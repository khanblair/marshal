package protocol

// GoogleCalendarChoice is one calendar a person has, for the tick list in Settings. Marshal reads
// the ticked ones for the calendar view, Home's coming-up list, and the briefs.
type GoogleCalendarChoice struct {
	// ID is Google's own id for the calendar.
	ID string `json:"id"`
	// Name is what the person calls it.
	Name string `json:"name"`
	// Color is the calendar's own color as #rrggbb, or empty.
	Color string `json:"color"`
	// Primary is the calendar named after the person's own address.
	Primary bool `json:"primary"`
	// Owned says the person owns it, as opposed to being shared it or subscribed to it.
	Owned bool `json:"owned"`
	// Selected says Marshal reads it. Until the person chooses, it follows what is ticked in Google
	// Calendar's own side list.
	Selected bool `json:"selected"`
}

// GoogleCalendarChoices is the answer to GET /v1/integrations/gcal/calendars.
type GoogleCalendarChoices struct {
	Calendars []GoogleCalendarChoice `json:"calendars"`
	// Chosen says the person has picked calendars themselves, so Selected is theirs and no longer
	// follows Google's side list.
	Chosen bool `json:"chosen"`
}

// SetGoogleCalendarsRequest is the body of PUT /v1/integrations/gcal/calendars: the ids of the
// calendars Marshal should read. An empty list is valid and means none.
type SetGoogleCalendarsRequest struct {
	IDs []string `json:"ids"`
}

// GoogleClientInfo is the answer to GET /v1/integrations/gcal/client: which Google client Marshal
// would connect with.
type GoogleClientInfo struct {
	// Bundled says this build has Marshal's own Google client, so a person connects with one click
	// and pastes nothing.
	Bundled bool `json:"bundled"`
	// Own says the person saved a client of their own, which is then the one used.
	Own bool `json:"own"`
}
