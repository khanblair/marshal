package protocol

import (
	"fmt"
	"slices"
	"unicode/utf8"
)

// The person using Marshal (docs/backend-checklist.md B2.2, B2.5, and B2.13, inventory N27 and N30,
// decision D2): their profile, the users list that member pickers read, their onboarding and tour
// progress, and the screen preferences that follow them between devices. All of it is per user
// from the start, though solo use has one user, the owner. Every change is published as me.updated
// on the `me` topic with the whole of the three as they now are, so a second device that applies
// it lands in the same place as the one that made the change.

// Theme is the color scheme a person picked. "system" follows the operating system.
type Theme string

const (
	// ThemeLight is the light scheme.
	ThemeLight Theme = "light"
	// ThemeDark is the dark scheme.
	ThemeDark Theme = "dark"
	// ThemeSystem follows the operating system's scheme. It is what a new user has.
	ThemeSystem Theme = "system"
)

// ThemeValues lists every theme, in the order the Settings theme cards show them.
func ThemeValues() []Theme { return []Theme{ThemeLight, ThemeDark, ThemeSystem} }

// Valid reports whether t is a theme.
func (t Theme) Valid() bool { return slices.Contains(ThemeValues(), t) }

// ProjectView is one of a project's views, the tabs under its name. A project remembers the one
// that was open last.
type ProjectView string

const (
	// ProjectViewChat is the project's chats.
	ProjectViewChat ProjectView = "chat"
	// ProjectViewAgents is the Agents table.
	ProjectViewAgents ProjectView = "agents"
	// ProjectViewBoard is the board. It is what a project opens in until another view is chosen.
	ProjectViewBoard ProjectView = "board"
	// ProjectViewList is the List table.
	ProjectViewList ProjectView = "list"
	// ProjectViewTimeline is the Timeline.
	ProjectViewTimeline ProjectView = "timeline"
	// ProjectViewCalendar is the Calendar.
	ProjectViewCalendar ProjectView = "calendar"
)

// ProjectViewValues lists every project view, in the order of the view switcher.
func ProjectViewValues() []ProjectView {
	return []ProjectView{
		ProjectViewChat, ProjectViewAgents, ProjectViewBoard, ProjectViewList, ProjectViewTimeline, ProjectViewCalendar,
	}
}

// Valid reports whether v is a project view.
func (v ProjectView) Valid() bool { return slices.Contains(ProjectViewValues(), v) }

// Swimlane is what a board's rows are grouped by. "none" draws one row.
type Swimlane string

const (
	// SwimlaneNone draws the board as one row. It is what a board has until another is chosen.
	SwimlaneNone Swimlane = "none"
	// SwimlaneRole groups cards by their role.
	SwimlaneRole Swimlane = "role"
	// SwimlaneAgent groups cards by their agent.
	SwimlaneAgent Swimlane = "agent"
	// SwimlanePackage groups cards by their package in a monorepo.
	SwimlanePackage Swimlane = "package"
	// SwimlaneLabel groups cards by their labels.
	SwimlaneLabel Swimlane = "label"
)

// SwimlaneValues lists every swimlane, in the order of the board's Swimlanes menu.
func SwimlaneValues() []Swimlane {
	return []Swimlane{SwimlaneNone, SwimlaneRole, SwimlaneAgent, SwimlanePackage, SwimlaneLabel}
}

// Valid reports whether s is a swimlane.
func (s Swimlane) Valid() bool { return slices.Contains(SwimlaneValues(), s) }

// FilterKey is what a filter chip filters by.
type FilterKey string

const (
	// FilterKeyStatus filters by the card's column.
	FilterKeyStatus FilterKey = "status"
	// FilterKeyRole filters by the card's role.
	FilterKeyRole FilterKey = "role"
	// FilterKeyAgent filters by the card's agent.
	FilterKeyAgent FilterKey = "agent"
	// FilterKeyModel filters by the card's model.
	FilterKeyModel FilterKey = "model"
	// FilterKeyLabel filters by one of the card's labels.
	FilterKeyLabel FilterKey = "label"
	// FilterKeyPackage filters by the card's package.
	FilterKeyPackage FilterKey = "package"
)

// FilterKeyValues lists every filter key, in the order of the board's Filter menu.
func FilterKeyValues() []FilterKey {
	return []FilterKey{
		FilterKeyStatus, FilterKeyRole, FilterKeyAgent, FilterKeyModel, FilterKeyLabel, FilterKeyPackage,
	}
}

// Valid reports whether k is a filter key.
func (k FilterKey) Valid() bool { return slices.Contains(FilterKeyValues(), k) }

// SortDirection says which way a table is sorted.
type SortDirection string

const (
	// SortDirectionAsc sorts from the smallest value up.
	SortDirectionAsc SortDirection = "asc"
	// SortDirectionDesc sorts from the largest value down.
	SortDirectionDesc SortDirection = "desc"
)

// SortDirectionValues lists both directions.
func SortDirectionValues() []SortDirection {
	return []SortDirection{SortDirectionAsc, SortDirectionDesc}
}

// Valid reports whether d is a sort direction.
func (d SortDirection) Valid() bool { return slices.Contains(SortDirectionValues(), d) }

// ProgressStatus is where a person is with onboarding or with the tour.
type ProgressStatus string

const (
	// ProgressStatusPending means it has not been finished: onboarding shows, at its saved step,
	// or the tour starts the next time Home opens.
	ProgressStatusPending ProgressStatus = "pending"
	// ProgressStatusDone means the person went through to the end.
	ProgressStatusDone ProgressStatus = "done"
	// ProgressStatusSkipped means the person skipped it. It does not show again by itself.
	ProgressStatusSkipped ProgressStatus = "skipped"
)

// ProgressStatusValues lists every progress status.
func ProgressStatusValues() []ProgressStatus {
	return []ProgressStatus{ProgressStatusPending, ProgressStatusDone, ProgressStatusSkipped}
}

// Valid reports whether s is a progress status.
func (s ProgressStatus) Valid() bool { return slices.Contains(ProgressStatusValues(), s) }

// Profile is the answer to GET /v1/me and PATCH /v1/me: the person the token belongs to. Paired
// devices are not here; they stay on the mock until Phase 9 adds pairing.
type Profile struct {
	// ID is the user's opaque id, the same as WhoAmI.UserID.
	ID string `json:"id"`
	// Name is the person's name. It is never empty.
	Name string `json:"name"`
	// Email is the address briefs are sent to, and empty when none was given.
	Email string `json:"email"`
	// Initials are up to two capital letters from the name, which the avatar shows when there is
	// no image.
	Initials string `json:"initials"`
	// TimeZone is an IANA time zone name, such as "Europe/London", that briefs and schedules run
	// in. Empty means none was chosen yet.
	TimeZone string `json:"timeZone"`
	// AvatarURL is the daemon path of the avatar image, with a version so a new image is a new
	// address, and null when there is no image. It needs the token like every other route, so a
	// client fetches it with the Authorization header and shows the bytes; an <img> tag cannot.
	AvatarURL *string `json:"avatarUrl" tstype:"string | null"`
	// TailnetIdentity is the Tailscale account this machine is signed in as. Empty until remote
	// access arrives in Phase 9.
	TailnetIdentity string `json:"tailnetIdentity"`
	// CreatedAt is when the user was made.
	CreatedAt Timestamp `json:"createdAt"`
	// UpdatedAt is when the profile last changed, including the avatar.
	UpdatedAt Timestamp `json:"updatedAt"`
}

// UpdateProfileRequest is the body of PATCH /v1/me. A field that is not set is left alone.
type UpdateProfileRequest struct {
	// Name renames the person. It cannot be empty.
	Name *string `json:"name,omitempty"`
	// Email changes the address. The empty string removes it.
	Email *string `json:"email,omitempty"`
	// TimeZone changes the time zone to an IANA name. The empty string removes it.
	TimeZone *string `json:"timeZone,omitempty"`
}

// User is one person in the users list that member pickers read.
type User struct {
	// ID is the user's opaque id.
	ID string `json:"id"`
	// Name is the person's name.
	Name string `json:"name"`
	// Initials are up to two capital letters from the name.
	Initials string `json:"initials"`
	// AvatarURL is the daemon path of their avatar image, or null when they have none.
	AvatarURL *string `json:"avatarUrl" tstype:"string | null"`
}

// UserListSnapshot is the answer to GET /v1/users. Solo use lists the owner only (decision D10).
type UserListSnapshot struct {
	// Users are everyone who can be put on a card, by name.
	Users []User `json:"users"`
	// ServerTime is the daemon's time when the list was made.
	ServerTime Timestamp `json:"serverTime"`
}

// Progress is the answer to GET and PATCH /v1/me/progress: how far a person is with onboarding
// and with the tour on Home. It is saved per user, so a new device resumes where another left off.
type Progress struct {
	// Onboarding is the first-launch screens.
	Onboarding OnboardingProgress `json:"onboarding"`
	// Tutorial is the tour on Home.
	Tutorial TutorialProgress `json:"tutorial"`
}

// OnboardingProgress is where a person is with the first-launch screens.
type OnboardingProgress struct {
	// Status is pending until the person finishes or skips onboarding.
	Status ProgressStatus `json:"status"`
	// Step is the screen onboarding resumes at, counted from 0.
	Step int `json:"step"`
	// FinishedAt is when onboarding was finished or skipped, and null while it is pending.
	FinishedAt *Timestamp `json:"finishedAt" tstype:"Timestamp | null"`
}

// TutorialProgress is where a person is with the tour. The step the tour is on is screen state;
// only finishing or skipping it is saved.
type TutorialProgress struct {
	// Status is pending until the person finishes or skips the tour. Replaying it sets it back.
	Status ProgressStatus `json:"status"`
	// FinishedAt is when the tour was finished or skipped, and null while it is pending.
	FinishedAt *Timestamp `json:"finishedAt" tstype:"Timestamp | null"`
}

// UpdateProgressRequest is the body of PATCH /v1/me/progress. A part that is not set is left alone.
type UpdateProgressRequest struct {
	// Onboarding changes the onboarding progress.
	Onboarding *UpdateOnboardingProgress `json:"onboarding,omitempty"`
	// Tutorial changes the tour progress.
	Tutorial *UpdateTutorialProgress `json:"tutorial,omitempty"`
}

// UpdateOnboardingProgress changes onboarding. A field that is not set is left alone.
type UpdateOnboardingProgress struct {
	// Step saves the screen to resume at, counted from 0.
	Step *int `json:"step,omitempty"`
	// Status finishes onboarding (done), skips it (skipped), or shows it again (pending).
	Status *ProgressStatus `json:"status,omitempty"`
}

// UpdateTutorialProgress changes the tour. A field that is not set is left alone.
type UpdateTutorialProgress struct {
	// Status finishes the tour (done), skips it (skipped), or sets it to show again (pending).
	Status *ProgressStatus `json:"status,omitempty"`
}

// Filter is one filter chip: a key and the value it keeps. The value is the text the chip shows,
// such as a column, a role, or a label name.
type Filter struct {
	// Key is what the chip filters by.
	Key FilterKey `json:"key"`
	// Value is what the card must have.
	Value string `json:"value"`
}

// maxFilters is the most filter chips one board, or one saved view, can hold.
const maxFilters = 50

// maxFilterValueChars is the longest value a filter chip can have.
const maxFilterValueChars = 200

// CheckFilters refuses a list of filter chips that a board cannot hold: an unknown key, an empty
// or overlong value, or more chips than a board shows. Both the preferences and the saved views
// keep filters, so the rule is here once.
func CheckFilters(filters []Filter) error {
	if len(filters) > maxFilters {
		return InvalidArgument(fmt.Sprintf("A board can have at most %d filters. Remove some and try again.", maxFilters))
	}
	for _, filter := range filters {
		if !filter.Key.Valid() {
			return InvalidArgument("That is not a filter Marshal knows. Filter by status, role, agent, model, label, or package.").
				With("key", string(filter.Key))
		}
		if filter.Value == "" || utf8.RuneCountInString(filter.Value) > maxFilterValueChars {
			return InvalidArgument(fmt.Sprintf("A filter needs a value of at most %d characters.", maxFilterValueChars))
		}
	}
	return nil
}

// SortOrder is how one table is sorted: by which column, and which way.
type SortOrder struct {
	// Key is the column's short key, such as "state" or "id". The screen owns the columns.
	Key string `json:"key"`
	// Direction is which way the column is sorted.
	Direction SortDirection `json:"direction"`
}

// SortPreferences are the sort orders of the two tables that can be sorted. A null order means
// the person never changed it, and the screen uses its own default.
type SortPreferences struct {
	// Agents is the Agents table's order.
	Agents *SortOrder `json:"agents" tstype:"SortOrder | null"`
	// List is the List table's order.
	List *SortOrder `json:"list" tstype:"SortOrder | null"`
}

// ProjectPreferences are one project's screen preferences (decision D2): what the project opens
// in, and the board's filters, search, and swimlane as they were left.
type ProjectPreferences struct {
	// LastView is the view the project opens in.
	LastView ProjectView `json:"lastView"`
	// Filters are the board's filter chips.
	Filters []Filter `json:"filters"`
	// Query is the board's search text.
	Query string `json:"query"`
	// Swimlane is what the board's rows are grouped by.
	Swimlane Swimlane `json:"swimlane"`
	// CollapsedLanes are the lanes that are folded, each as "<swimlane>:<lane>".
	CollapsedLanes []string `json:"collapsedLanes"`
	// ShowAllDone is true when the Done column shows every card rather than the newest 20.
	ShowAllDone bool `json:"showAllDone"`
	// SavedViewID is the saved view in use, and null when the filters were changed by hand or the
	// view was deleted.
	SavedViewID *string `json:"savedViewId" tstype:"string | null"`
}

// Preferences is the answer to GET and PATCH /v1/me/preferences: the screen preferences that
// follow a person between devices (decision D2). Layout (panel widths, a collapsed sidebar, split
// panes, the calendar's mode, and the dashboard's range) stays on each device and is not here.
type Preferences struct {
	// Theme is the color scheme.
	Theme Theme `json:"theme"`
	// ListColumns says which List columns are shown, by the column's short key. Only the columns
	// the person changed are here; the screen's own default covers the rest.
	ListColumns map[string]bool `json:"listColumns"`
	// Sort is how the two tables are sorted.
	Sort SortPreferences `json:"sort"`
	// Projects are each project's own preferences, by project id. A project that is not here
	// has the screen's defaults.
	Projects map[string]ProjectPreferences `json:"projects"`
}

// UpdatePreferencesRequest is the body of PATCH /v1/me/preferences. A field that is not set is left
// alone, and a project that is not named keeps its preferences.
type UpdatePreferencesRequest struct {
	// Theme changes the color scheme.
	Theme *Theme `json:"theme,omitempty"`
	// ListColumns shows or hides List columns. Each key named is set; the others keep their value.
	ListColumns map[string]bool `json:"listColumns,omitempty"`
	// Sort changes a table's order.
	Sort *UpdateSortPreferences `json:"sort,omitempty"`
	// Projects changes the preferences of the projects named, by project id.
	Projects map[string]UpdateProjectPreferences `json:"projects,omitempty"`
}

// UpdateSortPreferences changes the sort order of the tables it names.
type UpdateSortPreferences struct {
	// Agents sets the Agents table's order.
	Agents *SortOrder `json:"agents,omitempty"`
	// List sets the List table's order.
	List *SortOrder `json:"list,omitempty"`
}

// UpdateProjectPreferences changes one project's preferences. A field that is not set is left
// alone.
type UpdateProjectPreferences struct {
	// LastView changes the view the project opens in.
	LastView *ProjectView `json:"lastView,omitempty"`
	// Filters replaces the board's filter chips.
	Filters *[]Filter `json:"filters,omitempty"`
	// Query replaces the board's search text.
	Query *string `json:"query,omitempty"`
	// Swimlane changes what the board's rows are grouped by.
	Swimlane *Swimlane `json:"swimlane,omitempty"`
	// CollapsedLanes replaces the folded lanes.
	CollapsedLanes *[]string `json:"collapsedLanes,omitempty"`
	// ShowAllDone changes whether Done shows every card.
	ShowAllDone *bool `json:"showAllDone,omitempty"`
	// SavedViewID sets the saved view in use, one of this project's. The empty string clears it.
	SavedViewID *string `json:"savedViewId,omitempty"`
}

// MeUpdatedEventData is the payload of me.updated, on the `me` topic. It carries the profile, the
// preferences, and the progress as they are now, whichever of them changed, so a client replaces
// all three and applying the event twice changes nothing.
type MeUpdatedEventData struct {
	// Profile is the profile as it is now.
	Profile Profile `json:"profile"`
	// Preferences are the preferences as they are now.
	Preferences Preferences `json:"preferences"`
	// Progress is the progress as it is now.
	Progress Progress `json:"progress"`
}
